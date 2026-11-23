package net

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/OpenPrinting/goipp"
)

func TestPrinterDeviceID(t *testing.T) {
	const tail = ";CMD:PDF,URF,PWGRaster,JPEG,PNG;CLS:PRINTER;"
	for _, tc := range []struct{ name, want string }{
		{"HP LaserJet", "MFG:HP;MDL:LaserJet" + tail},
		{"hp LaserJet Pro", "MFG:HP;MDL:LaserJet Pro" + tail},
		{"Microsoft Print to PDF", "MFG:Microsoft;MDL:Print to PDF" + tail},
		{"Acme Printer 5", "MFG:Generic;MDL:Acme Printer 5" + tail},
		{"HP", "MFG:Generic;MDL:HP" + tail},
		{"Canon; MF:6000", "MFG:Canon;MDL:MF6000" + tail},
		{"Office\tPrinter\n2", "MFG:Generic;MDL:Office Printer 2" + tail},
		{"", "MFG:Generic;MDL:Printer" + tail},
		{" ;: ", "MFG:Generic;MDL:Printer" + tail},
	} {
		if got := printerDeviceID(tc.name); got != tc.want {
			t.Errorf("printerDeviceID(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestPrinterDeviceIDStaysWithinTextLimit(t *testing.T) {
	// Multi-byte runes must not be split when the model is shortened.
	for _, name := range []string{strings.Repeat("x", 5000), "HP " + strings.Repeat("é", 2000), strings.Repeat("日本 ", 1000)} {
		got := printerDeviceID(name)
		if len(got) > maxIPPTextOctets || len(got) < maxIPPTextOctets-4 {
			t.Errorf("device ID length = %d for %.12q", len(got), name)
		}
		if !utf8.ValidString(got) || !strings.HasSuffix(got, ";CMD:PDF,URF,PWGRaster,JPEG,PNG;CLS:PRINTER;") {
			t.Errorf("malformed device ID for %.12q", name)
		}
	}
}

func TestDeviceIDIsAdvertised(t *testing.T) {
	req := goipp.NewRequest(ipp20, goipp.OpGetPrinterAttributes, 1)
	rsp := serveIPP(req, nil, testPrinter(), testShared(), "pc", 631)
	want := "MFG:HP;MDL:LaserJet;CMD:PDF,URF,PWGRaster,JPEG,PNG;CLS:PRINTER;"
	if got := attrValues(t, rsp.Printer, "printer-device-id"); !slices.Equal(got, []string{want}) {
		t.Fatalf("printer-device-id = %v, want %q", got, want)
	}
	for _, attr := range rsp.Printer {
		if attr.Name == "printer-device-id" && attr.Values[0].T != goipp.TagText {
			t.Fatalf("printer-device-id tag = %v, want text", attr.Values[0].T)
		}
	}
}

func TestPrintRenderingIntent(t *testing.T) {
	shared := testShared()
	req := goipp.NewRequest(ipp20, goipp.OpGetPrinterAttributes, 1)
	rsp := serveIPP(req, nil, testPrinter(), shared, "pc", 631)
	for _, name := range []string{"print-rendering-intent-default", "print-rendering-intent-supported"} {
		if got := attrValues(t, rsp.Printer, name); !slices.Equal(got, []string{"auto"}) {
			t.Errorf("%s = %v, want [auto]", name, got)
		}
	}
	if got := attrValues(t, rsp.Printer, "job-creation-attributes-supported"); !slices.Contains(got, "print-rendering-intent") {
		t.Errorf("job-creation-attributes-supported lacks print-rendering-intent: %v", got)
	}
	for _, tc := range []struct {
		tag   goipp.Tag
		value string
		want  goipp.Status
	}{
		{goipp.TagKeyword, "auto", goipp.StatusOk},
		{goipp.TagKeyword, "perceptual", goipp.StatusErrorAttributesOrValues},
		{goipp.TagName, "auto", goipp.StatusErrorAttributesOrValues},
	} {
		validate := goipp.NewRequest(ipp20, goipp.OpValidateJob, 2)
		validate.Operation.Add(goipp.MakeAttr("ipp-attribute-fidelity", goipp.TagBoolean, goipp.Boolean(true)))
		validate.Job.Add(goipp.MakeAttr("print-rendering-intent", tc.tag, goipp.String(tc.value)))
		if got := goipp.Status(serveIPP(validate, nil, testPrinter(), shared, "pc", 631).Code); got != tc.want {
			t.Errorf("print-rendering-intent %v %q: status %s, want %s", tc.tag, tc.value, got, tc.want)
		}
	}
}
