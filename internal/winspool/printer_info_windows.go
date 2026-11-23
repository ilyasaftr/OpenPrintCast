//go:build windows

package winspool

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// ptrSize is the WinSpool pointer width on this architecture; it drives
// PRINTER_INFO_2W record layout below.
var ptrSize = unsafe.Sizeof(uintptr(0))

// maxPrinterInfoBytes bounds a GetPrinterW buffer so a corrupt size
// report cannot trigger a huge allocation.
const maxPrinterInfoBytes = 16 << 20

// printerInfo2Size is the byte size of one level-2 PRINTER_INFO_2W
// record: 13 string/security pointers followed by 8 DWORDs
// (Attributes, Priority, DefaultPriority, StartTime, UntilTime, Status,
// cJobs, AveragePPM).
func printerInfo2Size() uintptr { return 13*ptrSize + 8*4 }

// printerInfo2StatusOffset is the byte offset of the Status DWORD within
// a level-2 record: 13 pointers plus 5 DWORDs precede it.
func printerInfo2StatusOffset() uintptr { return 13*ptrSize + 5*4 }

// printerInfo2StringField reads the LPWSTR at pointer slot index of the
// record at rec.
func printerInfo2StringField(rec unsafe.Pointer, index uintptr) string {
	namePtr := *(**uint16)(unsafe.Add(rec, index*ptrSize))
	if namePtr == nil {
		return ""
	}
	return utf16PtrToString(unsafe.Pointer(namePtr))
}

// printerInfo2Name reads the pPrinterName field (second pointer) of the
// record at rec.
func printerInfo2Name(rec unsafe.Pointer) string {
	return printerInfo2StringField(rec, 1)
}

// printerInfo2Port reads the pPortName field (fourth pointer) of the
// record at rec.
func printerInfo2Port(rec unsafe.Pointer) string {
	return printerInfo2StringField(rec, 3)
}

// printerInfo2Attributes reads the Attributes DWORD, the first DWORD after
// the 13 pointers, of the record at rec.
func printerInfo2Attributes(rec unsafe.Pointer) uint32 {
	return *(*uint32)(unsafe.Add(rec, 13*ptrSize))
}

// printerInfo2Driver reads the pDriverName field (fifth pointer) of the
// record at rec.
func printerInfo2Driver(rec unsafe.Pointer) string {
	return printerInfo2StringField(rec, 4)
}

// utf16PtrToString reads a NUL-terminated UTF-16 string at p, capping
// the scan so a corrupt pointer cannot run away. Pointer arithmetic uses
// unsafe.Add throughout so no uintptr round-trip exists.
func utf16PtrToString(p unsafe.Pointer) string {
	const maxChars = 1 << 20
	n := 0
	for n < maxChars {
		if *(*uint16)(unsafe.Add(p, n*2)) == 0 {
			break
		}
		n++
	}
	if n == 0 {
		return ""
	}
	return windows.UTF16ToString(unsafe.Slice((*uint16)(p), n))
}

// printerInfo2Record is a standalone level-2 PRINTER_INFO_2W buffer as
// filled by GetPrinterW; its string fields point back into the buffer.
type printerInfo2Record []byte

// pointerField reads the pointer at slot index of the record.
func (r printerInfo2Record) pointerField(index uintptr) *uint16 {
	return *(**uint16)(unsafe.Pointer(&r[index*ptrSize]))
}

// port is the pPortName field (fourth pointer).
func (r printerInfo2Record) port() *uint16 { return r.pointerField(3) }

// driver is the pDriverName field (fifth pointer).
func (r printerInfo2Record) driver() *uint16 { return r.pointerField(4) }

// status reads the Status DWORD, reporting false if the buffer is too short.
func (r printerInfo2Record) status() (uint32, bool) {
	off := printerInfo2StatusOffset()
	if off+4 > uintptr(len(r)) {
		return 0, false
	}
	return *(*uint32)(unsafe.Pointer(&r[off])), true
}

// printerInfo2Needed asks GetPrinterW (level 2) for the required byte
// count; the sizing call is expected to fail.
func printerInfo2Needed(handle uintptr) uint32 {
	var needed uint32
	_, _, _ = procGetPrinterW.Call(handle, 2, 0, 0, uintptr(unsafe.Pointer(&needed)))
	return needed
}

// printerInfo2SizeValid reports whether needed is a plausible level-2
// record size.
func printerInfo2SizeValid(needed uint32) bool {
	return needed >= uint32(printerInfo2Size()) && needed <= maxPrinterInfoBytes
}

// readPrinterInfo2 fills a needed-byte level-2 record (needed > 0); on
// failure the error is the GetPrinterW system error.
func readPrinterInfo2(handle uintptr, needed uint32) (printerInfo2Record, error) {
	buf := make([]byte, needed)
	result, _, errno := procGetPrinterW.Call(handle, 2, uintptr(unsafe.Pointer(&buf[0])), uintptr(needed), uintptr(unsafe.Pointer(&needed)))
	if result == 0 {
		return nil, errno
	}
	return buf, nil
}

// fetchPrinterInfo2 reads a size-checked level-2 record for handle.
func fetchPrinterInfo2(handle uintptr) (printerInfo2Record, bool) {
	needed := printerInfo2Needed(handle)
	if !printerInfo2SizeValid(needed) {
		return nil, false
	}
	record, err := readPrinterInfo2(handle, needed)
	return record, err == nil
}
