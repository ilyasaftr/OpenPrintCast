package core

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// SupplyKind is the kind of consumable a printer reports, reduced to the
// kinds the bridge can describe to clients.
type SupplyKind string

const (
	SupplyToner      SupplyKind = "toner"
	SupplyInk        SupplyKind = "ink"
	SupplyWasteToner SupplyKind = "waste-toner"
	SupplyWasteInk   SupplyKind = "waste-ink"
	SupplyDrum       SupplyKind = "drum"
	SupplyFuser      SupplyKind = "fuser"
	SupplyOther      SupplyKind = "other"
)

// SupplyLevel is a supply's level in percent (0..MaxSupplyPercent) or one
// of the Printer MIB's negative markers for a level that is not a number
// (RFC 3805, prtMarkerSuppliesLevel). CUPS' marker-levels use the same
// values, so the markers pass through to clients unchanged.
type SupplyLevel int

const (
	// SupplyLevelOther (-1): the printer places no restriction on the value.
	SupplyLevelOther SupplyLevel = -1
	// SupplyLevelUnknown (-2): the printer does not know the level.
	SupplyLevelUnknown SupplyLevel = -2
	// SupplyLevelSomeRemaining (-3): the printer knows some supply is left
	// (or, for a receptacle, some space) without a measurement.
	SupplyLevelSomeRemaining SupplyLevel = -3
)

// MaxSupplyPercent is the level of a full supply.
const MaxSupplyPercent = 100

// SupplyLowPercent is the level at or below which a toner or ink supply is
// reported as low. It is the bridge's policy, not a printer value.
const SupplyLowPercent = 10

// Supply is one consumable of a printer as the bridge reports it.
type Supply struct {
	// Name is the printer's description of the supply, or one derived from
	// its colour and kind when the printer gave none that is readable.
	Name string
	// Color is the standard colorant name (cyan, black, light-magenta); it
	// is empty when the supply has no single colour.
	Color string
	Kind  SupplyKind
	// Level is the percent remaining, or for a Receptacle the percent
	// filled (the CUPS convention: a waste tank at 80 is nearly full).
	Level SupplyLevel
	// Receptacle is true for a container that fills up (waste toner)
	// instead of being used up.
	Receptacle bool
}

// Percent is the level as a percentage; ok is false for a marker.
func (l SupplyLevel) Percent() (percent int, ok bool) {
	return int(l), l >= 0
}

// Label is the short name for status text: the title-cased colour of a
// toner or ink supply ("Light cyan"), otherwise the supply's own name.
func (s Supply) Label() string {
	if s.Color != "" && !s.Receptacle && (s.Kind == SupplyToner || s.Kind == SupplyInk) {
		return titleCase(strings.ReplaceAll(s.Color, "-", " "))
	}
	return s.Name
}

// Marker-supply keywords of printer-state-reasons (RFC 8011 section 5.4.12
// with the -warning and -error severities).
const (
	ReasonSupplyLowWarning = "marker-supply-low-warning"
	ReasonSupplyEmptyError = "marker-supply-empty-error"
)

// SupplyStateReasons derives the printer-state-reasons keyword from levels
// for toner and ink: empty (0%) when any supply is used up, otherwise low
// (at or below SupplyLowPercent) when any is low. Only the more severe
// keyword is given, so the reasons never say both. Waste containers raise
// none: vendors disagree on what their level means, which is why CUPS' own
// SNMP backend ignores them too.
func SupplyStateReasons(supplies []Supply) []string {
	var empty, low bool
	for _, s := range supplies {
		percent, known := s.Level.Percent()
		if !known || s.Receptacle || (s.Kind != SupplyToner && s.Kind != SupplyInk) {
			continue
		}
		empty = empty || percent == 0
		low = low || percent <= SupplyLowPercent
	}
	switch {
	case empty:
		return []string{ReasonSupplyEmptyError}
	case low:
		return []string{ReasonSupplyLowWarning}
	default:
		return nil
	}
}

// maxSupplyNameRunes bounds a supply description taken from the network.
const maxSupplyNameRunes = 100

// cleanSupplyText makes a printer-supplied description safe to show: it
// returns "" for text that is not valid UTF-8 (a legacy code page would
// show as garbage), replaces control characters, collapses whitespace and
// bounds the length.
func cleanSupplyText(text string) string {
	if !utf8.ValidString(text) {
		return ""
	}
	text = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, text)
	text = strings.Join(strings.Fields(text), " ")
	if runes := []rune(text); len(runes) > maxSupplyNameRunes {
		text = strings.TrimSpace(string(runes[:maxSupplyNameRunes]))
	}
	return text
}

// titleCase upper-cases the first letter of text.
func titleCase(text string) string {
	if text == "" {
		return ""
	}
	first, size := utf8.DecodeRuneInString(text)
	return string(unicode.ToUpper(first)) + text[size:]
}
