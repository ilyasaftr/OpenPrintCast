//go:build windows

package winspool

import (
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// DeviceCapabilitiesW capability selectors for paper enumeration.
const (
	dcPapers    = 2
	dcPaperSize = 3
)

// paperDimension is one POINT entry from DC_PAPERSIZE, in tenths of a
// millimetre.
type paperDimension struct{ X, Y int32 }

func platformDefaultMedia(printer string) (int, int, bool) {
	info, ok := platformDefaultMediaDetails(printer)
	return info.Width, info.Height, ok
}

// physicalPaperSize reads the sheet size and resolution of dc; ok is false
// unless all four values are positive.
func physicalPaperSize(dc uintptr) (width, height, dpiX, dpiY int, ok bool) {
	width, height = deviceCaps(dc, gdiPhysicalWidth), deviceCaps(dc, gdiPhysicalHeight)
	dpiX, dpiY = deviceCaps(dc, gdiLogPixelsX), deviceCaps(dc, gdiLogPixelsY)
	return width, height, dpiX, dpiY, width > 0 && height > 0 && dpiX > 0 && dpiY > 0
}

// setPrintableMargins records the unprintable margins of a w x h pixel sheet
// when the driver reports a printable area that fits inside it.
func setPrintableMargins(dc uintptr, info *core.MediaDetails, w, h, dpiX, dpiY int) {
	offsetX, offsetY := deviceCaps(dc, gdiPhysicalOffsetX), deviceCaps(dc, gdiPhysicalOffsetY)
	printW, printH := deviceCaps(dc, gdiHorzRes), deviceCaps(dc, gdiVertRes)
	if offsetX < 0 || offsetY < 0 || printW <= 0 || printH <= 0 ||
		offsetX+printW > w || offsetY+printH > h {
		return
	}
	info.Left, info.Right = pixelsToHundredthsMM(offsetX, dpiX), pixelsToHundredthsMM(w-offsetX-printW, dpiX)
	info.Top, info.Bottom = pixelsToHundredthsMM(offsetY, dpiY), pixelsToHundredthsMM(h-offsetY-printH, dpiY)
	info.MarginsKnown = true
}

// Some drivers accept a paper ID in DEVMODE but silently keep the default
// paper. Verify the resulting DC before advertising the size to clients.
func paperModeDetails(name *uint16, mode []byte, width, height int) (core.MediaDetails, bool) {
	dc, _ := createPrinterDC(name, mode)
	if dc == 0 {
		return core.MediaDetails{}, false
	}
	defer procDeleteDC.Call(dc)
	w, h, dpiX, dpiY, ok := physicalPaperSize(dc)
	if !ok {
		return core.MediaDetails{}, false
	}
	info := core.MediaDetails{Width: pixelsToHundredthsMM(w, dpiX), Height: pixelsToHundredthsMM(h, dpiY)}
	if w <= h {
		setPrintableMargins(dc, &info, w, h, dpiX, dpiY)
	}
	return info, sameSheetSize(info.Width, info.Height, width, height)
}

func platformDefaultMediaDetails(printer string) (core.MediaDetails, bool) {
	name, err := windows.UTF16PtrFromString(printer)
	if err != nil {
		return core.MediaDetails{}, false
	}
	dc, _ := createDefaultPrinterDC(name)
	if dc == 0 {
		return core.MediaDetails{}, false
	}
	defer procDeleteDC.Call(dc)
	width, height, dpiX, dpiY, ok := physicalPaperSize(dc)
	if !ok {
		return core.MediaDetails{}, false
	}
	info := core.MediaDetails{
		Width: pixelsToHundredthsMM(width, dpiX), Height: pixelsToHundredthsMM(height, dpiY), DPIX: dpiX, DPIY: dpiY,
	}
	if info.Width <= info.Height {
		setPrintableMargins(dc, &info, width, height, dpiX, dpiY)
	}
	if info.Width > info.Height {
		info.Width, info.Height = info.Height, info.Width
	}
	return info, true
}

// driverPapers lists the paper IDs and sizes the driver advertises for the
// printer on port; nil slices mean the driver's answers were unusable.
func driverPapers(name, port *uint16) ([]uint16, []paperDimension) {
	count := deviceCapabilities(name, port, dcPapers, nil)
	if int32(count) <= 0 || count > 4096 {
		return nil, nil
	}
	// DeviceCapabilities has no buffer-size argument. Reserve space for the
	// full WORD ID range in case the driver's list changes between calls.
	ids := make([]uint16, 1<<16)
	dimensions := make([]paperDimension, 1<<16)
	gotIDs := deviceCapabilities(name, port, dcPapers, unsafe.Pointer(&ids[0]))
	gotSizes := deviceCapabilities(name, port, dcPaperSize, unsafe.Pointer(&dimensions[0]))
	runtime.KeepAlive(ids)
	runtime.KeepAlive(dimensions)
	if gotIDs != count || gotSizes != count {
		return nil, nil
	}
	return ids[:count], dimensions[:count]
}

// validatedPaperSize returns the size for paper id only if the driver
// honors it in a real device context.
func validatedPaperSize(printer string, name *uint16, id uint16, dimension paperDimension) (core.MediaSize, bool) {
	if id == 0 || id > 32767 || dimension.X <= 0 || dimension.Y <= 0 {
		return core.MediaSize{}, false
	}
	mode, _, err := printDevMode(printer, name, core.PrintOptions{PaperID: int(id)})
	width, height := int(dimension.X)*10, int(dimension.Y)*10
	if err != nil {
		return core.MediaSize{}, false
	}
	info, valid := paperModeDetails(name, mode, width, height)
	if !valid {
		return core.MediaSize{}, false
	}
	return core.MediaSize{
		PaperID: int(id), Width: width, Height: height,
		Left: info.Left, Right: info.Right, Top: info.Top, Bottom: info.Bottom, MarginsKnown: info.MarginsKnown,
	}, true
}

func platformSupportedMedia(printer string) []core.MediaSize {
	name, err := windows.UTF16PtrFromString(printer)
	if err != nil {
		return nil
	}
	handle, err := openPrinter(printer)
	if err != nil {
		return nil
	}
	defer closePrinter(handle)
	info, ok := fetchPrinterInfo2(handle)
	if !ok {
		return nil
	}
	port := info.port()
	if port == nil {
		return nil
	}
	ids, dimensions := driverPapers(name, port)
	runtime.KeepAlive(info)
	var sizes []core.MediaSize
	for i, id := range ids {
		if size, ok := validatedPaperSize(printer, name, id, dimensions[i]); ok {
			sizes = append(sizes, size)
		}
	}
	return sizes
}
