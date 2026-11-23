//go:build windows

package winspool

import (
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// EnumPrinters scope flags (from WINGDI.H).
const (
	printerEnumLocal       = 0x00000002
	printerEnumConnections = 0x00000004
)

// PRINTER_INFO_2W Attributes flags (from WINSPOOL.H).
const (
	printerAttributeNetwork = 0x00000010
	printerAttributeFax     = 0x00004000
	printerAttributeTS      = 0x00008000
)

// defaultPrinterName returns the best-effort default printer name via
// GetDefaultPrinterW, or "" when it cannot be determined.
func defaultPrinterName() string {
	var size uint32
	// Size query first; failure harvests the required character count.
	_, _, _ = procGetDefaultPrinterW.Call(0, uintptr(unsafe.Pointer(&size)))
	if size == 0 {
		return ""
	}
	buf := make([]uint16, size)
	r1, _, _ := procGetDefaultPrinterW.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if r1 == 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}

// enumPrinterRecords fills a buffer with level-2 PRINTER_INFO_2W records
// for local printers and connections and reports how many it holds.
func enumPrinterRecords() ([]byte, uint32, bool) {
	flags := uintptr(printerEnumLocal | printerEnumConnections)
	var needed, returned uint32
	// First call sizes the buffer; it is expected to fail. Only the byte
	// count is meaningful here: Windows reports zero enumerated printers
	// until the second (real) call, so checking `returned` would always
	// bail out and list nothing.
	_, _, _ = procEnumPrintersW.Call(flags, 0, 2, 0, 0,
		uintptr(unsafe.Pointer(&needed)),
		uintptr(unsafe.Pointer(&returned)))
	if needed == 0 {
		return nil, 0, false
	}
	buf := make([]byte, needed)
	var needed2, returned2 uint32
	r1, _, _ := procEnumPrintersW.Call(flags, 0, 2,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(needed),
		uintptr(unsafe.Pointer(&needed2)),
		uintptr(unsafe.Pointer(&returned2)))
	if r1 == 0 || returned2 == 0 {
		return nil, 0, false
	}
	return buf, returned2, true
}

// printerEvidenceFromRecord reads the classifier's evidence from the
// level-2 record at record.
func printerEvidenceFromRecord(record unsafe.Pointer) PrinterEvidence {
	attributes := printerInfo2Attributes(record)
	return PrinterEvidence{
		Name:         printerInfo2Name(record),
		DriverName:   printerInfo2Driver(record),
		PortName:     printerInfo2Port(record),
		IsFax:        attributes&printerAttributeFax != 0,
		IsNetwork:    attributes&printerAttributeNetwork != 0,
		IsRedirected: attributes&printerAttributeTS != 0,
	}
}

// printerDescriptorFromRecord converts the level-2 record at record, or
// reports false for an unnamed printer.
func printerDescriptorFromRecord(record unsafe.Pointer, defName string) (PrinterDescriptor, bool) {
	evidence := printerEvidenceFromRecord(record)
	if strings.TrimSpace(evidence.Name) == "" {
		return PrinterDescriptor{}, false
	}
	isDefault := defName != "" && strings.EqualFold(evidence.Name, defName)
	return NewPrinterDescriptorFromEvidence(evidence, isDefault), true
}

// platformList enumerates installed printers (local + connections) via
// EnumPrintersW (level 2) plus GetDefaultPrinterW for the default flag.
// It returns raw descriptors; sorting happens in SystemBackend.ListPrinters.
// Any spooler failure yields an empty slice, never an error.
func platformList() []PrinterDescriptor {
	buf, returned, ok := enumPrinterRecords()
	if !ok {
		return nil
	}
	recSize := printerInfo2Size()
	defName := defaultPrinterName()
	out := make([]PrinterDescriptor, 0, returned)
	for i := uint32(0); i < returned; i++ {
		off := uintptr(i) * recSize
		if off+recSize > uintptr(len(buf)) {
			break
		}
		descriptor, ok := printerDescriptorFromRecord(unsafe.Pointer(&buf[off]), defName)
		if !ok {
			continue
		}
		out = append(out, descriptor)
	}
	runtime.KeepAlive(buf)
	return out
}
