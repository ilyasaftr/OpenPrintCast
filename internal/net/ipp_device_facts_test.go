package net

import (
	"testing"

	"github.com/OpenPrinting/goipp"
)

// attrText is the first text value of the named attribute, or "".
func attrText(attrs goipp.Attributes, name string) string {
	for _, attr := range attrs {
		if attr.Name == name && len(attr.Values) > 0 {
			return attr.Values[0].V.String()
		}
	}
	return ""
}

func TestDeviceFactsAbsentWithoutData(t *testing.T) {
	rsp := printerAttrsResponse(goipp.NewRequest(ipp11, goipp.OpGetPrinterAttributes, 1), testPrinter(), nil, "pc", 631)
	if attrText(rsp.Printer, "printer-serial-number") != "" {
		t.Error("serial number appeared without device data")
	}
	if attrText(rsp.Printer, "printer-firmware-string-version") != "" {
		t.Error("firmware version appeared without device data")
	}
}

func TestDeviceFactsPublished(t *testing.T) {
	c := &printerAttrsContext{printer: testPrinter(), serial: "X1A2B3", firmware: "RF11I5"}
	var attrs goipp.Attributes
	addDeviceFacts(c, &attrs)
	if got := attrText(attrs, "printer-serial-number"); got != "X1A2B3" {
		t.Errorf("serial: got %q", got)
	}
	if got := attrText(attrs, "printer-firmware-string-version"); got != "RF11I5" {
		t.Errorf("firmware: got %q", got)
	}
	if got := attrText(attrs, "printer-firmware-name"); got != "Main" {
		t.Errorf("firmware name: got %q", got)
	}
}
