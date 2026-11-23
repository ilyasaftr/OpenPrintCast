package core

import (
	"slices"
	"strings"
	"testing"
)

// Raw values as RFC 3805 spells them: class 3 supplyThatIsConsumed, class 4
// receptacleThatIsFilled; type 3 toner, 4 wasteToner, 5 ink, 9 opc, 15
// fuser, 20 transferUnit; unit 19 percent, 13 tenthsOfGrams.
const (
	classConsumed   = 3
	classReceptacle = 4
	typeToner       = 3
	typeWasteToner  = 4
	typeInk         = 5
	typeOPC         = 9
	typeFuser       = 15
	typeTransfer    = 20
	unitPercent     = 19
	unitTenthGrams  = 13
)

func tonerRow(index int, colorant string, level, capacity, unit int) MarkerSupplyRow {
	return MarkerSupplyRow{
		Index: index, Class: classConsumed, Type: typeToner, Unit: unit,
		MaxCapacity: capacity, Level: level, Colorant: colorant,
	}
}

func TestNormalizeSuppliesPercentUnit(t *testing.T) {
	got := NormalizeSupplies([]MarkerSupplyRow{
		tonerRow(1, "black", 56, 100, unitPercent),
		tonerRow(2, "cyan", 77, 100, unitPercent),
		tonerRow(3, "magenta", 19, 100, unitPercent),
		tonerRow(4, "yellow", 31, 100, unitPercent),
	})
	want := map[string]SupplyLevel{"black": 56, "cyan": 77, "magenta": 19, "yellow": 31}
	if len(got) != len(want) {
		t.Fatalf("got %d supplies: %+v", len(got), got)
	}
	for _, s := range got {
		if s.Level != want[s.Color] || s.Kind != SupplyToner || s.Receptacle {
			t.Errorf("%s: %+v", s.Color, s)
		}
	}
}

func TestNormalizeSuppliesScalesCapacityToPercent(t *testing.T) {
	for name, tc := range map[string]struct {
		level, capacity, unit int
		want                  SupplyLevel
	}{
		"half of 3000 sheets":         {1500, 3000, 8, 50},
		"grams of toner":              {1234, 2000, unitTenthGrams, 62},
		"rounds to nearest":           {1, 3, 8, 33},
		"percent unit, capacity 100":  {7, 100, unitPercent, 7},
		"percent unit, no capacity":   {42, -2, unitPercent, 42},
		"above capacity is clamped":   {120, 100, unitPercent, 100},
		"empty":                       {0, 3000, 8, 0},
		"huge values do not overflow": {2147483647, 2147483647, 8, 100},
		"no capacity and no percent":  {42, -2, 8, SupplyLevelUnknown},
		"zero capacity":               {0, 0, 8, SupplyLevelUnknown},
		"percent unit above 100":      {150, -1, unitPercent, SupplyLevelUnknown},
	} {
		t.Run(name, func(t *testing.T) {
			got := NormalizeSupplies([]MarkerSupplyRow{tonerRow(1, "black", tc.level, tc.capacity, tc.unit)})
			if len(got) != 1 || got[0].Level != tc.want {
				t.Fatalf("got %+v, want level %d", got, tc.want)
			}
		})
	}
}

func TestNormalizeSuppliesKeepsMIBMarkers(t *testing.T) {
	for _, level := range []int{-1, -2, -3} {
		got := NormalizeSupplies([]MarkerSupplyRow{tonerRow(1, "black", level, 100, unitPercent)})
		if len(got) != 1 || got[0].Level != SupplyLevel(level) {
			t.Errorf("level %d: got %+v", level, got)
		}
		if _, ok := got[0].Level.Percent(); ok {
			t.Errorf("level %d reads as a percentage", level)
		}
	}
}

func TestNormalizeSuppliesWasteContainerIsPercentFilled(t *testing.T) {
	// RFC 3805: a receptacle's level is the space remaining, so 25 of 100
	// remaining is a container that is 75% full.
	got := NormalizeSupplies([]MarkerSupplyRow{{
		Index: 5, Class: classReceptacle, Type: typeWasteToner, Unit: unitPercent,
		MaxCapacity: 100, Level: 25, Colorant: "unknown", Description: "Waste Toner Bin",
	}})
	want := Supply{Name: "Waste Toner Bin", Kind: SupplyWasteToner, Level: 75, Receptacle: true}
	if len(got) != 1 || got[0] != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestNormalizeSuppliesDropsWhatItCannotInterpret(t *testing.T) {
	good := tonerRow(1, "black", 50, 100, unitPercent)
	for name, row := range map[string]MarkerSupplyRow{
		"level below the MIB range": tonerRow(2, "cyan", -4, 100, unitPercent),
		"class other":               {Index: 3, Class: 1, Type: typeToner, Level: 5, MaxCapacity: 100},
		"class unknown":             {Index: 4, Class: 2, Type: typeToner, Level: 5, MaxCapacity: 100},
		"class missing":             {Index: 5, Type: typeToner, Level: 5, MaxCapacity: 100},
	} {
		t.Run(name, func(t *testing.T) {
			got := NormalizeSupplies([]MarkerSupplyRow{good, row})
			if len(got) != 1 || got[0].Color != "black" {
				t.Fatalf("got %+v", got)
			}
		})
	}
	if got := NormalizeSupplies(nil); len(got) != 0 {
		t.Fatalf("no rows gave %+v", got)
	}
}

func TestNormalizeSuppliesKinds(t *testing.T) {
	for mibType, want := range map[int]SupplyKind{
		3: SupplyToner, 21: SupplyToner, 5: SupplyInk, 6: SupplyInk, 7: SupplyInk,
		4: SupplyWasteToner, 8: SupplyWasteInk, typeOPC: SupplyDrum, typeFuser: SupplyFuser,
		typeTransfer: SupplyOther, 10: SupplyOther, 1: SupplyOther, 0: SupplyOther,
	} {
		row := MarkerSupplyRow{Index: 1, Class: classConsumed, Type: mibType, MaxCapacity: 100, Level: 50}
		if got := NormalizeSupplies([]MarkerSupplyRow{row}); len(got) != 1 || got[0].Kind != want {
			t.Errorf("type %d: got %+v, want kind %s", mibType, got, want)
		}
	}
}

func TestNormalizeSuppliesColorants(t *testing.T) {
	for raw, want := range map[string]string{
		"cyan": "cyan", "Light Cyan": "light-cyan", "LIGHT_MAGENTA": "light-magenta",
		"  black  ": "black", "unknown": "", "other": "", "no-color": "", "": "", "???": "",
		"matte black!": "matte-black",
	} {
		got := NormalizeSupplies([]MarkerSupplyRow{tonerRow(1, raw, 50, 100, unitPercent)})
		if len(got) != 1 || got[0].Color != want {
			t.Errorf("colorant %q: got %+v, want %q", raw, got, want)
		}
	}
}

func TestNormalizeSuppliesNames(t *testing.T) {
	for name, tc := range map[string]struct {
		row  MarkerSupplyRow
		want string
	}{
		"description is kept": {MarkerSupplyRow{Description: "Black Toner Cartridge S/N:1", Colorant: "black"}, "Black Toner Cartridge S/N:1"},
		"controls are spaces": {MarkerSupplyRow{Description: "Cyan\x00Toner\r\n  Cartridge"}, "Cyan Toner Cartridge"},
		"legacy code page":    {MarkerSupplyRow{Description: "Schwarz Tonerkassette \xe4", Colorant: "black"}, "Black toner"},
		"empty":               {MarkerSupplyRow{Colorant: "light-cyan"}, "Light cyan toner"},
		"no colour":           {MarkerSupplyRow{}, "Toner"},
		"long is cut":         {MarkerSupplyRow{Description: strings.Repeat("ä", 300)}, strings.Repeat("ä", maxSupplyNameRunes)},
	} {
		t.Run(name, func(t *testing.T) {
			tc.row.Index, tc.row.Class, tc.row.Type, tc.row.MaxCapacity, tc.row.Level = 1, classConsumed, typeToner, 100, 50
			got := NormalizeSupplies([]MarkerSupplyRow{tc.row})
			if len(got) != 1 || got[0].Name != tc.want {
				t.Fatalf("got %+v, want name %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeSuppliesWasteWithoutDescriptionIsNamedByKind(t *testing.T) {
	row := MarkerSupplyRow{Index: 1, Class: classReceptacle, Type: typeWasteToner, Colorant: "black", MaxCapacity: 100, Level: 10}
	if got := NormalizeSupplies([]MarkerSupplyRow{row}); len(got) != 1 || got[0].Name != "Waste toner" {
		t.Fatalf("got %+v", got)
	}
}

func TestNormalizeSuppliesOrderIsDeterministic(t *testing.T) {
	rows := []MarkerSupplyRow{
		{Index: 9, Class: classReceptacle, Type: typeWasteToner, MaxCapacity: 100, Level: 90},
		tonerRow(7, "yellow", 31, 100, unitPercent),
		{Index: 8, Class: classConsumed, Type: typeFuser, MaxCapacity: 100, Level: 89},
		tonerRow(4, "light-cyan", 50, 100, unitPercent),
		tonerRow(3, "cyan", 77, 100, unitPercent),
		{Index: 6, Class: classConsumed, Type: typeInk, MaxCapacity: 100, Level: 60, Colorant: "black"},
		tonerRow(2, "black", 56, 100, unitPercent),
		tonerRow(5, "magenta", 19, 100, unitPercent),
		{Index: 1, Class: classConsumed, Type: typeOPC, MaxCapacity: 100, Level: 40},
	}
	order := func(rows []MarkerSupplyRow) []string {
		var out []string
		for _, s := range NormalizeSupplies(rows) {
			out = append(out, string(s.Kind)+"/"+s.Color)
		}
		return out
	}
	want := []string{
		"toner/black", "toner/cyan", "toner/magenta", "toner/yellow", "toner/light-cyan",
		"ink/black", "drum/", "fuser/", "waste-toner/",
	}
	if got := order(rows); !slices.Equal(got, want) {
		t.Fatalf("order = %v\nwant    %v", got, want)
	}
	reversed := slices.Clone(rows)
	slices.Reverse(reversed)
	if got := order(reversed); !slices.Equal(got, want) {
		t.Fatalf("reversed input changed the order: %v", got)
	}
}

func TestSupplyStateReasons(t *testing.T) {
	toner := func(level SupplyLevel) Supply { return Supply{Kind: SupplyToner, Level: level} }
	for name, tc := range map[string]struct {
		supplies []Supply
		want     []string
	}{
		"none":                    {nil, nil},
		"plenty":                  {[]Supply{toner(11), toner(100)}, nil},
		"exactly at the limit":    {[]Supply{toner(SupplyLowPercent)}, []string{ReasonSupplyLowWarning}},
		"empty":                   {[]Supply{toner(0)}, []string{ReasonSupplyEmptyError}},
		"empty beats low":         {[]Supply{toner(5), toner(0)}, []string{ReasonSupplyEmptyError}},
		"markers say nothing":     {[]Supply{toner(SupplyLevelUnknown), toner(SupplyLevelSomeRemaining), toner(SupplyLevelOther)}, nil},
		"ink counts":              {[]Supply{{Kind: SupplyInk, Level: 3}}, []string{ReasonSupplyLowWarning}},
		"drum and fuser do not":   {[]Supply{{Kind: SupplyDrum, Level: 0}, {Kind: SupplyFuser, Level: 1}}, nil},
		"waste is never reported": {[]Supply{{Kind: SupplyWasteToner, Level: 0, Receptacle: true}, {Kind: SupplyToner, Level: 0, Receptacle: true}}, nil},
	} {
		t.Run(name, func(t *testing.T) {
			if got := SupplyStateReasons(tc.supplies); !slices.Equal(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSupplyLabelAndColorantHex(t *testing.T) {
	for _, tc := range []struct {
		supply Supply
		want   string
	}{
		{Supply{Name: "Black Toner Cartridge", Color: "black", Kind: SupplyToner}, "Black"},
		{Supply{Name: "x", Color: "light-cyan", Kind: SupplyInk}, "Light cyan"},
		{Supply{Name: "Waste Toner Bin", Kind: SupplyWasteToner, Receptacle: true}, "Waste Toner Bin"},
		{Supply{Name: "Fuser Kit", Kind: SupplyFuser}, "Fuser Kit"},
	} {
		if got := tc.supply.Label(); got != tc.want {
			t.Errorf("Label(%+v) = %q, want %q", tc.supply, got, tc.want)
		}
	}
	for name, want := range map[string]string{"cyan": "#00FFFF", "black": "#000000", "light-magenta": "#FF77FF", "matte-black": "", "": ""} {
		if got := ColorantHex(name); got != want {
			t.Errorf("ColorantHex(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestConfigReadSupplyLevels(t *testing.T) {
	if !DefaultConfig().ReadSupplyLevels {
		t.Fatal("supply levels should be read by default")
	}
	for name, tc := range map[string]struct {
		content string
		want    bool
	}{
		"key absent keeps default": {`{"max_render_dpi": 600}`, true},
		"explicit true":            {`{"read_supply_levels": true}`, true},
		"explicit false":           {`{"read_supply_levels": false}`, false},
		"malformed keeps default":  {`{"read_supply_levels": "no"}`, true},
	} {
		t.Run(name, func(t *testing.T) {
			if got := loadConfigJSON(t, tc.content).ReadSupplyLevels; got != tc.want {
				t.Fatalf("ReadSupplyLevels = %v, want %v", got, tc.want)
			}
		})
	}
}
