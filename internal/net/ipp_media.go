package net

import (
	"slices"
	"strconv"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

const (
	// maxPaperID is the largest DMPAPER id a driver paper size may carry.
	maxPaperID = 32767
	// mediaSizeTolerance is how far (hundredths of a mm) a driver size may
	// stray from a standard size and still take its name.
	mediaSizeTolerance = 100
)

type mediaChoice struct {
	name                 string
	width, height, paper int
	margins              core.MediaDetails
}

var standardMediaSizes = []mediaChoice{
	{name: "iso_a3_297x420mm", width: 29700, height: 42000},
	{name: "iso_a4_210x297mm", width: 21000, height: 29700},
	{name: "iso_a5_148x210mm", width: 14800, height: 21000},
	{name: "iso_a6_105x148mm", width: 10500, height: 14800},
	{name: "na_letter_8.5x11in", width: 21590, height: 27940},
	{name: "na_legal_8.5x14in", width: 21590, height: 35560},
}

func namedMedia(width, height int) mediaChoice {
	if width > height {
		width, height = height, width
	}
	for _, size := range standardMediaSizes {
		if absInt(width-size.width) <= mediaSizeTolerance && absInt(height-size.height) <= mediaSizeTolerance {
			return size
		}
	}
	mm := func(n int) string { return strconv.FormatFloat(float64(n)/100, 'f', -1, 64) }
	return mediaChoice{name: "custom_default_" + mm(width) + "x" + mm(height) + "mm", width: width, height: height}
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// defaultMediaDetails asks the driver for its default paper; ok is false when
// the driver reports none or an unusable size.
func defaultMediaDetails(spool any, printer core.PrinterInfo) (info core.MediaDetails, ok bool) {
	if provider, supported := spool.(detailedMediaProvider); supported {
		info, ok = provider.DefaultMediaDetails(printer.WindowsName)
	} else if provider, supported := spool.(mediaProvider); supported {
		info.Width, info.Height, ok = provider.DefaultMedia(printer.WindowsName)
	}
	if !ok || info.Width <= 0 || info.Height <= 0 {
		return core.MediaDetails{}, false
	}
	return info, true
}

// supportedMediaChoices lists the driver's selectable papers.
func supportedMediaChoices(spool any, printer core.PrinterInfo) []mediaChoice {
	provider, ok := spool.(supportedMediaProvider)
	if !ok {
		return nil
	}
	var choices []mediaChoice
	for _, paper := range provider.SupportedMedia(printer.WindowsName) {
		if paper.PaperID <= 0 || paper.PaperID > maxPaperID || paper.Width <= 0 || paper.Height <= 0 {
			continue
		}
		choice := namedMedia(paper.Width, paper.Height)
		choice.paper = paper.PaperID
		choice.margins = core.MediaDetails{
			Left: paper.Left, Right: paper.Right, Top: paper.Top, Bottom: paper.Bottom, MarginsKnown: paper.MarginsKnown,
		}
		choices = append(choices, choice)
	}
	return choices
}

func mediaChoices(shared *SharedState, printer core.PrinterInfo) (core.MediaDetails, []mediaChoice) {
	if shared == nil {
		return core.MediaDetails{}, nil
	}
	var choices []mediaChoice
	seen := make(map[string]bool)
	info, hasDefault := defaultMediaDetails(capabilitySource(shared, printer), printer)
	if hasDefault {
		first := namedMedia(info.Width, info.Height)
		first.margins = info
		choices = append(choices, first)
		seen[first.name] = true
	}
	for _, choice := range supportedMediaChoices(capabilitySource(shared, printer), printer) {
		if !seen[choice.name] {
			choices = append(choices, choice)
			seen[choice.name] = true
		}
	}
	return info, choices
}

type mediaMargin struct {
	name  string
	value int
}

// mediaMargins lists the margins in media-col member order; none when the
// driver did not report them.
func mediaMargins(details core.MediaDetails) []mediaMargin {
	if !details.MarginsKnown {
		return nil
	}
	return []mediaMargin{
		{"media-left-margin", details.Left}, {"media-right-margin", details.Right},
		{"media-top-margin", details.Top}, {"media-bottom-margin", details.Bottom},
	}
}

func mediaSize(width, height int) goipp.Collection {
	return goipp.Collection{intAttr("x-dimension", width), intAttr("y-dimension", height)}
}

// mediaCollection describes choice as a media-col; negative margins are
// omitted.
func mediaCollection(choice mediaChoice) goipp.Collection {
	col := goipp.Collection{
		collectionAttr("media-size", mediaSize(choice.width, choice.height)),
		keywordAttr("media-source", valueAuto),
		keywordAttr("media-type", valueAuto),
	}
	for _, margin := range mediaMargins(choice.margins) {
		if margin.value >= 0 {
			col = append(col, intAttr(margin.name, margin.value))
		}
	}
	return col
}

func mediaAttrs(shared *SharedState, printer core.PrinterInfo) goipp.Attributes {
	info, choices := mediaChoices(shared, printer)
	if len(choices) == 0 {
		return nil
	}
	first := choices[0]
	margins := mediaMargins(first.margins)
	if slices.ContainsFunc(margins, func(m mediaMargin) bool { return m.value < 0 }) {
		return nil
	}
	hasDefault := first.paper == 0
	col := mediaCollection(first)
	attrs := mediaSupportedAttrs(first, margins)
	attrs = append(attrs, mediaDefaultAttrs(first, col, hasDefault)...)
	attrs.Add(collectionAttr("media-col-database", col))
	for _, choice := range choices[1:] {
		addAlternateMedia(&attrs, choice)
	}
	if hasDefault && mediaReady(shared, printer) {
		attrs.Add(keywordAttr("media-ready", first.name))
		attrs.Add(collectionAttr("media-col-ready", col))
	}
	return append(attrs, resolutionAttrs(info)...)
}

// mediaSupportedAttrs holds the *-supported attributes for the first choice.
func mediaSupportedAttrs(first mediaChoice, margins []mediaMargin) goipp.Attributes {
	attrs := goipp.Attributes{
		keywordAttr("media-supported", first.name),
		collectionAttr("media-size-supported", mediaSize(first.width, first.height)),
		keywordAttr("media-source-supported", valueAuto),
		keywordAttr("media-type-supported", valueAuto),
		// RFC 3805 uses -2 for unknown dimensions and capacity, and 5 for
		// unknown availability. "auto" represents the logical source only.
		stringsAttr("printer-input-tray", goipp.TagString,
			"type=other;mediafeed=-2;mediaxfeed=-2;maxcapacity=-2;level=-2;status=5;name=auto;"),
	}
	members := []string{"media-size", "media-source", "media-type"}
	for _, margin := range margins {
		attrs.Add(intAttr(margin.name+"-supported", margin.value))
		members = append(members, margin.name)
	}
	attrs.Add(keywordAttr("media-col-supported", members...))
	return attrs
}

func mediaDefaultAttrs(first mediaChoice, col goipp.Collection, hasDefault bool) goipp.Attributes {
	if !hasDefault {
		return goipp.Attributes{voidAttr("media-col-default", goipp.TagUnknown)}
	}
	return goipp.Attributes{
		keywordAttr("media-default", first.name),
		collectionAttr("media-col-default", col),
	}
}

// addAlternateMedia extends the supported/database attributes with a paper
// other than the default.
func addAlternateMedia(attrs *goipp.Attributes, choice mediaChoice) {
	for _, margin := range mediaMargins(choice.margins) {
		if margin.value < 0 {
			continue
		}
		addSupportedMediaValue(attrs, margin.name+"-supported", goipp.TagInteger, goipp.Integer(margin.value))
		addSupportedMediaValue(attrs, "media-col-supported", goipp.TagKeyword, goipp.String(margin.name))
	}
	appendAttrValue(*attrs, "media-supported", goipp.TagKeyword, goipp.String(choice.name))
	appendAttrValue(*attrs, "media-size-supported", goipp.TagBeginCollection, mediaSize(choice.width, choice.height))
	appendAttrValue(*attrs, "media-col-database", goipp.TagBeginCollection, mediaCollection(choice))
}

func mediaReady(shared *SharedState, printer core.PrinterInfo) bool {
	ready, ok := capabilitySource(shared, printer).(mediaReadyProvider)
	return ok && ready.MediaReady(printer.WindowsName)
}

func resolutionAttrs(info core.MediaDetails) goipp.Attributes {
	if info.DPIX <= 0 || info.DPIY <= 0 {
		return nil
	}
	resolution := goipp.Resolution{Xres: info.DPIX, Yres: info.DPIY, Units: goipp.UnitsDpi}
	return goipp.Attributes{
		resolutionAttr("printer-resolution-default", resolution),
		resolutionAttr("printer-resolution-supported", resolution),
	}
}

func findAttr(attrs goipp.Attributes, name string) *goipp.Attribute {
	for i := range attrs {
		if attrs[i].Name == name {
			return &attrs[i]
		}
	}
	return nil
}

// appendAttrValue adds a value to an existing attribute.
func appendAttrValue(attrs goipp.Attributes, name string, tag goipp.Tag, value goipp.Value) {
	if attr := findAttr(attrs, name); attr != nil {
		attr.Values.Add(tag, value)
	}
}

// addSupportedMediaValue adds value to the named attribute unless it is
// already listed, creating the attribute when missing.
func addSupportedMediaValue(attrs *goipp.Attributes, name string, tag goipp.Tag, value goipp.Value) {
	attr := findAttr(*attrs, name)
	if attr == nil {
		attrs.Add(goipp.MakeAttr(name, tag, value))
		return
	}
	for _, existing := range attr.Values {
		if existing.T == tag && existing.V.String() == value.String() {
			return
		}
	}
	attr.Values.Add(tag, value)
}
