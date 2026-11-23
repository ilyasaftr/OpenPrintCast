package core

import (
	"cmp"
	"slices"
	"strings"
)

// Printer MIB v2 (RFC 3805) enumerations used by prtMarkerSuppliesTable.
const (
	// prtMarkerSuppliesClass (PrtMarkerSuppliesClassTC).
	mibClassConsumed   = 3 // supplyThatIsConsumed
	mibClassReceptacle = 4 // receptacleThatIsFilled

	// prtMarkerSuppliesType (PrtMarkerSuppliesTypeTC).
	mibTypeToner          = 3
	mibTypeWasteToner     = 4
	mibTypeInk            = 5
	mibTypeInkCartridge   = 6
	mibTypeInkRibbon      = 7
	mibTypeWasteInk       = 8
	mibTypeOPC            = 9 // photo conductor
	mibTypeFuser          = 15
	mibTypeTonerCartridge = 21

	// prtMarkerSuppliesSupplyUnit (PrtMarkerSuppliesSupplyUnitTC).
	mibUnitPercent = 19
)

// MarkerSupplyRow is one prtMarkerSuppliesTable row as the printer reports
// it, with the name of its colorant (prtMarkerColorantValue) resolved. The
// numeric fields keep the MIB's own values, including its -1, -2 and -3
// markers.
type MarkerSupplyRow struct {
	Index       int // prtMarkerSuppliesIndex
	Class       int
	Type        int
	Unit        int
	MaxCapacity int
	Level       int
	Description string
	Colorant    string
}

// rankedSupply is a Supply with the keys NormalizeSupplies orders by.
type rankedSupply struct {
	Supply
	index int
}

// NormalizeSupplies turns raw Printer MIB rows into supplies with levels in
// percent. A row is dropped when it cannot be interpreted: a level outside
// the MIB's range, or a class that is neither consumed nor a receptacle
// (the meaning of its level would be a guess). A level that cannot be
// turned into a percentage (no capacity, or a unit other than percent)
// stays in the result as SupplyLevelUnknown rather than being invented.
// The order is deterministic: consumed supplies before receptacles, then by
// kind, then black, cyan, magenta, yellow and other colours, then by index.
func NormalizeSupplies(rows []MarkerSupplyRow) []Supply {
	ranked := make([]rankedSupply, 0, len(rows))
	for _, row := range rows {
		if supply, ok := normalizeSupply(row); ok {
			ranked = append(ranked, rankedSupply{supply, row.Index})
		}
	}
	slices.SortStableFunc(ranked, compareSupplies)
	supplies := make([]Supply, len(ranked))
	for i, r := range ranked {
		supplies[i] = r.Supply
	}
	return supplies
}

func normalizeSupply(row MarkerSupplyRow) (Supply, bool) {
	if row.Level < int(SupplyLevelSomeRemaining) {
		return Supply{}, false
	}
	receptacle := row.Class == mibClassReceptacle
	if !receptacle && row.Class != mibClassConsumed {
		return Supply{}, false
	}
	supply := Supply{
		Color:      normalizeColorant(row.Colorant),
		Kind:       kindFromMIBType(row.Type),
		Level:      supplyLevel(row, receptacle),
		Receptacle: receptacle,
	}
	supply.Name = cleanSupplyText(row.Description)
	if supply.Name == "" {
		supply.Name = generatedSupplyName(supply)
	}
	return supply, true
}

// kindFromMIBType maps prtMarkerSuppliesType to the kinds the bridge
// describes; every other type (developer, transfer unit, staples, ...) is
// SupplyOther.
func kindFromMIBType(mibType int) SupplyKind {
	switch mibType {
	case mibTypeToner, mibTypeTonerCartridge:
		return SupplyToner
	case mibTypeInk, mibTypeInkCartridge, mibTypeInkRibbon:
		return SupplyInk
	case mibTypeWasteToner:
		return SupplyWasteToner
	case mibTypeWasteInk:
		return SupplyWasteInk
	case mibTypeOPC:
		return SupplyDrum
	case mibTypeFuser:
		return SupplyFuser
	default:
		return SupplyOther
	}
}

// supplyLevel converts a row's level. RFC 3805 defines the level of a
// receptacle as the space remaining, so it is turned into the percent
// filled, which is what clients display for a waste container.
func supplyLevel(row MarkerSupplyRow, receptacle bool) SupplyLevel {
	if row.Level < 0 {
		return SupplyLevel(row.Level)
	}
	percent, ok := levelPercent(row)
	if !ok {
		return SupplyLevelUnknown
	}
	if receptacle {
		percent = MaxSupplyPercent - percent
	}
	return SupplyLevel(percent)
}

// levelPercent is the level as a rounded percentage of the capacity. A
// known capacity gives the ratio whatever the unit; without one only a
// percent unit makes the level a percentage itself.
func levelPercent(row MarkerSupplyRow) (int, bool) {
	switch {
	case row.MaxCapacity > 0:
		percent := (int64(row.Level)*MaxSupplyPercent + int64(row.MaxCapacity)/2) / int64(row.MaxCapacity)
		return int(min(percent, MaxSupplyPercent)), true
	case row.Unit == mibUnitPercent && row.Level <= MaxSupplyPercent:
		return row.Level, true
	default:
		return 0, false
	}
}

// generatedSupplyName names a supply whose description is missing or
// unreadable, from its colour and kind ("Black toner", "Waste toner").
func generatedSupplyName(s Supply) string {
	kind := "supply"
	if s.Kind != SupplyOther {
		kind = strings.ReplaceAll(string(s.Kind), "-", " ")
	}
	if s.Color == "" || s.Receptacle {
		return titleCase(kind)
	}
	return titleCase(strings.ReplaceAll(s.Color, "-", " ")) + " " + kind
}

func compareSupplies(a, b rankedSupply) int {
	return cmp.Or(
		cmp.Compare(boolRank(a.Receptacle), boolRank(b.Receptacle)),
		cmp.Compare(kindRank(a.Kind), kindRank(b.Kind)),
		cmp.Compare(colorRank(a.Color), colorRank(b.Color)),
		strings.Compare(a.Color, b.Color),
		cmp.Compare(a.index, b.index),
	)
}

func boolRank(b bool) int {
	if b {
		return 1
	}
	return 0
}

// supplyKindOrder lists kinds in display order.
var supplyKindOrder = []SupplyKind{SupplyToner, SupplyInk, SupplyDrum, SupplyFuser, SupplyOther, SupplyWasteToner, SupplyWasteInk}

func kindRank(kind SupplyKind) int { return slices.Index(supplyKindOrder, kind) }

// processColorOrder is the display order of the process colours; other
// colours follow, and a supply without a colour comes last.
var processColorOrder = []string{"black", "cyan", "magenta", "yellow"}

func colorRank(color string) int {
	if color == "" {
		return len(processColorOrder) + 1
	}
	if i := slices.Index(processColorOrder, color); i >= 0 {
		return i
	}
	return len(processColorOrder)
}
