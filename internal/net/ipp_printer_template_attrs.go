package net

import (
	"slices"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

const (
	// maxImageDimension bounds JPEG width and height in pixels.
	maxImageDimension = 65535
	// rasterResolutionDPI is the only PWG raster resolution advertised.
	rasterResolutionDPI = 300
)

// documentPrinterSizes are the paper sizes that mark a printer as
// a document printer.
var documentPrinterSizes = []string{"iso_a4_210x297mm", "na_letter_8.5x11in", "na_legal_8.5x14in", "iso_a3_297x420mm"}

func addMedia(c *printerAttrsContext, attrs *goipp.Attributes) {
	if len(c.media) == 0 {
		attrs.Add(voidAttr("media-col-default", goipp.TagUnknown))
		return
	}
	for _, attr := range c.media {
		attrs.Add(attr)
	}
}

func addDocumentFormats(_ *printerAttrsContext, attrs *goipp.Attributes) {
	attrs.Add(stringsAttr("document-format-default", goipp.TagMimeType, mimeOctetStream))
	attrs.Add(intAttr("document-password-supported", 0))
	attrs.Add(stringsAttr("document-format-supported", goipp.TagMimeType, supportedDocumentFormats()...))
	attrs.Add(keywordAttr("jpeg-features-supported", "progressive"))
	attrs.Add(rangeAttr("jpeg-k-octets-supported", 0, maxDocumentBytes/1024))
	attrs.Add(rangeAttr("jpeg-x-dimension-supported", 0, maxImageDimension))
	attrs.Add(rangeAttr("jpeg-y-dimension-supported", 1, maxImageDimension))
	attrs.Add(rangeAttr("pdf-k-octets-supported", 0, maxDocumentBytes/1024))
}

func addPrinterKind(c *printerAttrsContext, attrs *goipp.Attributes) {
	if offersDocumentSize(c.media) {
		attrs.Add(keywordAttr("printer-kind", "document"))
	}
}

func offersDocumentSize(media goipp.Attributes) bool {
	for _, attr := range media {
		if attr.Name != "media-supported" {
			continue
		}
		for _, value := range attr.Values {
			if slices.Contains(documentPrinterSizes, value.V.String()) {
				return true
			}
		}
	}
	return false
}

func addPrintScaling(_ *printerAttrsContext, attrs *goipp.Attributes) {
	attrs.Add(keywordAttr("print-scaling-default", "fit"))
	attrs.Add(keywordAttr("print-scaling-supported", "fit"))
}

func addRasterFormats(c *printerAttrsContext, attrs *goipp.Attributes) {
	urf := []string{"W8"}
	if c.printer.SupportsColor() {
		urf = append(urf, "SRGB24")
	}
	attrs.Add(keywordAttr("urf-supported", append(urf, "V1.4", "RS300", "DM1")...))
	attrs.Add(keywordAttr("pwg-raster-document-type-supported", "sgray_8", "srgb_8"))
	attrs.Add(resolutionAttr("pwg-raster-document-resolution-supported",
		goipp.Resolution{Xres: rasterResolutionDPI, Yres: rasterResolutionDPI, Units: goipp.UnitsDpi}))
}

func addJobTemplateDefaults(_ *printerAttrsContext, attrs *goipp.Attributes) {
	attrs.Add(keywordAttr("output-bin-default", valueAuto))
	attrs.Add(keywordAttr("output-bin-supported", valueAuto))
	attrs.Add(intAttr("copies-default", 1))
	attrs.Add(rangeAttr("copies-supported", 1, maxCopies))
	attrs.Add(enumAttr("finishings-default", finishingsNone))
	attrs.Add(enumAttr("finishings-supported", finishingsNone))
	attrs.Add(boolAttr("page-ranges-supported", true))
}

// addSides offers the two-sided keywords only when the driver reports and
// accepts duplex. The driver's own default is not read: one-sided stays the
// default either way.
func addSides(c *printerAttrsContext, attrs *goipp.Attributes) {
	supported := []string{core.OneSided.Keyword()}
	if c.duplex {
		supported = core.SidesKeywords()
	}
	attrs.Add(keywordAttr("sides-default", core.OneSided.Keyword()))
	attrs.Add(keywordAttr("sides-supported", supported...))
}

func addPrintRenderingIntent(_ *printerAttrsContext, attrs *goipp.Attributes) {
	attrs.Add(keywordAttr("print-rendering-intent-default", valueAuto))
	attrs.Add(keywordAttr("print-rendering-intent-supported", valueAuto))
}

func addPrintQuality(c *printerAttrsContext, attrs *goipp.Attributes) {
	if len(c.qualities) == 0 {
		return
	}
	attrs.Add(enumAttr("print-quality-default", c.defaultQuality))
	attrs.Add(enumAttr("print-quality-supported", c.qualities...))
}

func addOrientation(c *printerAttrsContext, attrs *goipp.Attributes) {
	if !c.orientation {
		return
	}
	attrs.Add(voidAttr("orientation-requested-default", goipp.TagNoValue))
	attrs.Add(enumAttr("orientation-requested-supported", orientationPortrait, orientationLandscape, orientationReverseLandscape, orientationReversePortrait))
}

func addColorMode(c *printerAttrsContext, attrs *goipp.Attributes) {
	attrs.Add(keywordAttr("print-color-mode-default", valueAuto))
	modes := []string{valueAuto}
	if c.printer.SupportsColor() {
		modes = append(modes, colorModeColor)
	}
	attrs.Add(keywordAttr("print-color-mode-supported", append(modes, colorModeMonochrome)...))
}

func addJobCreationSupport(c *printerAttrsContext, attrs *goipp.Attributes) {
	attrs.Add(keywordAttr("overrides-supported", "pages", "document-numbers", "document-number", "print-color-mode"))
	attrs.Add(keywordAttr("job-creation-attributes-supported", c.jobCreationAttributes()...))
}

// jobCreationAttributes lists the job attributes the printer accepts; the
// media, orientation and quality ones depend on the driver.
func (c *printerAttrsContext) jobCreationAttributes() []string {
	names := []string{"job-name", "copies", "finishings", "page-ranges",
		"print-color-mode", "print-content-optimize", "print-rendering-intent",
		"print-scaling", "sides", "output-bin", "overrides"}
	if len(c.media) > 0 {
		names = append(names, "media", "media-col")
		if opAttrString(c.media, "printer-resolution-supported") != "" {
			names = append(names, "printer-resolution")
		}
	}
	if c.orientation {
		names = append(names, "orientation-requested")
	}
	if len(c.qualities) > 0 {
		names = append(names, "print-quality")
	}
	return names
}

func addServiceCapabilities(_ *printerAttrsContext, attrs *goipp.Attributes) {
	attrs.Add(boolAttr("job-ids-supported", true))
	attrs.Add(boolAttr("preferred-attributes-supported", false))
	attrs.Add(keywordAttr("print-content-optimize-default", valueAuto))
	attrs.Add(keywordAttr("print-content-optimize-supported", valueAuto))
	attrs.Add(voidAttr("printer-geo-location", goipp.TagUnknown))
	attrs.Add(boolAttr("multiple-document-jobs-supported", false))
	attrs.Add(intAttr("multiple-operation-time-out", int(pendingJobTimeout.Seconds())))
	attrs.Add(keywordAttr("multiple-operation-time-out-action", "abort-job"))
	attrs.Add(keywordAttr("which-jobs-supported", whichJobsAll, whichJobsCompleted, whichJobsNotCompleted))
}

func addRasterSheetBack(_ *printerAttrsContext, attrs *goipp.Attributes) {
	attrs.Add(keywordAttr("pwg-raster-document-sheet-back", "normal"))
}

func addOperations(c *printerAttrsContext, attrs *goipp.Attributes) {
	ops := make([]int, len(advertisedOperations))
	for i, op := range advertisedOperations {
		ops[i] = int(op)
	}
	identify := false
	if c.shared != nil {
		_, identify = capabilitySource(c.shared, c.printer).(identifyProvider)
	}
	if identify {
		ops = append(ops, int(goipp.OpIdentifyPrinter))
		attrs.Add(keywordAttr("identify-actions-default", identifyActionSound))
		attrs.Add(keywordAttr("identify-actions-supported", identifyActionSound))
	}
	attrs.Add(enumAttr("operations-supported", ops...))
}
