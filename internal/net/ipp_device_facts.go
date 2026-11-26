package net

import (
	"github.com/OpenPrinting/goipp"
)

// addDeviceFacts publishes what the printer said about itself: the serial
// number (printer-serial-number, registered for IPP Enterprise Printing
// and recommended by IPP Everywhere 2.0) and the firmware version in the
// attributes AirPrint defined and the IANA registry carries. Both appear
// only when the device was read straight — a network printer read over
// SNMP has neither — and the firmware triple keeps the one-to-one
// cardinality the registration requires.
func addDeviceFacts(c *printerAttrsContext, attrs *goipp.Attributes) {
	if c.serial != "" {
		attrs.Add(stringsAttr("printer-serial-number", goipp.TagText, c.serial))
	}
	if c.firmware != "" {
		attrs.Add(stringsAttr("printer-firmware-name", goipp.TagName, "Main"))
		attrs.Add(stringsAttr("printer-firmware-string-version", goipp.TagText, c.firmware))
		attrs.Add(stringsAttr("printer-firmware-patches", goipp.TagText, ""))
	}
	// printer-media-sheets-completed is the device's own sheets-fed
	// counter, published only when the device keeps one: a model without
	// counters must not claim zero sheets.
	if c.pagesKnown {
		attrs.Add(intAttr("printer-media-sheets-completed", c.totalPages))
	}
}
