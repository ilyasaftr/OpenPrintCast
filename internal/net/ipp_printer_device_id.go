package net

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/OpenPrinting/goipp"
)

const (
	// maxIPPTextOctets is the IPP limit for a text value.
	maxIPPTextOctets = 1023
	// genericManufacturer is used when the Windows printer name does not
	// start with a manufacturer the bridge recognizes.
	genericManufacturer = "Generic"
	genericModel        = "Printer"
)

// deviceIDCommandSets name the advertised document formats the way IEEE
// 1284 CMD fields and CUPS's driverless device IDs do (PWG 5100.13 uses
// "PWGRaster"; Apple raster is "URF").
var deviceIDCommandSets = []string{"PDF", "URF", "PWGRaster", "JPEG", "PNG"}

func addDeviceID(c *printerAttrsContext, attrs *goipp.Attributes) {
	attrs.Add(stringsAttr("printer-device-id", goipp.TagText, printerDeviceID(c.printer.ModelName())))
}

// printerDeviceID builds an IEEE 1284 device ID from the Windows printer
// name, the only identity the bridge has.
func printerDeviceID(windowsName string) string {
	manufacturer, model := splitManufacturer(deviceIDValue(windowsName))
	id := formatDeviceID(manufacturer, model)
	if overflow := len(id) - maxIPPTextOctets; overflow > 0 {
		// Only the model can be long; shorten it to fit.
		id = formatDeviceID(manufacturer, truncateUTF8(model, len(model)-overflow))
	}
	return id
}

func formatDeviceID(manufacturer, model string) string {
	return "MFG:" + manufacturer + ";MDL:" + model + ";CMD:" + strings.Join(deviceIDCommandSets, ",") + ";CLS:PRINTER;"
}

// splitManufacturer takes a recognized manufacturer off the front of the
// name. A product line (see manufacturerRules) keeps its words in the model.
func splitManufacturer(name string) (manufacturer, model string) {
	if name == "" {
		return genericManufacturer, genericModel
	}
	rule, words, ok := matchManufacturer(name)
	if !ok {
		return genericManufacturer, name
	}
	if rule.keepPrefix {
		return rule.manufacturer, name
	}
	rest := strings.Join(strings.Fields(name)[words:], " ")
	if rest == "" {
		return genericManufacturer, name
	}
	return rule.manufacturer, rest
}

// deviceIDValue removes the device ID delimiters and control characters
// and collapses runs of white space.
func deviceIDValue(s string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r == ';' || r == ':':
			return -1
		case r == utf8.RuneError || unicode.IsControl(r):
			return ' '
		}
		return r
	}, s)
	return strings.Join(strings.Fields(cleaned), " ")
}

// truncateUTF8 cuts s to at most limit bytes without splitting a rune.
func truncateUTF8(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if len(s) <= limit {
		return s
	}
	for limit > 0 && !utf8.RuneStart(s[limit]) {
		limit--
	}
	return strings.TrimSpace(s[:limit])
}
