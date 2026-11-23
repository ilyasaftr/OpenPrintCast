package winspool

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// Printer MIB v2 (RFC 3805) object identifiers, written without the
// leading dot. Both tables are indexed by hrDeviceIndex and then by their
// own index, so a value's name ends in <hrDeviceIndex>.<index>.
const (
	// oidMarkerSuppliesEntry is prtMarkerSuppliesEntry; its columns follow.
	oidMarkerSuppliesEntry = "1.3.6.1.2.1.43.11.1.1"
	// oidMarkerColorantValue is prtMarkerColorantValue, the colorant name.
	oidMarkerColorantValue = "1.3.6.1.2.1.43.12.1.1.4"
)

// Columns of prtMarkerSuppliesEntry.
const (
	supplyColumnColorantIndex = 3
	supplyColumnClass         = 4
	supplyColumnType          = 5
	supplyColumnDescription   = 6
	supplyColumnUnit          = 7
	supplyColumnMaxCapacity   = 8
	supplyColumnLevel         = 9
)

// Bounds on what one printer may make the bridge hold: the Printer MIB
// tables of real printers have a few dozen rows at most.
const (
	maxSNMPVariables = 2048
	maxSupplyRows    = 64
)

// mibUnknown is the Printer MIB's "unknown" for the numeric columns it
// stands in for when a row lacks one.
const mibUnknown = -2

// snmpVar is one value of an SNMP walk, in the two shapes the Printer MIB
// uses: an integer or an octet string.
type snmpVar struct {
	OID     string // numeric, dotted, without a leading dot
	Integer int
	Text    string
	IsText  bool
}

// snmpWalker is the part of an SNMP client the supply reader needs: the
// values below a root OID, in order. Production uses gosnmp (see
// supply_gosnmp.go); tests use a fake.
type snmpWalker interface {
	Walk(ctx context.Context, rootOID string) ([]snmpVar, error)
}

// readSupplies reads the printer's marker supplies and colorants through
// walker and normalizes them. deviceIndex is the printer's hrDeviceIndex
// (the "SNMP Index" of its Windows port); rows of other devices, such as
// the other ports of a print server, are ignored. A printer without a
// supplies table gives no supplies and no error.
func readSupplies(ctx context.Context, walker snmpWalker, deviceIndex int) ([]core.Supply, error) {
	supplyVars, err := walker.Walk(ctx, oidMarkerSuppliesEntry)
	if err != nil {
		return nil, fmt.Errorf("read prtMarkerSuppliesTable: %w", err)
	}
	rows, colorantIndexes := parseSupplyRows(supplyVars, deviceIndex)
	if len(rows) == 0 {
		return nil, nil
	}
	colorantVars, err := walker.Walk(ctx, oidMarkerColorantValue)
	if err != nil {
		return nil, fmt.Errorf("read prtMarkerColorantTable: %w", err)
	}
	colorants := parseColorants(colorantVars, deviceIndex)
	for i := range rows {
		rows[i].Colorant = colorants[colorantIndexes[rows[i].Index]]
	}
	return core.NormalizeSupplies(rows), nil
}

// supplyColumns is one row of prtMarkerSuppliesTable being assembled.
type supplyColumns map[int]snmpVar

// parseSupplyRows groups the walk of prtMarkerSuppliesEntry into rows of
// deviceIndex, and returns each row's prtMarkerSuppliesColorantIndex by
// row index. A row without a level is dropped (there is nothing to show);
// any other missing column reads as the MIB's "unknown", which
// core.NormalizeSupplies then refuses to guess at.
func parseSupplyRows(vars []snmpVar, deviceIndex int) (rows []core.MarkerSupplyRow, colorantIndexes map[int]int) {
	byIndex := make(map[int]supplyColumns)
	for _, v := range vars {
		column, device, index, ok := splitTableOID(v.OID, oidMarkerSuppliesEntry)
		if !ok || device != deviceIndex || (len(byIndex) >= maxSupplyRows && byIndex[index] == nil) {
			continue
		}
		if byIndex[index] == nil {
			byIndex[index] = make(supplyColumns)
		}
		byIndex[index][column] = v
	}
	colorantIndexes = make(map[int]int, len(byIndex))
	for index, columns := range byIndex {
		if _, hasLevel := columns.integer(supplyColumnLevel); !hasLevel {
			continue
		}
		rows = append(rows, columns.row(index))
		colorantIndexes[index], _ = columns.integer(supplyColumnColorantIndex)
	}
	return rows, colorantIndexes
}

func (c supplyColumns) row(index int) core.MarkerSupplyRow {
	return core.MarkerSupplyRow{
		Index:       index,
		Class:       c.integerOr(supplyColumnClass, mibUnknown),
		Type:        c.integerOr(supplyColumnType, mibUnknown),
		Unit:        c.integerOr(supplyColumnUnit, mibUnknown),
		MaxCapacity: c.integerOr(supplyColumnMaxCapacity, mibUnknown),
		Level:       c.integerOr(supplyColumnLevel, mibUnknown),
		Description: c.text(supplyColumnDescription),
	}
}

// integer is the integer in column, if it has one.
func (c supplyColumns) integer(column int) (int, bool) {
	v, ok := c[column]
	return v.Integer, ok && !v.IsText
}

func (c supplyColumns) integerOr(column, fallback int) int {
	if value, ok := c.integer(column); ok {
		return value
	}
	return fallback
}

func (c supplyColumns) text(column int) string {
	if v, ok := c[column]; ok && v.IsText {
		return v.Text
	}
	return ""
}

// parseColorants maps prtMarkerColorantIndex to the colorant name, for
// deviceIndex.
func parseColorants(vars []snmpVar, deviceIndex int) map[int]string {
	colorants := make(map[int]string)
	for _, v := range vars {
		device, index, ok := splitIndexOID(v.OID, oidMarkerColorantValue)
		if ok && device == deviceIndex && v.IsText {
			colorants[index] = v.Text
		}
	}
	return colorants
}

// splitTableOID splits <entry>.<column>.<device>.<index>.
func splitTableOID(oid, entry string) (column, device, index int, ok bool) {
	rest, found := strings.CutPrefix(oid, entry+".")
	if !found {
		return 0, 0, 0, false
	}
	column, rest, ok = cutNumber(rest)
	if !ok {
		return 0, 0, 0, false
	}
	if device, index, ok = splitIndexOID(rest, ""); !ok {
		return 0, 0, 0, false
	}
	return column, device, index, true
}

// splitIndexOID splits <prefix>.<device>.<index>; an empty prefix means
// the OID is just <device>.<index>.
func splitIndexOID(oid, prefix string) (device, index int, ok bool) {
	if prefix != "" {
		var found bool
		if oid, found = strings.CutPrefix(oid, prefix+"."); !found {
			return 0, 0, false
		}
	}
	device, rest, ok := cutNumber(oid)
	if !ok {
		return 0, 0, false
	}
	index, err := strconv.Atoi(rest)
	if err != nil || index < 0 {
		return 0, 0, false
	}
	return device, index, true
}

// cutNumber reads the number before the first dot and returns the rest.
func cutNumber(oid string) (number int, rest string, ok bool) {
	head, rest, _ := strings.Cut(oid, ".")
	number, err := strconv.Atoi(head)
	if err != nil || number < 0 || rest == "" {
		return 0, "", false
	}
	return number, rest, true
}
