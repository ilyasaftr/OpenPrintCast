package net

import (
	"slices"
	"testing"

	"github.com/OpenPrinting/goipp"
)

func TestMediaCollectionUsesWindowsDefault(t *testing.T) {
	for _, tc := range []struct {
		width int
		want  goipp.Status
	}{{21000, goipp.StatusOk}, {21590, goipp.StatusErrorAttributesOrValues}} {
		req := goipp.NewRequest(ipp11, goipp.OpValidateJob, 1)
		req.Operation.Add(goipp.MakeAttr("ipp-attribute-fidelity", goipp.TagBoolean, goipp.Boolean(true)))
		req.Job.Add(goipp.MakeAttrCollection("media-col",
			goipp.MakeAttrCollection("media-size",
				goipp.MakeAttr("x-dimension", goipp.TagInteger, goipp.Integer(tc.width)),
				goipp.MakeAttr("y-dimension", goipp.TagInteger, goipp.Integer(29700)))))
		rsp := serveIPP(req, nil, testPrinter(), testShared(), "pc", 631)
		if goipp.Status(rsp.Code) != tc.want {
			t.Fatalf("width %d: got %s, want %s", tc.width, goipp.Status(rsp.Code), tc.want)
		}
	}
}

func TestPrinterReportsDriverMedia(t *testing.T) {
	shared := testShared()
	attrs := mediaAttrs(shared, testPrinter())
	for name, want := range map[string]string{
		"media-left-margin-supported": "300", "media-bottom-margin-supported": "400",
		"media-ready": "iso_a4_210x297mm", "printer-resolution-default": "600x600dpi",
	} {
		got := attrValues(t, attrs, name)
		if len(got) != 1 || got[0] != want {
			t.Fatalf("%s = %v, want %s", name, got, want)
		}
	}
	for _, name := range []string{"media-col-default", "media-col-database", "media-col-ready", "media-size-supported"} {
		if got := attrValues(t, attrs, name); len(got) != 1 {
			t.Fatalf("%s = %v", name, got)
		}
	}
	req := goipp.NewRequest(ipp11, goipp.OpValidateJob, 1)
	req.Operation.Add(goipp.MakeAttr("ipp-attribute-fidelity", goipp.TagBoolean, goipp.Boolean(true)))
	req.Job.Add(goipp.MakeAttrCollection("media-col",
		goipp.MakeAttrCollection("media-size",
			goipp.MakeAttr("x-dimension", goipp.TagInteger, goipp.Integer(21000)),
			goipp.MakeAttr("y-dimension", goipp.TagInteger, goipp.Integer(29700))),
		goipp.MakeAttr("media-left-margin", goipp.TagInteger, goipp.Integer(0))))
	rsp := serveIPP(req, nil, testPrinter(), shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusErrorAttributesOrValues {
		t.Fatalf("unsupported borderless request status = %s", goipp.Status(rsp.Code))
	}
}

func TestDriverSupportedMediaIsSelectedForJobs(t *testing.T) {
	shared := testShared()
	spool := &mediaSpooler{&fakeSpooler{}}
	shared.SetSpooler(spool)
	printer := testPrinter()
	attrs := mediaAttrs(shared, printer)
	if got := attrValues(t, attrs, "media-supported"); !slices.Contains(got, "na_letter_8.5x11in") {
		t.Fatalf("Letter not advertised: %v", got)
	}
	rsp := printerAttrsResponse(goipp.NewRequest(ipp11, goipp.OpGetPrinterAttributes, 1), printer, shared, "pc", 631)
	wire, err := rsp.EncodeBytes()
	if err != nil {
		t.Fatal(err)
	}
	var decoded goipp.Message
	if err := decoded.DecodeBytes(wire); err != nil {
		t.Fatal(err)
	}
	if got := attrValues(t, decoded.Printer, "media-col-database"); len(got) != 2 {
		t.Fatalf("media collections after IPP round-trip: %v", got)
	}
	for _, media := range []goipp.Attribute{
		goipp.MakeAttr("media", goipp.TagKeyword, goipp.String("na_letter_8.5x11in")),
		goipp.MakeAttrCollection("media-col",
			goipp.MakeAttrCollection("media-size",
				goipp.MakeAttr("x-dimension", goipp.TagInteger, goipp.Integer(21590)),
				goipp.MakeAttr("y-dimension", goipp.TagInteger, goipp.Integer(27940))),
			goipp.MakeAttr("media-left-margin", goipp.TagInteger, goipp.Integer(200))),
	} {
		req := goipp.NewRequest(ipp11, goipp.OpPrintJob, 1)
		req.Operation.Add(goipp.MakeAttr("ipp-attribute-fidelity", goipp.TagBoolean, goipp.Boolean(true)))
		req.Job.Add(media)
		response := serveIPP(req, []byte("%PDF-1.4"), printer, shared, "pc", 631)
		if status := goipp.Status(response.Code); status != goipp.StatusOk {
			t.Fatalf("%s rejected: %s", media.Name, status)
		}
		waitIPPJob(t, shared, responseJobID(t, response))
		spool.mu.Lock()
		paper := spool.lastOptions.PaperID
		spool.mu.Unlock()
		if paper != 1 {
			t.Fatalf("%s selected Windows paper ID %d, want 1", media.Name, paper)
		}
	}
	req := goipp.NewRequest(ipp11, goipp.OpValidateJob, 1)
	req.Operation.Add(goipp.MakeAttr("ipp-attribute-fidelity", goipp.TagBoolean, goipp.Boolean(true)))
	req.Job.Add(goipp.MakeAttr("media", goipp.TagKeyword, goipp.String("na_legal_8.5x14in")))
	if got := goipp.Status(serveIPP(req, nil, printer, shared, "pc", 631).Code); got != goipp.StatusErrorAttributesOrValues {
		t.Fatalf("unsupported paper accepted: %s", got)
	}
}

func TestSupportedMediaWithUnknownDefault(t *testing.T) {
	shared := testShared()
	spool := &unknownDefaultMediaSpooler{&mediaSpooler{&fakeSpooler{}}}
	shared.SetSpooler(spool)
	printer := testPrinter()
	attrs := mediaAttrs(shared, printer)
	if got := attrValues(t, attrs, "media-supported"); !slices.Equal(got, []string{"iso_a4_210x297mm", "na_letter_8.5x11in"}) {
		t.Fatalf("supported media = %v", got)
	}
	for _, name := range []string{"media-default", "media-ready", "media-col-ready", "printer-resolution-default"} {
		if got := attrValues(t, attrs, name); len(got) != 0 {
			t.Fatalf("%s must not be invented: %v", name, got)
		}
	}
	if got := attrValues(t, attrs, "media-col-database"); len(got) != 2 {
		t.Fatalf("media collections = %v", got)
	}
	foundUnknown := false
	for _, attr := range attrs {
		if attr.Name == "media-col-default" {
			if len(attr.Values) != 1 || attr.Values[0].T != goipp.TagUnknown {
				t.Fatalf("unknown default = %v", attr)
			}
			foundUnknown = true
		}
	}
	if !foundUnknown {
		t.Fatal("unknown media-col-default was not advertised")
	}
	req := goipp.NewRequest(ipp11, goipp.OpPrintJob, 1)
	req.Operation.Add(goipp.MakeAttr("ipp-attribute-fidelity", goipp.TagBoolean, goipp.Boolean(true)))
	req.Job.Add(goipp.MakeAttr("media", goipp.TagKeyword, goipp.String("iso_a4_210x297mm")))
	rsp := serveIPP(req, []byte("%PDF-1.4"), printer, shared, "pc", 631)
	if status := goipp.Status(rsp.Code); status != goipp.StatusOk {
		t.Fatalf("validated media rejected: %s", status)
	}
	waitIPPJob(t, shared, responseJobID(t, rsp))
	spool.mu.Lock()
	paper := spool.lastOptions.PaperID
	spool.mu.Unlock()
	if paper != 9 {
		t.Fatalf("selected Windows paper ID %d, want 9", paper)
	}
}

func TestMacOSLetterFallbackAgainstA4Driver(t *testing.T) {
	shared := testShared()
	get := goipp.NewRequest(ipp11, goipp.OpGetPrinterAttributes, 1)
	get.Operation.Add(goipp.MakeAttr("requested-attributes", goipp.TagKeyword,
		goipp.String("all"), goipp.String("media-col-database")))
	rsp := serveIPP(get, nil, testPrinter(), shared, "pc", 631)
	if len(attrValues(t, rsp.Printer, "media-col-database")) != 1 ||
		opAttrString(rsp.Printer, "media-default") != "iso_a4_210x297mm" {
		t.Fatalf("missing A4 media database: %v", rsp.Printer)
	}
	for _, tc := range []struct {
		media    string
		fidelity bool
		want     goipp.Status
	}{
		{"iso_a4_210x297mm", true, goipp.StatusOk},
		{"na_letter_8.5x11in", false, goipp.StatusOkIgnoredOrSubstituted},
		{"na_letter_8.5x11in", true, goipp.StatusErrorAttributesOrValues},
	} {
		req := goipp.NewRequest(ipp11, goipp.OpValidateJob, 2)
		req.Operation.Add(goipp.MakeAttr("ipp-attribute-fidelity", goipp.TagBoolean, goipp.Boolean(tc.fidelity)))
		req.Job.Add(goipp.MakeAttr("media", goipp.TagKeyword, goipp.String(tc.media)))
		rsp = serveIPP(req, nil, testPrinter(), shared, "pc", 631)
		if goipp.Status(rsp.Code) != tc.want {
			t.Errorf("media %s fidelity %t: got %s, want %s", tc.media, tc.fidelity, goipp.Status(rsp.Code), tc.want)
		}
	}
}

func TestDefaultMediaMatchesPrinter(t *testing.T) {
	attrs := mediaAttrs(testShared(), testPrinter())
	if got := attrValues(t, attrs, "media-default"); len(got) != 1 || got[0] != "iso_a4_210x297mm" {
		t.Fatalf("media-default = %v", got)
	}
	if len(attrValues(t, attrs, "media-supported")) != 1 {
		t.Fatal("only the Windows driver default should be advertised")
	}
}
