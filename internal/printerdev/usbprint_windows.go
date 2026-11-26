//go:build windows

package printerdev

import (
	"errors"
	"fmt"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The USB printing device interface of usbprint.sys, the class driver
// Windows itself loads for every USB printer: opening it needs no driver
// replacement and no elevated rights.
var guidUSBPrint = windows.GUID{
	Data1: 0x28d78fad,
	Data2: 0x5a12,
	Data3: 0x11d1,
	Data4: [8]byte{0xae, 0x5b, 0x00, 0x00, 0xf8, 0x03, 0xa8, 0xc2},
}

// ioctlUSBPrintGet1284ID asks usbprint.sys for the printer's IEEE 1284
// device ID: CTL_CODE(FILE_DEVICE_UNKNOWN, 13, METHOD_BUFFERED,
// FILE_ANY_ACCESS).
const ioctlUSBPrintGet1284ID = 0x22<<16 | 13<<2

// ioTimeout bounds one read or write of the device; a printer that stops
// answering mid-conversation must not hang the caller.
const ioTimeout = 3 * time.Second

// setupapi.dll and the enumeration calls; the device interface API has no
// x/sys/windows binding.
var (
	setupapi                        = windows.NewLazySystemDLL("setupapi.dll")
	procSetupDiGetClassDevsW        = setupapi.NewProc("SetupDiGetClassDevsW")
	procSetupDiEnumDeviceInterfaces = setupapi.NewProc("SetupDiEnumDeviceInterfaces")
	procSetupDiGetDeviceInterfaceW  = setupapi.NewProc("SetupDiGetDeviceInterfaceDetailW")
	procSetupDiDestroy              = setupapi.NewProc("SetupDiDestroyDeviceInfoList")
)

const (
	digcfPresent         = 0x2
	digcfDeviceInterface = 0x10
	errorNoMoreItems     = windows.ERROR_NO_MORE_ITEMS
)

// spDeviceInterfaceData is SP_DEVICE_INTERFACE_DATA.
type spDeviceInterfaceData struct {
	size     uint32
	guid     windows.GUID
	flags    uint32
	reserved uintptr
}

// usbprintDevice is an open handle of one USB printing device with
// overlapped reads and writes bounded by ioTimeout.
type usbprintDevice struct {
	handle windows.Handle
	path   string
}

// enumUSBPrintPaths lists the device paths of every present USB printer.
func enumUSBPrintPaths() ([]string, error) {
	set, _, err := syscall.SyscallN(procSetupDiGetClassDevsW.Addr(),
		uintptr(unsafe.Pointer(&guidUSBPrint)), 0, 0, digcfPresent|digcfDeviceInterface)
	if set == 0 || windows.Handle(set) == windows.InvalidHandle {
		return nil, fmt.Errorf("SetupDiGetClassDevs: %w", err)
	}
	defer syscall.SyscallN(procSetupDiDestroy.Addr(), set)

	var paths []string
	for index := 0; ; index++ {
		data := spDeviceInterfaceData{size: uint32(unsafe.Sizeof(spDeviceInterfaceData{}))}
		res, _, err := syscall.SyscallN(procSetupDiEnumDeviceInterfaces.Addr(),
			set, 0, uintptr(unsafe.Pointer(&guidUSBPrint)), uintptr(index), uintptr(unsafe.Pointer(&data)))
		if res == 0 {
			if errors.Is(err, errorNoMoreItems) {
				return paths, nil
			}
			return nil, fmt.Errorf("SetupDiEnumDeviceInterfaces: %w", err)
		}
		// The detail call sizes its buffer first, then fills it.
		var needed uint32
		syscall.SyscallN(procSetupDiGetDeviceInterfaceW.Addr(), set, uintptr(unsafe.Pointer(&data)), 0, 0, uintptr(unsafe.Pointer(&needed)), 0)
		if needed == 0 {
			continue
		}
		buf := make([]byte, needed)
		// SP_DEVICE_INTERFACE_DETAIL_DATA_W: cbSize is 8 on 64-bit
		// (sizeof DWORD + sizeof UNICODE ring), then the path string.
		if unsafe.Sizeof(uintptr(0)) == 8 {
			*(*uint32)(unsafe.Pointer(&buf[0])) = 8
		} else {
			*(*uint32)(unsafe.Pointer(&buf[0])) = 4 + 2
		}
		res, _, _ = syscall.SyscallN(procSetupDiGetDeviceInterfaceW.Addr(),
			set, uintptr(unsafe.Pointer(&data)), uintptr(unsafe.Pointer(&buf[0])), uintptr(needed), uintptr(unsafe.Pointer(&needed)), 0)
		if res == 0 {
			continue
		}
		chars := (*[1 << 20]uint16)(unsafe.Pointer(&buf[4]))[:(needed-4)/2]
		if path := strings.TrimRight(string(utf16.Decode(chars)), "\x00"); path != "" {
			paths = append(paths, path)
		}
	}
}

// openUSBPrint opens one device path for overlapped reading and writing.
func openUSBPrint(path string) (*usbprintDevice, error) {
	wide, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(wide,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return &usbprintDevice{handle: handle, path: path}, nil
}

// Close releases the device handle.
func (d *usbprintDevice) Close() error { return windows.CloseHandle(d.handle) }

// deviceID1284 reads the printer's IEEE 1284 device ID: two length bytes
// (big-endian, counting themselves) then the ASCII identification string.
func (d *usbprintDevice) deviceID1284() (string, error) {
	buf := make([]byte, 1024)
	var done uint32
	err := windows.DeviceIoControl(d.handle, ioctlUSBPrintGet1284ID, nil, 0, &buf[0], uint32(len(buf)), &done, nil)
	if err != nil {
		return "", fmt.Errorf("read 1284 id: %w", err)
	}
	if done < 3 {
		return "", errors.New("short 1284 id")
	}
	length := int(buf[0])<<8 | int(buf[1])
	if length > int(done) || length < 2 {
		length = int(done)
	}
	id := strings.TrimRight(string(buf[2:length]), "\x00")
	return id, nil
}

// overlappedIO runs one overlapped read or write and waits for it with a
// timeout, cancelling when the time runs out.
func (d *usbprintDevice) overlappedIO(write bool, p []byte) (int, error) {
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(event)
	var overlapped windows.Overlapped
	overlapped.HEvent = event

	var done uint32
	if write {
		err = windows.WriteFile(d.handle, p, &done, &overlapped)
	} else {
		err = windows.ReadFile(d.handle, p, &done, &overlapped)
	}
	if err != nil && err != windows.ERROR_IO_PENDING {
		return 0, err
	}
	if err == nil {
		return int(done), nil // completed inline
	}
	wait, err := windows.WaitForSingleObject(event, uint32(ioTimeout.Milliseconds()))
	if err != nil {
		return 0, err
	}
	if wait != windows.WAIT_OBJECT_0 {
		// Timed out or abandoned: cancel the operation so the handle is
		// reusable, then report the timeout.
		_ = cancelIoEx(d.handle, &overlapped)
		windows.WaitForSingleObject(event, 1000)
		return 0, errTimeout
	}
	if err := windows.GetOverlappedResult(d.handle, &overlapped, &done, false); err != nil {
		return 0, err
	}
	return int(done), nil
}

var errTimeout = errors.New("device io timed out")

var procCancelIoEx = windows.NewLazySystemDLL("kernel32.dll").NewProc("CancelIoEx")

func cancelIoEx(handle windows.Handle, overlapped *windows.Overlapped) error {
	res, _, err := syscall.SyscallN(procCancelIoEx.Addr(), uintptr(handle), uintptr(unsafe.Pointer(overlapped)))
	if res == 0 {
		return err
	}
	return nil
}

// Write sends bytes to the printer.
func (d *usbprintDevice) Write(p []byte) (int, error) { return d.overlappedIO(true, p) }

// Read receives bytes from the printer.
func (d *usbprintDevice) Read(p []byte) (int, error) { return d.overlappedIO(false, p) }

// openPrinterDevice finds the USB printing device the Windows queue
// printer prints to and opens it, with its IEEE 1284 device ID.
func openPrinterDevice(printer string) (deviceHandle, error) {
	paths, err := enumUSBPrintPaths()
	if err != nil {
		return deviceHandle{}, err
	}
	if len(paths) == 0 {
		return deviceHandle{}, errNoProbeSource
	}
	want := normalize(printer)
	for _, path := range paths {
		dev, err := openUSBPrint(path)
		if err != nil {
			continue
		}
		id, err := dev.deviceID1284()
		if err != nil {
			dev.Close()
			continue
		}
		if deviceNameMatches(want, id) {
			return deviceHandle{dev: dev, deviceID: id}, nil
		}
		dev.Close()
	}
	return deviceHandle{}, errNoProbeSource
}
