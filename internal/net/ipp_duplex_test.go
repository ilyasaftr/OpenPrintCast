package net

import (
	"slices"
	"testing"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// duplexTestShared serves a driver that does, or does not, offer duplex.
func duplexTestShared(duplex bool) (*SharedState, *fakeSpooler) {
	shared := testShared()
	base := &fakeSpooler{}
	if duplex {
		shared.SetSpooler(&duplexSpooler{&mediaSpooler{base}})
	} else {
		shared.SetSpooler(&mediaSpooler{base})
	}
	return shared, base
}

// sidesRequest asks for the given sides keyword with ipp-attribute-fidelity,
// so an unsupported value fails the request.
func sidesRequest(op goipp.Op, requestID uint32, keyword string) *goipp.Message {
	req := goipp.NewRequest(ipp11, op, requestID)
	req.Operation.Add(goipp.MakeAttr("ipp-attribute-fidelity", goipp.TagBoolean, goipp.Boolean(true)))
	req.Job.Add(goipp.MakeAttr("sides", goipp.TagKeyword, goipp.String(keyword)))
	return req
}

func TestSidesAttributesFollowDuplexProvider(t *testing.T) {
	for _, tc := range []struct {
		name      string
		duplex    bool
		supported []string
	}{
		{"duplex driver", true, []string{"one-sided", "two-sided-long-edge", "two-sided-short-edge"}},
		{"one-sided driver", false, []string{"one-sided"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shared, _ := duplexTestShared(tc.duplex)
			rsp := serveIPP(goipp.NewRequest(ipp11, goipp.OpGetPrinterAttributes, 1), nil, testPrinter(), shared, "pc", 631)
			if got := attrValues(t, rsp.Printer, "sides-supported"); !slices.Equal(got, tc.supported) {
				t.Errorf("sides-supported = %v, want %v", got, tc.supported)
			}
			// The driver default is never read: one-sided stays the default.
			if got := attrValues(t, rsp.Printer, "sides-default"); !slices.Equal(got, []string{"one-sided"}) {
				t.Errorf("sides-default = %v", got)
			}
			if !slices.Contains(attrValues(t, rsp.Printer, "job-creation-attributes-supported"), "sides") {
				t.Error("sides missing from job-creation-attributes-supported")
			}
		})
	}
}

func TestSidesWithoutProviderStayOneSided(t *testing.T) {
	rsp := printerAttrsResponse(goipp.NewRequest(ipp11, goipp.OpGetPrinterAttributes, 1), testPrinter(), nil, "pc", 631)
	if got := attrValues(t, rsp.Printer, "sides-supported"); !slices.Equal(got, []string{"one-sided"}) {
		t.Fatalf("sides-supported = %v", got)
	}
}

func TestSidesJobOptionAcceptance(t *testing.T) {
	for _, op := range []goipp.Op{goipp.OpPrintJob, goipp.OpCreateJob, goipp.OpValidateJob} {
		for _, tc := range []struct {
			keyword string
			duplex  bool
			want    goipp.Status
		}{
			{"one-sided", true, goipp.StatusOk},
			{"two-sided-long-edge", true, goipp.StatusOk},
			{"two-sided-short-edge", true, goipp.StatusOk},
			{"one-sided", false, goipp.StatusOk},
			{"two-sided-long-edge", false, goipp.StatusErrorAttributesOrValues},
			{"two-sided-short-edge", false, goipp.StatusErrorAttributesOrValues},
			{"two-sided-bogus", true, goipp.StatusErrorAttributesOrValues},
			{"", true, goipp.StatusErrorAttributesOrValues},
		} {
			shared, _ := duplexTestShared(tc.duplex)
			rsp := serveIPP(sidesRequest(op, 1, tc.keyword), []byte("%PDF-1.4"), testPrinter(), shared, "pc", 631)
			if got := goipp.Status(rsp.Code); got != tc.want {
				t.Errorf("%s sides=%q duplex=%t: status %s, want %s", op, tc.keyword, tc.duplex, got, tc.want)
			}
			if tc.want == goipp.StatusOk && op != goipp.OpValidateJob {
				settleSidesJob(t, shared, op, rsp)
			}
		}
	}
}

// settleSidesJob lets an accepted job finish, so no goroutine outlives the
// test.
func settleSidesJob(t *testing.T, shared *SharedState, op goipp.Op, rsp *goipp.Message) {
	t.Helper()
	id := responseJobID(t, rsp)
	if op == goipp.OpCreateJob {
		shared.cancelIPPJob("hp", id)
		return
	}
	waitIPPJob(t, shared, id)
}

func TestSidesRejectsMalformedValues(t *testing.T) {
	shared, _ := duplexTestShared(true)
	for name, attr := range map[string]goipp.Attribute{
		"name tag": goipp.MakeAttr("sides", goipp.TagName, goipp.String("two-sided-long-edge")),
		"two values": goipp.MakeAttr("sides", goipp.TagKeyword, goipp.String("one-sided"),
			goipp.String("two-sided-long-edge")),
	} {
		req := goipp.NewRequest(ipp11, goipp.OpValidateJob, 1)
		req.Operation.Add(goipp.MakeAttr("ipp-attribute-fidelity", goipp.TagBoolean, goipp.Boolean(true)))
		req.Job.Add(attr)
		rsp := serveIPP(req, nil, testPrinter(), shared, "pc", 631)
		if goipp.Status(rsp.Code) != goipp.StatusErrorAttributesOrValues {
			t.Errorf("%s: status %s", name, goipp.Status(rsp.Code))
		}
	}
}

func TestSidesReachTheSpooler(t *testing.T) {
	for keyword, want := range map[string]core.Sides{
		"one-sided":            core.OneSided,
		"two-sided-long-edge":  core.TwoSidedLongEdge,
		"two-sided-short-edge": core.TwoSidedShortEdge,
	} {
		t.Run("Print-Job "+keyword, func(t *testing.T) {
			shared, spooler := duplexTestShared(true)
			rsp := serveIPP(sidesRequest(goipp.OpPrintJob, 1, keyword), []byte("%PDF-1.4"), testPrinter(), shared, "pc", 631)
			waitIPPJob(t, shared, responseJobID(t, rsp))
			if got := spooler.lastOptions.Sides; got != want {
				t.Fatalf("spooled sides = %v, want %v", got, want)
			}
		})
		t.Run("Create-Job "+keyword, func(t *testing.T) {
			shared, spooler := duplexTestShared(true)
			rsp := serveIPP(sidesRequest(goipp.OpCreateJob, 1, keyword), nil, testPrinter(), shared, "pc", 631)
			if got := attrValues(t, rsp.Job, "sides"); !slices.Equal(got, []string{keyword}) {
				t.Fatalf("job attributes echo sides = %v", got)
			}
			id := responseJobID(t, rsp)
			send := goipp.NewRequest(ipp11, goipp.OpSendDocument, 2)
			send.Operation.Add(goipp.MakeAttr("job-id", goipp.TagInteger, goipp.Integer(id)))
			send.Operation.Add(goipp.MakeAttr("last-document", goipp.TagBoolean, goipp.Boolean(true)))
			serveIPP(send, []byte("%PDF-1.4"), testPrinter(), shared, "pc", 631)
			waitIPPJob(t, shared, id)
			if got := spooler.lastOptions.Sides; got != want {
				t.Fatalf("spooled sides = %v, want %v", got, want)
			}
		})
	}
}

func TestUnsupportedSidesAreIgnoredWithoutFidelity(t *testing.T) {
	shared, spooler := duplexTestShared(false)
	req := goipp.NewRequest(ipp11, goipp.OpPrintJob, 1)
	req.Job.Add(goipp.MakeAttr("sides", goipp.TagKeyword, goipp.String("two-sided-long-edge")))
	rsp := serveIPP(req, []byte("%PDF-1.4"), testPrinter(), shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusOkIgnoredOrSubstituted || len(rsp.Unsupported) != 1 {
		t.Fatalf("status = %s, unsupported = %v", goipp.Status(rsp.Code), rsp.Unsupported)
	}
	waitIPPJob(t, shared, responseJobID(t, rsp))
	if spooler.lastOptions.Sides != core.OneSided {
		t.Fatalf("an ignored sides request reached the spooler: %v", spooler.lastOptions.Sides)
	}
}
