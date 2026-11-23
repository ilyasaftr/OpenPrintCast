package net

import (
	"testing"

	"github.com/OpenPrinting/goipp"
)

func TestAirPrintMarkers(t *testing.T) {
	markers := AirPrintMarkers(testPrinter())
	if features := attrValues(t, markers, "ipp-features-supported"); len(features) != 0 {
		t.Fatalf("should not advertise unsupported AirPrint features: %v", features)
	}
	for _, name := range []string{
		"printer-info", "printer-location", "printer-make-and-model",
	} {
		if got := attrValues(t, markers, name); len(got) == 0 {
			t.Errorf("marker %q missing", name)
		}
	}
}

func TestInjectMarkersNoDuplicates(t *testing.T) {
	rsp := goipp.NewResponse(ipp11, goipp.StatusOk, 1)
	rsp.Printer.Add(goipp.MakeAttr("printer-info", goipp.TagText, goipp.String("custom")))
	InjectMarkers(rsp, testPrinter())
	if got := attrValues(t, rsp.Printer, "printer-info"); len(got) != 1 || got[0] != "custom" {
		t.Fatalf("printer-info = %v, want existing [custom] untouched", got)
	}
	if got := attrValues(t, rsp.Printer, "printer-location"); len(got) != 1 {
		t.Fatalf("printer-location injected %d times, want 1", len(got))
	}
}
