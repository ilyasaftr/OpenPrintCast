package net

import (
	"strings"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// AirPrintMarkers returns descriptive printer attributes.
func AirPrintMarkers(printer core.PrinterInfo) goipp.Attributes {
	var out goipp.Attributes
	display := printer.DisplayName()
	location := strings.TrimSpace(printer.Location)
	if location == "" {
		location = "OpenPrintCast on " + HostLabel()
	}
	out.Add(stringsAttr("printer-info", goipp.TagText, display))
	out.Add(stringsAttr("printer-location", goipp.TagText, location))
	out.Add(stringsAttr("printer-make-and-model", goipp.TagText, display))
	return out
}

// InjectMarkers appends AirPrintMarkers to the response Printer group,
// skipping names already present so inner values are never duplicated.
func InjectMarkers(rsp *goipp.Message, printer core.PrinterInfo) {
	present := make(map[string]bool, len(rsp.Printer))
	for _, a := range rsp.Printer {
		present[a.Name] = true
	}
	for _, m := range AirPrintMarkers(printer) {
		if !present[m.Name] {
			rsp.Printer.Add(m)
		}
	}
}
