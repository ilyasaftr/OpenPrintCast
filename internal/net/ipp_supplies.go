package net

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// Marker attributes: how a supply is shown to the two families of clients.
// macOS (CUPS) reads the CUPS marker-* attributes; PWG 5100.13 clients and
// the IPP Everywhere conformance suite read printer-supply, its description
// and the info URI.
const (
	// markerNoColor is marker-colors' value for a supply without one
	// colour; the CUPS documents call it "none".
	markerNoColor = "none"
	// markerFullLevel is marker-high-levels for every supply: the bridge
	// knows no near-full condition, so none is claimed.
	markerFullLevel = core.MaxSupplyPercent
	// receptacleLowLevel is marker-low-levels for a container that fills up
	// ("a value of 0 should be used for supplies that are filled").
	receptacleLowLevel = 0
	// markerMessageFormat and markerMessageSeparator build marker-message,
	// for example "Black 8%, Cyan 75%".
	markerMessageFormat    = "%s %d%%"
	markerMessageSeparator = ", "
)

// Values of printer-supply (PWG 5100.13 section 6.6.11, table 21). A
// measured level is a percentage, so its capacity is 100; a level that is
// not a number (unknown, some remaining) states an unknown capacity (-2).
const (
	supplyClassConsumed   = "supplyThatIsConsumed"
	supplyClassReceptacle = "receptacleThatIsFilled"
	supplyUnitPercent     = "percent"
	supplyUnitOther       = "other"
	supplyCapacityPercent = core.MaxSupplyPercent
	supplyCapacityUnknown = -2
	// supplyColorantNone is the colorantname of a supply with no colorant
	// (table 21, note 1).
	supplyColorantNone = "no-color"
)

// markerType holds the two spellings of a supply kind: CUPS' marker-types
// keyword and the Printer MIB label printer-supply's type= carries.
type markerType struct{ keyword, label string }

// markerTypes lists the kinds that have a spelling. SupplyOther has none:
// CUPS clients know no keyword for it, so it is not published.
var markerTypes = map[core.SupplyKind]markerType{
	core.SupplyToner:      {"toner", "toner"},
	core.SupplyInk:        {"ink", "ink"},
	core.SupplyWasteToner: {"waste-toner", "wasteToner"},
	core.SupplyWasteInk:   {"waste-ink", "wasteInk"},
	core.SupplyDrum:       {"opc", "opc"},
	core.SupplyFuser:      {"fuser", "fuser"},
}

// publishedSupplies is what the backend knows of the printer's supplies,
// without the kinds clients cannot be told about. The result is empty when
// the backend has no data.
func publishedSupplies(shared *SharedState, printer core.PrinterInfo) ([]core.Supply, string) {
	supplies, host := supplyLevels(shared, printer)
	published := make([]core.Supply, 0, len(supplies))
	for _, supply := range supplies {
		if _, ok := markerTypes[supply.Kind]; ok {
			published = append(published, supply)
		}
	}
	if len(published) == 0 {
		return nil, host
	}
	return published, host
}

// addSupplies adds the supply attributes, or nothing at all without data.
func addSupplies(c *printerAttrsContext, attrs *goipp.Attributes) {
	if len(c.supplies) == 0 {
		return
	}
	attrs.Add(markerNamesAttr(c.supplies))
	attrs.Add(markerTypesAttr(c.supplies))
	attrs.Add(markerLevelsAttr(c.supplies))
	attrs.Add(markerColorsAttr(c.supplies))
	attrs.Add(markerLowLevelsAttr(c.supplies))
	attrs.Add(perSupplyInts("marker-high-levels", c.supplies, func(core.Supply) int { return markerFullLevel }))
	if message := markerMessage(c.supplies); message != "" {
		attrs.Add(stringsAttr("marker-message", goipp.TagText, message))
	}
	attrs.Add(printerSupplyAttr(c.supplies))
	attrs.Add(stringsAttr("printer-supply-description", goipp.TagText, supplyNames(c.supplies)...))
	attrs.Add(stringsAttr("printer-supply-info-uri", goipp.TagURI, supplyInfoURI(c)))
}

func supplyNames(supplies []core.Supply) []string {
	names := make([]string, len(supplies))
	for i, supply := range supplies {
		names[i] = supply.Name
	}
	return names
}

func markerNamesAttr(supplies []core.Supply) goipp.Attribute {
	return stringsAttr("marker-names", goipp.TagName, supplyNames(supplies)...)
}

func markerTypesAttr(supplies []core.Supply) goipp.Attribute {
	keywords := make([]string, len(supplies))
	for i, supply := range supplies {
		keywords[i] = markerTypes[supply.Kind].keyword
	}
	return keywordAttr("marker-types", keywords...)
}

// markerLevelsAttr gives the percent, or the MIB marker CUPS shares.
func markerLevelsAttr(supplies []core.Supply) goipp.Attribute {
	return perSupplyInts("marker-levels", supplies, func(s core.Supply) int { return int(s.Level) })
}

// markerColorsAttr gives each supply's sRGB colour; a supply whose colour
// is not a standard colorant has none rather than a guessed one.
func markerColorsAttr(supplies []core.Supply) goipp.Attribute {
	colors := make([]string, len(supplies))
	for i, supply := range supplies {
		if colors[i] = core.ColorantHex(supply.Color); colors[i] == "" {
			colors[i] = markerNoColor
		}
	}
	return stringsAttr("marker-colors", goipp.TagName, colors...)
}

func markerLowLevelsAttr(supplies []core.Supply) goipp.Attribute {
	return perSupplyInts("marker-low-levels", supplies, func(s core.Supply) int {
		if s.Receptacle {
			return receptacleLowLevel
		}
		return core.SupplyLowPercent
	})
}

// perSupplyInts builds an integer attribute with one value per supply.
func perSupplyInts(name string, supplies []core.Supply, value func(core.Supply) int) goipp.Attribute {
	attr := goipp.Attribute{Name: name}
	for _, supply := range supplies {
		attr.Values.Add(goipp.TagInteger, goipp.Integer(value(supply)))
	}
	return attr
}

// markerMessage summarises the measured levels ("Black 8%, Cyan 75%"); it
// is empty when no level is a number.
func markerMessage(supplies []core.Supply) string {
	var parts []string
	for _, supply := range supplies {
		if percent, ok := supply.Level.Percent(); ok {
			parts = append(parts, fmt.Sprintf(markerMessageFormat, supply.Label(), percent))
		}
	}
	return strings.Join(parts, markerMessageSeparator)
}

// printerSupplyAttr is printer-supply: one octetString per supply in the
// PWG 5100.13 key=value syntax, the way CUPS builds it from marker-*.
func printerSupplyAttr(supplies []core.Supply) goipp.Attribute {
	attr := goipp.Attribute{Name: "printer-supply"}
	for i, supply := range supplies {
		attr.Values.Add(goipp.TagString, goipp.Binary(printerSupplyValue(i+1, supply)))
	}
	return attr
}

func printerSupplyValue(index int, supply core.Supply) string {
	class, colorant := supplyClassConsumed, supply.Color
	if supply.Receptacle {
		class = supplyClassReceptacle
	}
	if colorant == "" {
		colorant = supplyColorantNone
	}
	unit, capacity := supplyUnitOther, supplyCapacityUnknown
	if _, measured := supply.Level.Percent(); measured {
		unit, capacity = supplyUnitPercent, supplyCapacityPercent
	}
	return fmt.Sprintf("index=%d;class=%s;type=%s;unit=%s;maxcapacity=%d;level=%d;colorantname=%s;",
		index, class, markerTypes[supply.Kind].label, unit, capacity, int(supply.Level), colorant)
}

// supplyInfoURI is the web page for managing the printer's supplies
// (printer-supply-info-uri must be an http or https page): the printer's
// own embedded web server, which is where replacement and ordering live,
// when its address is known from the Windows port; otherwise the bridge's
// page for the printer, which always answers.
func supplyInfoURI(c *printerAttrsContext) string {
	if uri, ok := printerWebURI(c.supplyHost); ok {
		return uri
	}
	return c.webURI()
}

// printerWebURI is http://<host>/ for a printer's host name or address.
func printerWebURI(host string) (string, bool) {
	if host == "" {
		return "", false
	}
	if addr, err := netip.ParseAddr(host); err == nil {
		if addr.Zone() != "" {
			return "", false
		}
		if addr.Is6() {
			host = "[" + host + "]"
		}
	}
	return "http://" + host + "/", true
}

// mergeSupplyReasons adds the keywords derived from supply levels to the
// printer-state-reasons the driver reported, without repeating a condition
// the driver already named (Windows says marker-supply-empty, the bridge
// says marker-supply-empty-error: one condition) and without saying both
// "low" and "empty": empty wins.
func mergeSupplyReasons(reasons, derived []string) []string {
	if len(derived) == 0 {
		return reasons
	}
	merged := slices.DeleteFunc(slices.Clone(reasons), func(r string) bool { return r == "none" })
	for _, keyword := range derived {
		if !slices.ContainsFunc(merged, func(r string) bool { return supplyCondition(r) == supplyCondition(keyword) }) {
			merged = append(merged, keyword)
		}
	}
	if slices.ContainsFunc(merged, isSupplyEmpty) {
		merged = slices.DeleteFunc(merged, isSupplyLow)
	}
	return merged
}

// Printer-state-reasons severity suffixes (RFC 8011 section 5.4.12).
var reasonSeverities = []string{"-error", "-warning", "-report"}

// supplyCondition is a reason without its severity suffix, so
// "marker-supply-low" and "marker-supply-low-warning" are one condition.
func supplyCondition(reason string) string {
	for _, suffix := range reasonSeverities {
		if base, found := strings.CutSuffix(reason, suffix); found {
			return base
		}
	}
	return reason
}

func isSupplyEmpty(reason string) bool { return supplyCondition(reason) == "marker-supply-empty" }

func isSupplyLow(reason string) bool { return supplyCondition(reason) == "marker-supply-low" }
