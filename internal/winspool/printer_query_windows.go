//go:build windows

package winspool

import (
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

func platformPagesPerMinute(printer string) (int, bool) {
	handle, err := openPrinter(printer)
	if err != nil {
		return 0, false
	}
	defer closePrinter(handle)
	name, _ := windows.UTF16PtrFromString("printPagesPerMinute")
	for _, keyName := range []string{"DsDriver", "DsSpooler"} {
		key, _ := windows.UTF16PtrFromString(keyName)
		var kind, value, needed uint32
		result, _, _ := procGetPrinterDataExW.Call(handle, uintptr(unsafe.Pointer(key)),
			uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&kind)),
			uintptr(unsafe.Pointer(&value)), unsafe.Sizeof(value), uintptr(unsafe.Pointer(&needed)))
		if result == 0 && kind == windows.REG_DWORD && needed == 4 && value > 0 && value <= 0x7fffffff {
			return int(value), true
		}
	}
	return 0, false
}

func platformIdentifyPrinter(printer string) error {
	const mbIconAsterisk = 0x40
	result, _, err := procMessageBeep.Call(mbIconAsterisk)
	if result == 0 {
		return SpoolError(printer, gdiError("MessageBeep", err).Error())
	}
	return nil
}

// queryDeviceCapability asks DeviceCapabilitiesW for a capability that has
// no output buffer, on the port the printer's queue is bound to. ok is false
// when the queue or its port cannot be read.
func queryDeviceCapability(printer string, capability uintptr) (result int32, ok bool) {
	name, err := windows.UTF16PtrFromString(printer)
	if err != nil {
		return 0, false
	}
	handle, err := openPrinter(printer)
	if err != nil {
		return 0, false
	}
	defer closePrinter(handle)
	info, ok := fetchPrinterInfo2(handle)
	if !ok {
		return 0, false
	}
	port := info.port()
	if port == nil {
		return 0, false
	}
	result = int32(deviceCapabilities(name, port, capability, nil))
	runtime.KeepAlive(info)
	return result, true
}

func platformColorSupported(printer string) (bool, bool) {
	const dcColorDevice = 32
	result, ok := queryDeviceCapability(printer, dcColorDevice)
	if !ok {
		return false, false
	}
	switch result {
	case 0:
		return false, true
	case 1:
		// A color device is not enough: the bridge must also be able to
		// select color in the driver's settings for a client request.
		name, err := windows.UTF16PtrFromString(printer)
		if err != nil {
			return false, false
		}
		_, _, err = printDevMode(printer, name, core.PrintOptions{Color: true})
		return err == nil, true
	default:
		return false, false
	}
}

// Check the driver, not the user-configurable queue name, so renamed queues
// still receive a noninteractive output path.
func microsoftPDFDriver(printer string) (bool, error) {
	handle, err := openPrinter(printer)
	if err != nil {
		return false, err
	}
	defer closePrinter(handle)
	needed := printerInfo2Needed(handle)
	if !printerInfo2SizeValid(needed) {
		return false, SpoolError(printer, "cannot retrieve printer driver")
	}
	info, err := readPrinterInfo2(handle, needed)
	if err != nil {
		return false, SpoolError(printer, gdiError("GetPrinterW", err).Error())
	}
	driver := info.driver()
	if driver == nil {
		return false, SpoolError(printer, "printer driver name is unavailable")
	}
	name := utf16PtrToString(unsafe.Pointer(driver))
	runtime.KeepAlive(info)
	return strings.EqualFold(name, "Microsoft Print To PDF"), nil
}

// platformStatus queries the live queue state via OpenPrinterW plus
// GetPrinterW (level 2). Unknown names and query failures report
// PrinterStatusUnknown instead of erroring.
func platformStatus(printer string) PrinterStatus {
	bits, ok := platformStatusBits(printer)
	if !ok {
		return PrinterStatusUnknown
	}
	return statusFromWinspoolBits(bits)
}

func platformStatusBits(printer string) (uint32, bool) {
	handle, err := openPrinter(printer)
	if err != nil {
		return 0, false
	}
	defer closePrinter(handle)

	// First call harvests the required byte count; it is expected to fail.
	needed := printerInfo2Needed(handle)
	if needed == 0 {
		return 0, false
	}
	info, err := readPrinterInfo2(handle, needed)
	if err != nil {
		return 0, false
	}
	return info.status()
}
