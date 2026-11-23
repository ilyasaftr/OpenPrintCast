package core

// IPP/Windows value bounds and sentinels used when validating options.
const (
	minOrientation = 3
	maxOrientation = 6
	minQuality     = 3
	maxQuality     = 5
	maxPaperID     = 32767
	// Open-ended page-range sentinels: "last page" and "second to last".
	lastPageSentinel         = 0x7fffffff
	secondToLastPageSentinel = 0x7ffffffe
)

// Per-range color modes accepted in PageOverride.ColorMode.
const (
	colorModeAuto       = "auto"
	colorModeColor      = "color"
	colorModeMonochrome = "monochrome"
)

// PageRange uses inclusive, one-based document page numbers.
type PageRange struct {
	First int
	Last  int
}

type PageOverride struct {
	Pages     []PageRange
	Documents []PageRange
	ColorMode string
}

type PrintOptions struct {
	PageRanges  []PageRange
	Monochrome  bool
	Color       bool
	Orientation int
	Quality     int
	PaperID     int
	Sides       Sides
	Overrides   []PageOverride
}

func (o PrintOptions) Valid() bool {
	if o.Monochrome && o.Color {
		return false
	}
	if o.Orientation != 0 && (o.Orientation < minOrientation || o.Orientation > maxOrientation) {
		return false
	}
	if o.Quality != 0 && (o.Quality < minQuality || o.Quality > maxQuality) {
		return false
	}
	if o.PaperID < 0 || o.PaperID > maxPaperID || !o.Sides.Valid() {
		return false
	}
	for _, r := range o.PageRanges {
		if r.First < 1 || r.Last < r.First {
			return false
		}
	}
	for _, override := range o.Overrides {
		if !override.valid() {
			return false
		}
	}
	return true
}

func (p PageOverride) valid() bool {
	if len(p.Pages) == 0 || (p.ColorMode != colorModeAuto && p.ColorMode != colorModeColor && p.ColorMode != colorModeMonochrome) {
		return false
	}
	for _, ranges := range [][]PageRange{p.Pages, p.Documents} {
		previous := 0
		for _, r := range ranges {
			if r.First <= previous || r.Last < r.First || r.Last > lastPageSentinel {
				return false
			}
			previous = r.Last
		}
	}
	return true
}

func (o PrintOptions) IncludesPage(page int) bool {
	if len(o.PageRanges) == 0 {
		return true
	}
	for _, r := range o.PageRanges {
		if page >= r.First && page <= r.Last {
			return true
		}
	}
	return false
}

func (o PrintOptions) IsDefault() bool {
	return !o.Monochrome && !o.Color && o.Orientation == 0 && o.Quality == 0 && o.PaperID == 0 && o.Sides == OneSided && len(o.PageRanges) == 0 && len(o.Overrides) == 0
}

// RequiresColor forces the driver into color mode if any page needs color.
func (o PrintOptions) RequiresColor() bool {
	if o.Color {
		return true
	}
	for _, override := range o.Overrides {
		if override.ColorMode == colorModeColor {
			return true
		}
	}
	return false
}

func rangeIncludes(ranges []PageRange, number, total int) bool {
	if len(ranges) == 0 {
		return true
	}
	resolve := func(value int) int {
		switch value {
		case lastPageSentinel:
			return total
		case secondToLastPageSentinel:
			return total - 1
		default:
			return value
		}
	}
	for _, r := range ranges {
		if number >= resolve(r.First) && number <= resolve(r.Last) {
			return true
		}
	}
	return false
}

// PageMonochrome applies page overrides for the bridge's single document.
// autoMonochrome describes the unmodified Windows driver default.
func (o PrintOptions) PageMonochrome(page, total int, autoMonochrome bool) bool {
	monochrome := o.Monochrome || (!o.Color && o.RequiresColor() && autoMonochrome)
	for _, override := range o.Overrides {
		if !rangeIncludes(override.Documents, 1, 1) || !rangeIncludes(override.Pages, page, total) {
			continue
		}
		switch override.ColorMode {
		case colorModeMonochrome:
			monochrome = true
		case colorModeColor:
			monochrome = false
		case colorModeAuto:
			monochrome = o.RequiresColor() && autoMonochrome
		}
	}
	return monochrome
}
