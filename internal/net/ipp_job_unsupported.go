package net

import (
	"slices"
	"strconv"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// jobOptionContext is what a job-template check may consult.
type jobOptionContext struct {
	req     *goipp.Message
	shared  *SharedState
	printer core.PrinterInfo
}

// unsupportedCheck reports whether a job-template attribute is unsupported.
type unsupportedCheck func(c jobOptionContext, attr goipp.Attribute) bool

// jobOptionChecks holds one check per job-template attribute the bridge
// understands; any other attribute is unsupported.
var jobOptionChecks = map[string]unsupportedCheck{
	"copies":                 checkCopies,
	"sides":                  checkSides,
	"media":                  checkMedia,
	"media-col":              checkMediaCol,
	"number-up":              fixedValue(goipp.TagInteger, "1"),
	"job-hold-until":         fixedValue(goipp.TagKeyword, "no-hold"),
	"job-sheets":             fixedValue(goipp.TagKeyword, "none"),
	"finishings":             fixedValue(goipp.TagEnum, strconv.Itoa(finishingsNone)),
	"print-content-optimize": fixedValue(goipp.TagKeyword, valueAuto),
	"print-scaling":          fixedValue(goipp.TagKeyword, "fit"),
	"print-rendering-intent": fixedValue(goipp.TagKeyword, valueAuto),
	"output-bin":             checkOutputBin,
	"printer-resolution":     checkPrinterResolution,
	"print-quality":          checkPrintQuality,
	"orientation-requested":  checkOrientation,
	"print-color-mode":       checkColorMode,
	"overrides":              checkOverrides,
	"page-ranges": func(jobOptionContext, goipp.Attribute) bool {
		// Parsed by jobOptions.
		return false
	},
}

func unsupportedJobOptions(req *goipp.Message, shared *SharedState, printer core.PrinterInfo) goipp.Attributes {
	ctx := jobOptionContext{req: req, shared: shared, printer: printer}
	var unsupported goipp.Attributes
	seen := make(map[string]bool)
	for _, attr := range req.Job {
		if seen[attr.Name] {
			unsupported.Add(attr)
			continue
		}
		seen[attr.Name] = true
		if check, known := jobOptionChecks[attr.Name]; !known || check(ctx, attr) {
			unsupported.Add(attr)
		}
	}
	return unsupported
}

// fixedValue accepts only one value of the given tag and text.
func fixedValue(tag goipp.Tag, want string) unsupportedCheck {
	return func(_ jobOptionContext, attr goipp.Attribute) bool {
		return !singleValue(attr, tag, want)
	}
}

func checkCopies(c jobOptionContext, _ goipp.Attribute) bool {
	_, ok := jobCopies(c.req)
	return !ok
}

// checkSides accepts one-sided always, and a two-sided keyword only when the
// driver offers duplex.
func checkSides(c jobOptionContext, attr goipp.Attribute) bool {
	if !hasSingleValue(attr, goipp.TagKeyword) {
		return true
	}
	sides, known := core.ParseSides(firstValue(attr))
	return !known || sides != core.OneSided && !duplexSupported(c.shared, c.printer)
}

func checkMedia(c jobOptionContext, attr goipp.Attribute) bool {
	_, ok := mediaChoiceForName(attr, c.shared, c.printer)
	return !ok
}

// checkMediaCol also rejects a media-col that names a different paper than
// a media attribute in the same request.
func checkMediaCol(c jobOptionContext, attr goipp.Attribute) bool {
	choice, ok := mediaChoiceForCollection(attr, c.shared, c.printer)
	if !ok {
		return true
	}
	for _, other := range c.req.Job {
		if other.Name != "media" {
			continue
		}
		named, valid := mediaChoiceForName(other, c.shared, c.printer)
		if valid && named.name != choice.name {
			return true
		}
	}
	return false
}

func checkOutputBin(_ jobOptionContext, attr goipp.Attribute) bool {
	return len(attr.Values) != 1 || !isNameOrKeyword(attr.Values[0].T) || firstValue(attr) != valueAuto
}

// checkPrinterResolution accepts only the driver resolution, and rejects a
// non-default print-quality that the driver ties to it.
func checkPrinterResolution(c jobOptionContext, attr goipp.Attribute) bool {
	resolution := opAttrString(mediaAttrs(c.shared, c.printer), "printer-resolution-supported")
	if len(attr.Values) != 1 || attr.Values[0].T != goipp.TagResolution || resolution == "" || firstValue(attr) != resolution {
		return true
	}
	quality := opAttrString(c.req.Job, "print-quality")
	if quality == "" {
		return false
	}
	provider, ok := capabilitySource(c.shared, c.printer).(printQualityProvider)
	if !ok {
		return false
	}
	supported, def, known := provider.PrintQualities(c.printer.WindowsName)
	requested, err := strconv.Atoi(quality)
	return known && err == nil && slices.Contains(supported, requested) && requested != def
}

func checkPrintQuality(c jobOptionContext, attr goipp.Attribute) bool {
	supported, _, _ := printQualities(c.shared, c.printer)
	quality, parseErr := strconv.Atoi(firstValue(attr))
	return len(attr.Values) != 1 || attr.Values[0].T != goipp.TagEnum || parseErr != nil || !slices.Contains(supported, quality)
}

func checkOrientation(c jobOptionContext, _ goipp.Attribute) bool {
	provider, ok := capabilitySource(c.shared, c.printer).(orientationProvider)
	return !ok || !provider.OrientationSupported(c.printer.WindowsName)
}

func checkColorMode(c jobOptionContext, attr goipp.Attribute) bool {
	mode := firstValue(attr)
	return len(attr.Values) != 1 || attr.Values[0].T != goipp.TagKeyword ||
		!validColorMode(mode) || mode == colorModeColor && !c.printer.SupportsColor()
}

// checkOverrides rejects per-page color on a monochrome printer.
func checkOverrides(c jobOptionContext, _ goipp.Attribute) bool {
	options, valid := jobOptions(c.req)
	if !valid || c.printer.SupportsColor() {
		return false
	}
	return slices.ContainsFunc(options.Overrides, func(o core.PageOverride) bool {
		return o.ColorMode == colorModeColor
	})
}
