package net

import (
	"math"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

const (
	maxCopies = 99
	// maxOverrideValues bounds both the overrides attribute and each range
	// list inside one override.
	maxOverrideValues = 128
	// finishingsNone is the only finishings enum the bridge supports.
	finishingsNone = 3
)

// IPP orientation-requested enum values (RFC 8011 section 5.2.10).
const (
	orientationPortrait         = 3
	orientationLandscape        = 4
	orientationReverseLandscape = 5
	orientationReversePortrait  = 6
)

const (
	colorModeColor      = "color"
	colorModeMonochrome = "monochrome"
)

func validColorMode(mode string) bool {
	return mode == valueAuto || mode == colorModeColor || mode == colorModeMonochrome
}

// jobOptionParser decodes one job-template attribute into opts, reporting
// whether the attribute is well formed.
type jobOptionParser func(attr goipp.Attribute, opts *core.PrintOptions) bool

var jobOptionParsers = map[string]jobOptionParser{
	"page-ranges":           parsePageRanges,
	"print-color-mode":      parseColorMode,
	"sides":                 parseSides,
	"orientation-requested": parseOrientation,
	"print-quality":         parseQuality,
	"overrides":             parseOverrides,
}

func jobOptions(req *goipp.Message) (core.PrintOptions, bool) {
	var opts core.PrintOptions
	seen := make(map[string]bool)
	for _, attr := range req.Job {
		parse, known := jobOptionParsers[attr.Name]
		if !known {
			continue
		}
		if seen[attr.Name] || !parse(attr, &opts) {
			return opts, false
		}
		seen[attr.Name] = true
	}
	return opts, true
}

func parsePageRanges(attr goipp.Attribute, opts *core.PrintOptions) bool {
	if len(attr.Values) == 0 {
		return false
	}
	for _, value := range attr.Values {
		r, ok := value.V.(goipp.Range)
		if !ok || value.T != goipp.TagRange || r.Lower < 1 || r.Upper < r.Lower {
			return false
		}
		opts.PageRanges = append(opts.PageRanges, core.PageRange{First: r.Lower, Last: r.Upper})
	}
	return true
}

func parseColorMode(attr goipp.Attribute, opts *core.PrintOptions) bool {
	if !hasSingleValue(attr, goipp.TagKeyword) {
		return false
	}
	switch attr.Values[0].V.String() {
	case valueAuto:
	case colorModeColor:
		opts.Color = true
	case colorModeMonochrome:
		opts.Monochrome = true
	default:
		return false
	}
	return true
}

func parseSides(attr goipp.Attribute, opts *core.PrintOptions) bool {
	if !hasSingleValue(attr, goipp.TagKeyword) {
		return false
	}
	sides, known := core.ParseSides(firstValue(attr))
	opts.Sides = sides
	return known
}

func parseOrientation(attr goipp.Attribute, opts *core.PrintOptions) bool {
	if !hasSingleValue(attr, goipp.TagEnum) {
		return false
	}
	orientation, ok := attr.Values[0].V.(goipp.Integer)
	if !ok || orientation < orientationPortrait || orientation > orientationReversePortrait {
		return false
	}
	opts.Orientation = int(orientation)
	return true
}

func parseQuality(attr goipp.Attribute, opts *core.PrintOptions) bool {
	if !hasSingleValue(attr, goipp.TagEnum) {
		return false
	}
	quality, ok := attr.Values[0].V.(goipp.Integer)
	if !ok || quality < printQualityDraft || quality > printQualityHigh {
		return false
	}
	opts.Quality = int(quality)
	return true
}

func parseOverrides(attr goipp.Attribute, opts *core.PrintOptions) bool {
	if len(attr.Values) == 0 || len(attr.Values) > maxOverrideValues {
		return false
	}
	for _, value := range attr.Values {
		col, ok := value.V.(goipp.Collection)
		if !ok || value.T != goipp.TagBeginCollection {
			return false
		}
		override, ok := parseOverride(col)
		if !ok {
			return false
		}
		opts.Overrides = append(opts.Overrides, override)
	}
	return true
}

// parseOverride decodes one overrides member collection: pages, an optional
// document selector, and the print-color-mode, in that order.
func parseOverride(col goipp.Collection) (core.PageOverride, bool) {
	if len(col) < 2 || col[0].Name != "pages" {
		return core.PageOverride{}, false
	}
	pages, ok := overrideRanges(col[0])
	if !ok {
		return core.PageOverride{}, false
	}
	override := core.PageOverride{Pages: pages}
	index := 1
	switch col[index].Name {
	case "document-numbers":
		if override.Documents, ok = overrideRanges(col[index]); !ok {
			return core.PageOverride{}, false
		}
		index++
	case "document-number":
		if override.Documents, ok = overrideDocumentNumber(col[index]); !ok {
			return core.PageOverride{}, false
		}
		index++
	}
	if index != len(col)-1 || col[index].Name != "print-color-mode" || !hasSingleValue(col[index], goipp.TagKeyword) {
		return core.PageOverride{}, false
	}
	override.ColorMode = col[index].Values[0].V.String()
	return override, validColorMode(override.ColorMode)
}

func overrideDocumentNumber(member goipp.Attribute) ([]core.PageRange, bool) {
	if !hasSingleValue(member, goipp.TagInteger) {
		return nil, false
	}
	number, valid := member.Values[0].V.(goipp.Integer)
	if !valid || number < 1 {
		return nil, false
	}
	return []core.PageRange{{First: int(number), Last: int(number)}}, true
}

func overrideRanges(attr goipp.Attribute) ([]core.PageRange, bool) {
	if len(attr.Values) == 0 || len(attr.Values) > maxOverrideValues {
		return nil, false
	}
	var ranges []core.PageRange
	previous := 0
	for _, value := range attr.Values {
		r, ok := value.V.(goipp.Range)
		if !ok || value.T != goipp.TagRange || r.Lower <= previous || r.Upper < r.Lower || r.Upper > math.MaxInt32 {
			return nil, false
		}
		ranges = append(ranges, core.PageRange{First: r.Lower, Last: r.Upper})
		previous = r.Upper
	}
	return ranges, true
}

func jobCopies(req *goipp.Message) (int, bool) {
	for _, attr := range req.Job {
		if attr.Name == "copies" {
			if !hasSingleValue(attr, goipp.TagInteger) {
				return 0, false
			}
			value, ok := attr.Values[0].V.(goipp.Integer)
			return int(value), ok && value >= 1 && value <= maxCopies
		}
	}
	return 1, true
}

// validJobAttributes returns the requested copies and whether both the
// copies and the print options of req are acceptable.
func validJobAttributes(req *goipp.Message) (copies int, ok bool) {
	copies, ok = jobCopies(req)
	if !ok {
		return copies, false
	}
	_, ok = jobOptions(req)
	return copies, ok
}

func selectedPaper(req *goipp.Message, shared *SharedState, printer core.PrinterInfo) int {
	for _, attr := range req.Job {
		switch attr.Name {
		case "media":
			if choice, ok := mediaChoiceForName(attr, shared, printer); ok {
				return choice.paper
			}
		case "media-col":
			if choice, ok := mediaChoiceForCollection(attr, shared, printer); ok {
				return choice.paper
			}
		}
	}
	return 0
}
