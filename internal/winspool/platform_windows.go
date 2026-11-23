//go:build windows

package winspool

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// WinSpool entry points in winspool.drv, loaded lazily so no cgo and no
// link-time dependency beyond what LazyDLL resolves at first call.
var (
	modWinspool            = windows.NewLazySystemDLL("winspool.drv")
	procEnumPrintersW      = modWinspool.NewProc("EnumPrintersW")
	procGetDefaultPrinterW = modWinspool.NewProc("GetDefaultPrinterW")
	procOpenPrinterW       = modWinspool.NewProc("OpenPrinterW")
	procStartDocPrinterW   = modWinspool.NewProc("StartDocPrinterW")
	procWritePrinter       = modWinspool.NewProc("WritePrinter")
	procEndDocPrinter      = modWinspool.NewProc("EndDocPrinter")
	procClosePrinter       = modWinspool.NewProc("ClosePrinter")
	procGetPrinterW        = modWinspool.NewProc("GetPrinterW")
	procGetPrinterDataExW  = modWinspool.NewProc("GetPrinterDataExW")
	procDeviceCapabilities = modWinspool.NewProc("DeviceCapabilitiesW")
	procDocumentProperties = modWinspool.NewProc("DocumentPropertiesW")
	procMessageBeep        = windows.NewLazySystemDLL("user32.dll").NewProc("MessageBeep")
)

// openPrinter opens printer and returns its handle; the caller owns
// exactly one ClosePrinter for it.
func openPrinter(printer string) (uintptr, error) {
	namePtr, err := windows.UTF16PtrFromString(printer)
	if err != nil {
		return 0, NotFoundError(printer)
	}
	var handle uintptr
	r1, _, errno := procOpenPrinterW.Call(
		uintptr(unsafe.Pointer(namePtr)),
		uintptr(unsafe.Pointer(&handle)),
		0,
	)
	if r1 == 0 || handle == 0 {
		return 0, NotFoundError(fmt.Sprintf("%s: OpenPrinterW: %s", printer, errno.Error()))
	}
	return handle, nil
}

// closePrinter releases handle, ignoring the result; failures on close
// carry no job state worth reporting.
func closePrinter(handle uintptr) {
	_, _, _ = procClosePrinter.Call(handle)
}

// deviceCapabilities calls DeviceCapabilitiesW for the printer on port,
// filling output when it is non-nil.
func deviceCapabilities(name, port *uint16, capability uintptr, output unsafe.Pointer) uintptr {
	result, _, _ := procDeviceCapabilities.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(port)), capability, uintptr(output), 0)
	return result
}
