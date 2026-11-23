package winspool

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// fakeWalker answers walks from canned values and records them. It never
// touches a network.
type fakeWalker struct {
	tables map[string][]snmpVar
	errs   map[string]error
	walked []string
}

func (f *fakeWalker) Walk(_ context.Context, root string) ([]snmpVar, error) {
	f.walked = append(f.walked, root)
	if err := f.errs[root]; err != nil {
		return nil, err
	}
	return f.tables[root], nil
}

// supplyOID is the name of prtMarkerSuppliesEntry column of row (device, index).
func supplyOID(column, device, index int) string {
	return fmt.Sprintf("%s.%d.%d.%d", oidMarkerSuppliesEntry, column, device, index)
}

func intVar(oid string, value int) snmpVar { return snmpVar{OID: oid, Integer: value} }
func textVar(oid, value string) snmpVar    { return snmpVar{OID: oid, Text: value, IsText: true} }
func colorantOID(device, index int) string {
	return fmt.Sprintf("%s.%d.%d", oidMarkerColorantValue, device, index)
}
func (f *fakeWalker) add(root string, v ...snmpVar) { f.tables[root] = append(f.tables[root], v...) }

// supplyRow adds one prtMarkerSuppliesTable row of device 1 in the shape of
// the RFC 3805 / PWG 5100.13 examples.
func (f *fakeWalker) supplyRow(index, colorant, class, mibType, unit, capacity, level int, description string) {
	columns := map[int]int{
		supplyColumnColorantIndex: colorant, supplyColumnClass: class, supplyColumnType: mibType,
		supplyColumnUnit: unit, supplyColumnMaxCapacity: capacity, supplyColumnLevel: level,
	}
	for _, column := range []int{
		supplyColumnColorantIndex, supplyColumnClass, supplyColumnType, supplyColumnUnit,
		supplyColumnMaxCapacity, supplyColumnLevel,
	} {
		f.add(oidMarkerSuppliesEntry, intVar(supplyOID(column, 1, index), columns[column]))
	}
	f.add(oidMarkerSuppliesEntry, textVar(supplyOID(supplyColumnDescription, 1, index), description))
}

func newFakeWalker() *fakeWalker {
	return &fakeWalker{tables: map[string][]snmpVar{}, errs: map[string]error{}}
}

// cmykPrinter is the PWG 5100.13 example (figure 12): CMYK toner in
// percent plus a waste bin that has 33% space left.
func cmykPrinter() *fakeWalker {
	f := newFakeWalker()
	const percent, consumed, receptacle, toner, wasteToner = 19, 3, 4, 3, 4
	f.supplyRow(1, 1, consumed, toner, percent, 100, 56, "Black Toner Cartridge S/N:16859422")
	f.supplyRow(2, 2, consumed, toner, percent, 100, 77, "Cyan Toner Cartridge S/N:16852765")
	f.supplyRow(3, 3, consumed, toner, percent, 100, 19, "Magenta Toner Cartridge S/N:16859681")
	f.supplyRow(4, 4, consumed, toner, percent, 100, 31, "Yellow Toner Cartridge S/N:16859372")
	f.supplyRow(5, 0, receptacle, wasteToner, percent, 100, 33, "Waste Toner Bin S/N:16816815")
	for i, name := range []string{"black", "cyan", "magenta", "yellow"} {
		f.add(oidMarkerColorantValue, textVar(colorantOID(1, i+1), name))
	}
	return f
}

func TestReadSuppliesParsesTheRFCTables(t *testing.T) {
	walker := cmykPrinter()
	got, err := readSupplies(context.Background(), walker, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := []core.Supply{
		{Name: "Black Toner Cartridge S/N:16859422", Color: "black", Kind: core.SupplyToner, Level: 56},
		{Name: "Cyan Toner Cartridge S/N:16852765", Color: "cyan", Kind: core.SupplyToner, Level: 77},
		{Name: "Magenta Toner Cartridge S/N:16859681", Color: "magenta", Kind: core.SupplyToner, Level: 19},
		{Name: "Yellow Toner Cartridge S/N:16859372", Color: "yellow", Kind: core.SupplyToner, Level: 31},
		{Name: "Waste Toner Bin S/N:16816815", Kind: core.SupplyWasteToner, Level: 67, Receptacle: true},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
	if wantWalks := []string{oidMarkerSuppliesEntry, oidMarkerColorantValue}; !slices.Equal(walker.walked, wantWalks) {
		t.Fatalf("walked %v, want %v", walker.walked, wantWalks)
	}
}

func TestReadSuppliesIgnoresOtherDevices(t *testing.T) {
	walker := cmykPrinter()
	// A print server's second device has its own rows under device index 2.
	walker.add(oidMarkerSuppliesEntry,
		intVar(supplyOID(supplyColumnColorantIndex, 2, 1), 1),
		intVar(supplyOID(supplyColumnClass, 2, 1), 3), intVar(supplyOID(supplyColumnType, 2, 1), 3),
		intVar(supplyOID(supplyColumnLevel, 2, 1), 5), intVar(supplyOID(supplyColumnMaxCapacity, 2, 1), 100))
	walker.add(oidMarkerColorantValue, textVar(colorantOID(2, 1), "red"))
	got, err := readSupplies(context.Background(), walker, 1)
	if err != nil || len(got) != 5 {
		t.Fatalf("device 1: %d supplies, %v", len(got), err)
	}
	got, err = readSupplies(context.Background(), walker, 2)
	if err != nil || len(got) != 1 || got[0].Level != 5 || got[0].Color != "red" {
		t.Fatalf("device 2: %+v, %v", got, err)
	}
}

func TestReadSuppliesWithMissingColorants(t *testing.T) {
	walker := cmykPrinter()
	delete(walker.tables, oidMarkerColorantValue)
	got, err := readSupplies(context.Background(), walker, 1)
	if err != nil || len(got) != 5 {
		t.Fatalf("%d supplies, %v", len(got), err)
	}
	for _, s := range got {
		if s.Color != "" {
			t.Errorf("%s has colour %q without a colorant table", s.Name, s.Color)
		}
	}
}

func TestReadSuppliesPartialRows(t *testing.T) {
	walker := newFakeWalker()
	// Row 1 has no level column: dropped. Row 2 has a level but no class.
	// Row 3 lacks capacity and unit: its level cannot be a percentage.
	walker.add(oidMarkerSuppliesEntry,
		intVar(supplyOID(supplyColumnClass, 1, 1), 3), intVar(supplyOID(supplyColumnType, 1, 1), 3),
		intVar(supplyOID(supplyColumnLevel, 1, 2), 40), intVar(supplyOID(supplyColumnType, 1, 2), 3),
		intVar(supplyOID(supplyColumnClass, 1, 3), 3), intVar(supplyOID(supplyColumnType, 1, 3), 5),
		intVar(supplyOID(supplyColumnLevel, 1, 3), 700))
	got, err := readSupplies(context.Background(), walker, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Kind != core.SupplyInk || got[0].Level != core.SupplyLevelUnknown {
		t.Fatalf("got %+v", got)
	}
}

func TestReadSuppliesIgnoresMalformedAndForeignValues(t *testing.T) {
	walker := cmykPrinter()
	walker.add(oidMarkerSuppliesEntry,
		intVar(oidMarkerSuppliesEntry+".x.1.1", 1), intVar(oidMarkerSuppliesEntry+".9.1", 1),
		intVar("1.3.6.1.2.1.43.11.1.2.9.1.1", 1), intVar(oidMarkerSuppliesEntry+".9.1.-1", 1),
		// A text where an integer belongs counts as missing.
		textVar(supplyOID(supplyColumnLevel, 1, 9), "full"))
	walker.add(oidMarkerColorantValue, intVar(colorantOID(1, 9), 3), textVar(oidMarkerColorantValue+".1", "bad"))
	got, err := readSupplies(context.Background(), walker, 1)
	if err != nil || len(got) != 5 {
		t.Fatalf("%d supplies, %v", len(got), err)
	}
}

func TestReadSuppliesWalkErrors(t *testing.T) {
	boom := errors.New("request timeout")
	for name, root := range map[string]string{"supplies": oidMarkerSuppliesEntry, "colorants": oidMarkerColorantValue} {
		t.Run(name, func(t *testing.T) {
			walker := cmykPrinter()
			walker.errs[root] = boom
			got, err := readSupplies(context.Background(), walker, 1)
			if !errors.Is(err, boom) || got != nil {
				t.Fatalf("got %+v, %v: a failed read must give no levels", got, err)
			}
		})
	}
}

func TestReadSuppliesPrinterWithoutTable(t *testing.T) {
	walker := newFakeWalker()
	got, err := readSupplies(context.Background(), walker, 1)
	if err != nil || got != nil {
		t.Fatalf("got %+v, %v", got, err)
	}
	if len(walker.walked) != 1 {
		t.Fatalf("walked %v: no colorants are needed without supplies", walker.walked)
	}
}

func TestReadSuppliesBoundsTheNumberOfRows(t *testing.T) {
	walker := newFakeWalker()
	for i := 1; i <= maxSupplyRows+20; i++ {
		walker.add(oidMarkerSuppliesEntry,
			intVar(supplyOID(supplyColumnClass, 1, i), 3), intVar(supplyOID(supplyColumnLevel, 1, i), 50),
			intVar(supplyOID(supplyColumnMaxCapacity, 1, i), 100))
	}
	got, err := readSupplies(context.Background(), walker, 1)
	if err != nil || len(got) != maxSupplyRows {
		t.Fatalf("%d supplies, %v", len(got), err)
	}
}

func TestSplitTableOID(t *testing.T) {
	for _, tc := range []struct {
		oid                   string
		column, device, index int
		ok                    bool
	}{
		{supplyOID(9, 1, 4), 9, 1, 4, true},
		{supplyOID(9, 12, 345), 9, 12, 345, true},
		{oidMarkerSuppliesEntry + ".9.1", 0, 0, 0, false},
		{oidMarkerSuppliesEntry + ".9.1.4.7", 0, 0, 0, false},
		{oidMarkerSuppliesEntry + ".9.1.", 0, 0, 0, false},
		{oidMarkerSuppliesEntry, 0, 0, 0, false},
		{strings.TrimSuffix(supplyOID(9, 1, 4), "4") + "x", 0, 0, 0, false},
	} {
		column, device, index, ok := splitTableOID(tc.oid, oidMarkerSuppliesEntry)
		if ok != tc.ok || column != tc.column || device != tc.device || index != tc.index {
			t.Errorf("%s: got %d %d %d %v", tc.oid, column, device, index, ok)
		}
	}
}
