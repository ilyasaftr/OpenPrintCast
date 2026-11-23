package net

import (
	"slices"
	"testing"

	"github.com/OpenPrinting/goipp"
)

func TestPrintKitReadyMediaAndFinishings(t *testing.T) {
	shared := testShared()
	req := goipp.NewRequest(goipp.MakeVersion(2, 0), goipp.OpGetPrinterAttributes, 1)
	req.Operation.Add(goipp.MakeAttr("requested-attributes", goipp.TagKeyword,
		goipp.String("media-col-ready"), goipp.String("media-source-supported"),
		goipp.String("media-type-supported"), goipp.String("finishings-supported")))
	rsp := serveIPP(req, nil, testPrinter(), shared, "pc", 631)
	wire, err := rsp.EncodeBytes()
	if err != nil {
		t.Fatal(err)
	}
	var decoded goipp.Message
	if err := decoded.DecodeBytes(wire); err != nil {
		t.Fatal(err)
	}
	rsp = &decoded
	if got := attrValues(t, rsp.Printer, "finishings-supported"); len(got) != 1 || got[0] != "3" {
		t.Fatalf("finishings-supported = %v", got)
	}
	var ready goipp.Collection
	for _, attr := range rsp.Printer {
		if attr.Name == "media-col-ready" && len(attr.Values) == 1 {
			ready, _ = attr.Values[0].V.(goipp.Collection)
		}
	}
	if opAttrString(goipp.Attributes(ready), "media-source") != "auto" ||
		opAttrString(goipp.Attributes(ready), "media-type") != "auto" {
		t.Fatalf("ready media lacks automatic source or type: %v", ready)
	}

	validate := goipp.NewRequest(goipp.MakeVersion(2, 0), goipp.OpValidateJob, 2)
	validate.Operation.Add(goipp.MakeAttr("ipp-attribute-fidelity", goipp.TagBoolean, goipp.Boolean(true)))
	validate.Job.Add(goipp.MakeAttr("media-col", goipp.TagBeginCollection, ready))
	validate.Job.Add(goipp.MakeAttr("finishings", goipp.TagEnum, goipp.Integer(3)))
	if result := serveIPP(validate, nil, testPrinter(), shared, "pc", 631); goipp.Status(result.Code) != goipp.StatusOk {
		t.Fatalf("advertised ready media rejected: %s", goipp.Status(result.Code))
	}
	validate.Job[1] = goipp.MakeAttr("finishings", goipp.TagEnum, goipp.Integer(20))
	if result := serveIPP(validate, nil, testPrinter(), shared, "pc", 631); goipp.Status(result.Code) != goipp.StatusErrorAttributesOrValues {
		t.Fatalf("unsupported finishing accepted: %s", goipp.Status(result.Code))
	}
}

func TestDriverlessCapabilities(t *testing.T) {
	shared := testShared()
	req := goipp.NewRequest(ipp20, goipp.OpGetPrinterAttributes, 1)
	rsp := serveIPP(req, nil, testPrinter(), shared, "pc", 631)
	for name, want := range map[string][]string{
		"printer-kind":                {"document"},
		"document-password-supported": {"0"},
		"print-scaling-default":       {"fit"},
		"print-scaling-supported":     {"fit"},
		"jpeg-features-supported":     {"progressive"},
		"jpeg-k-octets-supported":     {"0-262144"},
		"jpeg-x-dimension-supported":  {"0-65535"},
		"jpeg-y-dimension-supported":  {"1-65535"},
		"pdf-k-octets-supported":      {"0-262144"},
	} {
		if got := attrValues(t, rsp.Printer, name); !slices.Equal(got, want) {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	if got := attrValues(t, rsp.Printer, "printer-mandatory-job-attributes"); len(got) != 0 {
		t.Errorf("no mandatory job attributes expected: %v", got)
	}
	validate := goipp.NewRequest(ipp20, goipp.OpValidateJob, 2)
	validate.Operation.Add(goipp.MakeAttr("ipp-attribute-fidelity", goipp.TagBoolean, goipp.Boolean(true)))
	validate.Job.Add(goipp.MakeAttr("print-scaling", goipp.TagKeyword, goipp.String("fit")))
	if result := serveIPP(validate, nil, testPrinter(), shared, "pc", 631); goipp.Status(result.Code) != goipp.StatusOk {
		t.Fatalf("fit scaling rejected: %s", goipp.Status(result.Code))
	}
	validate.Job[0] = goipp.MakeAttr("print-scaling", goipp.TagKeyword, goipp.String("fill"))
	if result := serveIPP(validate, nil, testPrinter(), shared, "pc", 631); goipp.Status(result.Code) != goipp.StatusErrorAttributesOrValues {
		t.Fatalf("unsupported fill scaling accepted: %s", goipp.Status(result.Code))
	}
	validate.Job = nil
	validate.Operation.Add(goipp.MakeAttr("document-password", goipp.TagString, goipp.Binary("secret")))
	if result := serveIPP(validate, nil, testPrinter(), shared, "pc", 631); goipp.Status(result.Code) != goipp.StatusErrorAttributesOrValues {
		t.Fatalf("unsupported document password accepted: %s", goipp.Status(result.Code))
	}
}

func TestPrintKitAutomaticInputSource(t *testing.T) {
	req := goipp.NewRequest(ipp20, goipp.OpGetPrinterAttributes, 1)
	req.Operation.Add(goipp.MakeAttr("requested-attributes", goipp.TagKeyword,
		goipp.String("printer-state-reasons"), goipp.String("media-source-supported"),
		goipp.String("printer-input-tray"), goipp.String("printer-mandatory-job-attributes")))
	rsp := serveIPP(req, nil, testPrinter(), testShared(), "pc", 631)
	wire, err := rsp.EncodeBytes()
	if err != nil {
		t.Fatal(err)
	}
	var decoded goipp.Message
	if err := decoded.DecodeBytes(wire); err != nil {
		t.Fatal(err)
	}
	want := "type=other;mediafeed=-2;mediaxfeed=-2;maxcapacity=-2;level=-2;status=5;name=auto;"
	for _, attr := range decoded.Printer {
		if attr.Name == "printer-input-tray" {
			if len(attr.Values) != 1 || attr.Values[0].T != goipp.TagString {
				t.Fatalf("automatic input source = %v", attr.Values)
			}
			value, ok := attr.Values[0].V.(goipp.Binary)
			if !ok || string(value) != want {
				t.Fatalf("automatic input source = %v", attr.Values)
			}
			return
		}
	}
	t.Fatal("missing automatic input source")
}

func TestPrinterDNSSDNameTracksAdvertisement(t *testing.T) {
	printer := testPrinter()
	printer.AirPrintName = "Office (PDF). Printer"
	adv := &Advertiser{
		host:       "pc",
		registered: map[string]bool{printer.ID: true},
		instances:  map[string]string{printer.ID: printer.AirPrintName},
		names:      make(map[string]int),
	}
	shared := testShared()
	shared.instanceName = adv.advertisedName
	req := goipp.NewRequest(ipp20, goipp.OpGetPrinterAttributes, 1)
	check := func(want string) {
		t.Helper()
		rsp := serveIPP(req, nil, printer, shared, "pc", 631)
		if got := attrValues(t, rsp.Printer, "printer-dns-sd-name"); len(got) != 1 || got[0] != want {
			t.Fatalf("printer-dns-sd-name = %v, want %q", got, want)
		}
	}
	check(printer.AirPrintName)
	adv.names[printer.ID] = 1
	check(printer.AirPrintName)
	conflictName := printer.AirPrintName + " (" + PrinterUUID(printer.ID, "pc")[:8] + ")"
	adv.instances[printer.ID] = conflictName
	check(conflictName)
	delete(adv.registered, printer.ID)
	rsp := serveIPP(req, nil, printer, shared, "pc", 631)
	if got := attrValues(t, rsp.Printer, "printer-dns-sd-name"); len(got) != 0 {
		t.Fatalf("unregistered printer name = %v", got)
	}
}

func TestPrinterDefaultsRequestedGroup(t *testing.T) {
	req := goipp.NewRequest(goipp.MakeVersion(2, 0), goipp.OpGetPrinterAttributes, 1)
	req.Operation.Add(goipp.MakeAttr("requested-attributes", goipp.TagKeyword,
		goipp.String("printer-defaults"), goipp.String("printer-state")))
	rsp := serveIPP(req, nil, testPrinter(), testShared(), "pc", 631)
	for _, name := range []string{"media-default", "media-col-default", "printer-resolution-default", "copies-default", "printer-state"} {
		if len(attrValues(t, rsp.Printer, name)) != 1 {
			t.Errorf("requested group missing %s", name)
		}
	}
	if len(attrValues(t, rsp.Printer, "media-supported")) != 0 {
		t.Error("unrequested supported attribute was included")
	}
}

func TestDefaultQualityAndUnknownSpeed(t *testing.T) {
	shared := testShared()
	shared.SetSpooler(&unknownCapabilitiesSpooler{fakeSpooler: &fakeSpooler{}})
	req := goipp.NewRequest(goipp.MakeVersion(2, 0), goipp.OpGetPrinterAttributes, 1)
	rsp := serveIPP(req, nil, testPrinter(), shared, "pc", 631)
	wire, err := rsp.EncodeBytes()
	if err != nil {
		t.Fatal(err)
	}
	var decoded goipp.Message
	if err := decoded.DecodeBytes(wire); err != nil {
		t.Fatal(err)
	}
	rsp = &decoded
	if got := attrValues(t, rsp.Printer, "print-quality-supported"); len(got) != 1 || got[0] != "4" {
		t.Fatalf("quality support = %v, want the bridge default only", got)
	}
	for _, name := range []string{"pages-per-minute", "pages-per-minute-color"} {
		found := false
		for _, attr := range rsp.Printer {
			if attr.Name == name && len(attr.Values) == 1 && attr.Values[0].T == goipp.TagUnknown {
				found = true
			}
		}
		if !found {
			t.Errorf("%s must report unknown rather than an invented speed", name)
		}
	}
	req = goipp.NewRequest(goipp.MakeVersion(2, 0), goipp.OpPrintJob, 2)
	req.Operation.Add(goipp.MakeAttr("ipp-attribute-fidelity", goipp.TagBoolean, goipp.Boolean(true)))
	req.Job.Add(goipp.MakeAttr("print-quality", goipp.TagEnum, goipp.Integer(4)))
	rsp = serveIPP(req, []byte("%PDF-1.4"), testPrinter(), shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusOk {
		t.Fatalf("default quality print status = %s", goipp.Status(rsp.Code))
	}
	waitIPPJob(t, shared, responseJobID(t, rsp))
	if got := shared.Spool.(*unknownCapabilitiesSpooler).lastOptions.Quality; got != 0 {
		t.Fatalf("default quality should preserve driver settings, got %d", got)
	}
}

type unknownCapabilitiesSpooler struct{ *fakeSpooler }

func (*unknownCapabilitiesSpooler) PagesPerMinute(string) (int, bool) { return 0, false }

func (*unknownCapabilitiesSpooler) ColorPagesPerMinute(string) (int, bool) { return 0, false }

func TestPrinterAttributesReflectWindowsStatus(t *testing.T) {
	shared := testShared()
	spool := shared.Spool.(*fakeSpooler)
	spool.reportedState = 5
	spool.reportedReasons = []string{"media-empty", "media-needed"}
	req := goipp.NewRequest(ipp11, goipp.OpGetPrinterAttributes, 1)
	rsp := serveIPP(req, nil, testPrinter(), shared, "pc", 631)
	if got := attrValues(t, rsp.Printer, "pages-per-minute"); len(got) != 1 || got[0] != "18" {
		t.Fatalf("driver speed = %v", got)
	}
	if got := attrValues(t, rsp.Printer, "printer-state"); len(got) != 1 || got[0] != "5" {
		t.Fatalf("printer state = %v", got)
	}
	if got := attrValues(t, rsp.Printer, "printer-state-reasons"); len(got) != 2 || got[1] != "media-needed" {
		t.Fatalf("printer reasons = %v", got)
	}
	shared.SetPaused(true)
	rsp = serveIPP(req, nil, testPrinter(), shared, "pc", 631)
	if got := attrValues(t, rsp.Printer, "printer-state-reasons"); len(got) != 3 || got[2] != "paused" {
		t.Fatalf("paused reasons = %v", got)
	}
}

func TestMonochromeCapabilities(t *testing.T) {
	printer := testPrinter()
	mono := false
	printer.ColorCapable = &mono
	req := goipp.NewRequest(ipp11, goipp.OpGetPrinterAttributes, 1)
	rsp := printerAttrsResponse(req, printer, testShared(), "pc", 631)
	if got := attrValues(t, rsp.Printer, "color-supported"); len(got) != 1 || got[0] != "false" {
		t.Fatalf("color-supported = %v", got)
	}
	for _, v := range attrValues(t, rsp.Printer, "print-color-mode-supported") {
		if v == "color" {
			t.Fatal("monochrome printer advertised color mode")
		}
	}
}

func TestPrinterAdvertisesPlainIPPWithoutTLS(t *testing.T) {
	rsp := printerAttrsResponse(goipp.NewRequest(ipp11, goipp.OpGetPrinterAttributes, 1), testPrinter(), testShared(), "pc", 631)
	if got := attrValues(t, rsp.Printer, "uri-security-supported"); len(got) != 1 || got[0] != "none" {
		t.Fatalf("uri-security-supported = %v, want none", got)
	}
	if got := attrValues(t, rsp.Printer, "printer-uri-supported"); len(got) != 1 || got[0] != "ipp://pc.local:631/ipp/print/hp" {
		t.Fatalf("printer-uri-supported = %v", got)
	}
}
