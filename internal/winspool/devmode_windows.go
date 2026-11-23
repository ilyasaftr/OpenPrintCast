//go:build windows

package winspool

import (
	"runtime"
	"slices"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// DEVMODEW dmFields flags and DocumentPropertiesW modes (from WINGDI.H).
const (
	dmOrientation  = 0x1
	dmPaperSize    = 0x2
	dmPaperLength  = 0x4
	dmPaperWidth   = 0x8
	dmCopies       = 0x100
	dmPrintQuality = 0x400
	dmColor        = 0x800
	dmDuplex       = 0x1000

	dmOutBuffer = 2
	dmInBuffer  = 8
)

// devModeQualities maps IPP print-quality values to DMRES_* settings.
var devModeQualities = map[int]int16{3: -1, 4: -3, 5: -4}

// Prefix of DEVMODEW through dmDuplex; the driver-specific tail is retained
// in the original buffer returned by DocumentPropertiesW.
type printerDevMode struct {
	deviceName    [32]uint16
	specVersion   uint16
	driverVersion uint16
	size          uint16
	driverExtra   uint16
	fields        uint32
	orientation   int16
	paperSize     int16
	paperLength   int16
	paperWidth    int16
	scale         int16
	copies        int16
	defaultSource int16
	printQuality  int16
	color         int16
	duplex        int16
}

// devModeOf views the start of a validated DEVMODE buffer.
func devModeOf(buf []byte) *printerDevMode {
	return (*printerDevMode)(unsafe.Pointer(&buf[0]))
}

func currentDevMode(printer string, handle uintptr, name *uint16) ([]byte, error) {
	size, _, _ := procDocumentProperties.Call(0, handle, uintptr(unsafe.Pointer(name)), 0, 0, 0)
	if int32(size) < int32(unsafe.Sizeof(printerDevMode{})) || size > 16<<20 {
		return nil, SpoolError(printer, "cannot retrieve printer settings size")
	}
	buf := make([]byte, int(size))
	result, _, _ := procDocumentProperties.Call(0, handle, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&buf[0])), 0, dmOutBuffer)
	if int32(result) != 1 {
		return nil, SpoolError(printer, "cannot retrieve printer settings")
	}
	mode := devModeOf(buf)
	if int(mode.size) < int(unsafe.Sizeof(printerDevMode{})) || int(mode.size)+int(mode.driverExtra) > len(buf) {
		return nil, SpoolError(printer, "invalid printer settings")
	}
	return buf, nil
}

// devModeRequest records which DEVMODE fields were forced, so the driver's
// merged reply can be verified field by field.
type devModeRequest struct {
	copies      bool
	duplex      int16
	paperID     int
	color       bool
	quality     int16
	orientation int16
}

// honoredBy reports whether the driver kept every requested setting.
func (r devModeRequest) honoredBy(mode *printerDevMode) bool {
	return (!r.copies || (mode.fields&dmCopies != 0 && mode.copies == 1)) &&
		(r.duplex == 0 || (mode.fields&dmDuplex != 0 && mode.duplex == r.duplex)) &&
		(r.paperID == 0 || (mode.fields&dmPaperSize != 0 && int(mode.paperSize) == r.paperID)) &&
		(!r.color || (mode.fields&dmColor != 0 && mode.color == 2)) &&
		(r.quality == 0 || (mode.fields&dmPrintQuality != 0 && mode.printQuality == r.quality)) &&
		(r.orientation == 0 || (mode.fields&dmOrientation != 0 && mode.orientation == r.orientation))
}

// applyDevModeOptions forces single-copy output, one-sided unless the options
// ask for two-sided, and the requested options into mode, rejecting settings
// the driver cannot express.
func applyDevModeOptions(printer string, mode *printerDevMode, options core.PrintOptions) (devModeRequest, error) {
	request := devModeRequest{
		copies:  mode.fields&dmCopies != 0,
		paperID: options.PaperID,
		color:   options.RequiresColor(),
	}
	if request.copies {
		mode.copies = 1
	}
	duplex, err := selectDevModeDuplex(printer, mode, options.Sides)
	if err != nil {
		return request, err
	}
	request.duplex = duplex
	if options.PaperID != 0 {
		mode.fields = mode.fields&^(dmPaperLength|dmPaperWidth) | dmPaperSize
		mode.paperSize = int16(options.PaperID)
	}
	if request.color {
		if mode.fields&dmColor == 0 {
			return request, SpoolError(printer, "printer does not support color selection")
		}
		mode.color = 2
	}
	quality, err := selectDevModeQuality(printer, mode, options.Quality)
	if err != nil {
		return request, err
	}
	request.quality = quality
	orientation, err := selectDevModeOrientation(printer, mode, options.Orientation)
	if err != nil {
		return request, err
	}
	request.orientation = orientation
	return request, nil
}

// selectDevModeDuplex forces the requested sides into mode. A driver without
// a duplex field only prints one-sided, so a two-sided request is rejected.
func selectDevModeDuplex(printer string, mode *printerDevMode, sides core.Sides) (int16, error) {
	if mode.fields&dmDuplex == 0 {
		if sides != core.OneSided {
			return 0, SpoolError(printer, "printer does not support duplex selection")
		}
		return 0, nil
	}
	mode.duplex = devModeDuplex(sides)
	return mode.duplex, nil
}

// selectDevModeQuality applies an IPP print quality; zero means unspecified.
func selectDevModeQuality(printer string, mode *printerDevMode, requested int) (int16, error) {
	if requested == 0 {
		return 0, nil
	}
	if mode.fields&dmPrintQuality == 0 {
		return 0, SpoolError(printer, "printer does not support quality selection")
	}
	quality := devModeQualities[requested]
	if quality == 0 {
		return 0, SpoolError(printer, "invalid print quality")
	}
	mode.printQuality = quality
	return quality, nil
}

// selectDevModeOrientation applies an IPP orientation; zero means unspecified.
func selectDevModeOrientation(printer string, mode *printerDevMode, requested int) (int16, error) {
	if requested == 0 {
		return 0, nil
	}
	if mode.fields&dmOrientation == 0 {
		return 0, SpoolError(printer, "printer does not support orientation selection")
	}
	orientation := int16(1) // DMORIENT_PORTRAIT
	if requested == 4 || requested == 5 {
		orientation = 2 // DMORIENT_LANDSCAPE
	}
	mode.orientation = orientation
	return orientation, nil
}

func printDevMode(printer string, name *uint16, options core.PrintOptions) ([]byte, bool, error) {
	handle, err := openPrinter(printer)
	if err != nil {
		return nil, false, err
	}
	defer closePrinter(handle)
	buf, err := currentDevMode(printer, handle, name)
	if err != nil {
		return nil, false, err
	}
	mode := devModeOf(buf)
	defaultMonochrome := mode.fields&dmColor != 0 && mode.color == 1
	request, err := applyDevModeOptions(printer, mode, options)
	if err != nil {
		return nil, false, err
	}
	result, _, _ := procDocumentProperties.Call(0, handle, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&buf[0])), dmOutBuffer|dmInBuffer)
	runtime.KeepAlive(buf)
	if int32(result) != 1 || !request.honoredBy(mode) {
		return nil, false, SpoolError(printer, "printer rejected requested print settings")
	}
	return buf, defaultMonochrome, nil
}

// defaultPrintQuality maps the driver's current DMRES_* setting back to an
// IPP print quality.
func defaultPrintQuality(mode *printerDevMode) (int, bool) {
	if mode.fields&dmPrintQuality == 0 {
		return 0, false
	}
	switch mode.printQuality {
	case -1:
		return 3, true
	case -3:
		return 4, true
	case -4:
		return 5, true
	default:
		return 0, false
	}
}

func platformPrintQualities(printer string) ([]int, int, bool) {
	name, err := windows.UTF16PtrFromString(printer)
	if err != nil {
		return nil, 0, false
	}
	handle, err := openPrinter(printer)
	if err != nil {
		return nil, 0, false
	}
	buf, err := currentDevMode(printer, handle, name)
	closePrinter(handle)
	if err != nil {
		return nil, 0, false
	}
	defaultQuality, ok := defaultPrintQuality(devModeOf(buf))
	if !ok {
		return nil, 0, false
	}
	var supported []int
	for _, quality := range []int{3, 4, 5} {
		if _, _, err := printDevMode(printer, name, core.PrintOptions{Quality: quality}); err == nil {
			supported = append(supported, quality)
		}
	}
	if !slices.Contains(supported, defaultQuality) {
		return nil, 0, false
	}
	return supported, defaultQuality, true
}

func platformSupportsOrientation(printer string) bool {
	name, err := windows.UTF16PtrFromString(printer)
	if err != nil {
		return false
	}
	_, _, portraitErr := printDevMode(printer, name, core.PrintOptions{Orientation: 3})
	_, _, landscapeErr := printDevMode(printer, name, core.PrintOptions{Orientation: 4})
	return portraitErr == nil && landscapeErr == nil
}

// dcDuplex is the DeviceCapabilitiesW DC_DUPLEX query (WINGDI.H): 1 when the
// printer supports duplex printing, otherwise 0.
const dcDuplex = 7

// platformSupportsDuplex requires the driver both to report duplex and to
// accept each two-sided DEVMODE, so two-sided output is never advertised on
// a capability the driver would reject at print time.
func platformSupportsDuplex(printer string) bool {
	if reported, ok := queryDeviceCapability(printer, dcDuplex); !ok || reported != 1 {
		return false
	}
	name, err := windows.UTF16PtrFromString(printer)
	if err != nil {
		return false
	}
	for _, sides := range []core.Sides{core.TwoSidedLongEdge, core.TwoSidedShortEdge} {
		if _, _, err := printDevMode(printer, name, core.PrintOptions{Sides: sides}); err != nil {
			return false
		}
	}
	return true
}
