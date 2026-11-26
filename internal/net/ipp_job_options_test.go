package net

import (
	"strconv"
	"testing"

	"github.com/OpenPrinting/goipp"
)

type qualitySpooler struct{ *fakeSpooler }

func (f *qualitySpooler) PrintQualities(string) ([]int, int, bool) {
	return []int{3, 4, 5}, 4, true
}

func TestRejectsUnsupportedJobOptions(t *testing.T) {
	for _, op := range []goipp.Op{goipp.OpPrintJob, goipp.OpValidateJob, goipp.OpCreateJob} {
		shared := testShared()
		req := goipp.NewRequest(ipp11, op, 1)
		req.Operation.Add(goipp.MakeAttr("ipp-attribute-fidelity", goipp.TagBoolean, goipp.Boolean(true)))
		req.Job.Add(goipp.MakeAttr("sides", goipp.TagKeyword, goipp.String("two-sided-long-edge")))
		rsp := serveIPP(req, []byte("%PDF-1.4"), testPrinter(), shared, "pc", 631)
		if goipp.Status(rsp.Code) != goipp.StatusErrorAttributesOrValues || shared.Spool.(*fakeSpooler).calls != 0 {
			t.Fatalf("%s status = %s", op, goipp.Status(rsp.Code))
		}
	}
	req := goipp.NewRequest(ipp11, goipp.OpValidateJob, 1)
	req.Job.Add(goipp.MakeAttr("page-ranges", goipp.TagRange, goipp.Range{Lower: 0, Upper: 2}))
	if rsp := serveIPP(req, nil, testPrinter(), testShared(), "pc", 631); goipp.Status(rsp.Code) != goipp.StatusErrorAttributesOrValues {
		t.Fatalf("invalid page range status = %s", goipp.Status(rsp.Code))
	}
}

func TestNonFidelityIgnoresUnsupportedOptions(t *testing.T) {
	for _, op := range []goipp.Op{goipp.OpPrintJob, goipp.OpValidateJob, goipp.OpCreateJob} {
		t.Run(op.String(), func(t *testing.T) {
			shared := testShared()
			req := goipp.NewRequest(ipp11, op, 1)
			req.Job.Add(goipp.MakeAttr("sides", goipp.TagKeyword, goipp.String("two-sided-long-edge")))
			req.Job.Add(goipp.MakeAttr("print-quality", goipp.TagEnum, goipp.Integer(5)))
			rsp := serveIPP(req, []byte("%PDF-1.4"), testPrinter(), shared, "pc", 631)
			if goipp.Status(rsp.Code) != goipp.StatusOkIgnoredOrSubstituted || len(rsp.Unsupported) != 2 ||
				rsp.Unsupported[0].Name != "sides" || rsp.Unsupported[1].Name != "print-quality" {
				t.Fatalf("status = %s, unsupported = %v", goipp.Status(rsp.Code), rsp.Unsupported)
			}
			if op != goipp.OpValidateJob {
				id := responseJobID(t, rsp)
				job, _ := shared.getIPPJob("hp", id)
				if opAttrString(job.template, "sides") != "" {
					t.Fatal("ignored value was retained in the job")
				}
				if op == goipp.OpCreateJob {
					shared.cancelIPPJob("hp", id)
				} else {
					waitIPPJob(t, shared, id)
				}
			}
		})
	}
}

func TestOrientationOptionsArePreserved(t *testing.T) {
	for orientation := 3; orientation <= 6; orientation++ {
		shared := testShared()
		create := goipp.NewRequest(ipp11, goipp.OpCreateJob, 1)
		create.Job.Add(goipp.MakeAttr("orientation-requested", goipp.TagEnum, goipp.Integer(orientation)))
		rsp := serveIPP(create, nil, testPrinter(), shared, "pc", 631)
		if goipp.Status(rsp.Code) != goipp.StatusOk {
			t.Fatalf("orientation %d create = %s", orientation, goipp.Status(rsp.Code))
		}
		id, _ := strconv.Atoi(attrValues(t, rsp.Job, "job-id")[0])
		send := goipp.NewRequest(ipp11, goipp.OpSendDocument, 2)
		send.Operation.Add(goipp.MakeAttr("job-id", goipp.TagInteger, goipp.Integer(id)))
		send.Operation.Add(goipp.MakeAttr("last-document", goipp.TagBoolean, goipp.Boolean(true)))
		rsp = serveIPP(send, []byte("%PDF-1.4"), testPrinter(), shared, "pc", 631)
		waitIPPJob(t, shared, uint32(id))
		if goipp.Status(rsp.Code) != goipp.StatusOk || shared.Spool.(*fakeSpooler).lastOptions.Orientation != orientation {
			t.Fatalf("orientation %d send = %s, options = %+v", orientation, goipp.Status(rsp.Code), shared.Spool.(*fakeSpooler).lastOptions)
		}
	}
}

func TestDriverSupportedPrintQuality(t *testing.T) {
	shared := testShared()
	spooler := &qualitySpooler{fakeSpooler: &fakeSpooler{}}
	shared.SetSpooler(spooler)
	get := goipp.NewRequest(ipp11, goipp.OpGetPrinterAttributes, 1)
	rsp := serveIPP(get, nil, testPrinter(), shared, "pc", 631)
	if got := attrValues(t, rsp.Printer, "print-quality-supported"); len(got) != 3 || got[0] != "3" || got[2] != "5" {
		t.Fatalf("quality support = %v", got)
	}
	req := goipp.NewRequest(ipp11, goipp.OpPrintJob, 2)
	req.Operation.Add(goipp.MakeAttr("ipp-attribute-fidelity", goipp.TagBoolean, goipp.Boolean(true)))
	req.Job.Add(goipp.MakeAttr("print-quality", goipp.TagEnum, goipp.Integer(5)))
	rsp = serveIPP(req, []byte("%PDF-1.4"), testPrinter(), shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusOk {
		t.Fatalf("print status = %s", goipp.Status(rsp.Code))
	}
	waitIPPJob(t, shared, responseJobID(t, rsp))
	if spooler.lastOptions.Quality != 5 {
		t.Fatalf("spooled quality = %d", spooler.lastOptions.Quality)
	}
	req.Code = goipp.Code(goipp.OpValidateJob)
	req.Job.Add(goipp.MakeAttr("printer-resolution", goipp.TagResolution,
		goipp.Resolution{Xres: 600, Yres: 600, Units: goipp.UnitsDpi}))
	rsp = serveIPP(req, nil, testPrinter(), shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusErrorAttributesOrValues {
		t.Fatalf("conflicting resolution status = %s", goipp.Status(rsp.Code))
	}
}

func TestPageOverridesPreserveOptions(t *testing.T) {
	shared := testShared()
	create := goipp.NewRequest(ipp11, goipp.OpCreateJob, 1)
	create.Job.Add(goipp.MakeAttr("print-color-mode", goipp.TagKeyword, goipp.String("monochrome")))
	create.Job.Add(goipp.MakeAttrCollection("overrides",
		goipp.MakeAttr("pages", goipp.TagRange, goipp.Range{Lower: 2, Upper: 2}),
		goipp.MakeAttr("document-numbers", goipp.TagRange, goipp.Range{Lower: 1, Upper: 1}),
		goipp.MakeAttr("print-color-mode", goipp.TagKeyword, goipp.String("color"))))
	rsp := serveIPP(create, nil, testPrinter(), shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusOk {
		t.Fatalf("create status = %s", goipp.Status(rsp.Code))
	}
	id, _ := strconv.Atoi(attrValues(t, rsp.Job, "job-id")[0])
	send := goipp.NewRequest(ipp11, goipp.OpSendDocument, 2)
	send.Operation.Add(goipp.MakeAttr("job-id", goipp.TagInteger, goipp.Integer(id)))
	send.Operation.Add(goipp.MakeAttr("last-document", goipp.TagBoolean, goipp.Boolean(true)))
	rsp = serveIPP(send, []byte("%PDF-1.4"), testPrinter(), shared, "pc", 631)
	waitIPPJob(t, shared, uint32(id))
	options := shared.Spool.(*fakeSpooler).lastOptions
	if goipp.Status(rsp.Code) != goipp.StatusOk || !options.RequiresColor() ||
		!options.PageMonochrome(1, 3, false) || options.PageMonochrome(2, 3, false) ||
		!options.PageMonochrome(3, 3, false) {
		t.Fatalf("status = %s, options = %+v", goipp.Status(rsp.Code), options)
	}
	job, _ := shared.getIPPJob("hp", uint32(id))
	if len(attrValues(t, ippJobAttrs(job, testPrinter(), "pc", 631, nil), "overrides")) != 1 {
		t.Fatal("job attributes did not retain page overrides")
	}
}

func TestRejectsInvalidOverrides(t *testing.T) {
	for _, attr := range []goipp.Attribute{
		goipp.MakeAttrCollection("overrides",
			goipp.MakeAttr("pages", goipp.TagRange, goipp.Range{Lower: 3, Upper: 1}),
			goipp.MakeAttr("print-color-mode", goipp.TagKeyword, goipp.String("color"))),
		goipp.MakeAttrCollection("overrides",
			goipp.MakeAttr("pages", goipp.TagRange, goipp.Range{Lower: 1, Upper: 2}),
			goipp.MakeAttr("sides", goipp.TagKeyword, goipp.String("two-sided-long-edge"))),
	} {
		req := goipp.NewRequest(ipp11, goipp.OpValidateJob, 1)
		req.Job.Add(attr)
		rsp := serveIPP(req, nil, testPrinter(), testShared(), "pc", 631)
		if goipp.Status(rsp.Code) != goipp.StatusErrorAttributesOrValues {
			t.Fatalf("invalid override status = %s", goipp.Status(rsp.Code))
		}
	}
}

func TestCreateJobPreservesPrintOptions(t *testing.T) {
	shared := testShared()
	req := goipp.NewRequest(ipp11, goipp.OpCreateJob, 1)
	req.Job.Add(goipp.MakeAttr("page-ranges", goipp.TagRange, goipp.Range{Lower: 2, Upper: 3}))
	req.Job.Add(goipp.MakeAttr("print-color-mode", goipp.TagKeyword, goipp.String("monochrome")))
	rsp := serveIPP(req, nil, testPrinter(), shared, "pc", 631)
	values := attrValues(t, rsp.Job, "job-id")
	if len(values) != 1 {
		t.Fatalf("create response: %v", rsp)
	}
	id, err := strconv.Atoi(values[0])
	if err != nil {
		t.Fatal(err)
	}
	send := goipp.NewRequest(ipp11, goipp.OpSendDocument, 2)
	send.Operation.Add(goipp.MakeAttr("job-id", goipp.TagInteger, goipp.Integer(id)))
	send.Operation.Add(goipp.MakeAttr("last-document", goipp.TagBoolean, goipp.Boolean(true)))
	rsp = serveIPP(send, []byte("%PDF-1.4"), testPrinter(), shared, "pc", 631)
	waitIPPJob(t, shared, uint32(id))
	opts := shared.Spool.(*fakeSpooler).lastOptions
	if goipp.Status(rsp.Code) != goipp.StatusOk || !opts.Monochrome || opts.IncludesPage(1) || !opts.IncludesPage(2) || !opts.IncludesPage(3) || opts.IncludesPage(4) {
		t.Fatalf("status %s, options %+v", goipp.Status(rsp.Code), opts)
	}
}
