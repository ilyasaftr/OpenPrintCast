//go:build windows

package winspool

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modGDI               = windows.NewLazySystemDLL("gdi32.dll")
	procCreateDC         = modGDI.NewProc("CreateDCW")
	procDeleteDC         = modGDI.NewProc("DeleteDC")
	procGetDeviceCaps    = modGDI.NewProc("GetDeviceCaps")
	procStartDoc         = modGDI.NewProc("StartDocW")
	procStartPage        = modGDI.NewProc("StartPage")
	procStretchDIBits    = modGDI.NewProc("StretchDIBits")
	procCreateDIBSection = modGDI.NewProc("CreateDIBSection")
	procDeleteObject     = modGDI.NewProc("DeleteObject")
	procEndPage          = modGDI.NewProc("EndPage")
	procEndDoc           = modGDI.NewProc("EndDoc")
	procAbortDoc         = modGDI.NewProc("AbortDoc")
)

// GetDeviceCaps indexes (from WINGDI.H).
const (
	gdiHorzRes         = 8
	gdiVertRes         = 10
	gdiLogPixelsX      = 88
	gdiLogPixelsY      = 90
	gdiPhysicalWidth   = 110
	gdiPhysicalHeight  = 111
	gdiPhysicalOffsetX = 112
	gdiPhysicalOffsetY = 113
)

func gdiError(op string, err error) error {
	if err == nil || errors.Is(err, syscall.Errno(0)) {
		return fmt.Errorf("%s failed without a system error", op)
	}
	return fmt.Errorf("%s: %w", op, err)
}

type docInfoW struct {
	size      int32
	name      *uint16
	output    *uint16
	datatype  *uint16
	typeFlags uint32
}

type bitmapInfoHeader struct {
	size          uint32
	width         int32
	height        int32
	planes        uint16
	bitCount      uint16
	compression   uint32
	sizeImage     uint32
	xPelsPerMeter int32
	yPelsPerMeter int32
	clrUsed       uint32
	clrImportant  uint32
}

// createPrinterDC creates a device context for the printer using the
// DEVMODE in mode, which must not be empty.
func createPrinterDC(name *uint16, mode []byte) (uintptr, error) {
	dc, _, errno := procCreateDC.Call(0, uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&mode[0])))
	runtime.KeepAlive(mode)
	return dc, errno
}

// createDefaultPrinterDC creates a device context with the driver's
// default DEVMODE.
func createDefaultPrinterDC(name *uint16) (uintptr, error) {
	dc, _, errno := procCreateDC.Call(0, uintptr(unsafe.Pointer(name)), 0, 0)
	return dc, errno
}

// deviceCaps reads one GetDeviceCaps value from dc.
func deviceCaps(dc uintptr, index uintptr) int {
	value, _, _ := procGetDeviceCaps.Call(dc, index)
	return int(int32(value))
}
