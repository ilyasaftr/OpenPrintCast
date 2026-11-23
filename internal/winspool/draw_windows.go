//go:build windows

package winspool

import (
	"context"
	"fmt"
	"image"
	"unsafe"
)

// bandDIB is a DIB section holding one band of pixels in native memory.
type bandDIB struct {
	handle uintptr
	bits   unsafe.Pointer
	pixels []byte
}

// newBandDIB creates a top-down 32-bit DIB section that can hold the
// largest band of a srcW x srcH image. The caller must release it.
func newBandDIB(dc uintptr, srcW, srcH int) (bandDIB, error) {
	// GDI can reject Go heap buffers that span separate virtual memory
	// reservations. A DIB section keeps each band in native memory.
	bandBytes := srcW * min(srcH, int(SpoolBandRows)) * 4
	bandInfo := bitmapInfoHeader{
		size: uint32(unsafe.Sizeof(bitmapInfoHeader{})), width: int32(srcW),
		height: -int32(min(srcH, int(SpoolBandRows))), planes: 1, bitCount: 32,
	}
	var bits unsafe.Pointer
	bitmap, _, errno := procCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&bandInfo)), 0,
		uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bitmap == 0 {
		return bandDIB{}, gdiError("CreateDIBSection", errno)
	}
	if bits == nil {
		procDeleteObject.Call(bitmap)
		return bandDIB{}, fmt.Errorf("CreateDIBSection returned no pixel buffer")
	}
	return bandDIB{handle: bitmap, bits: bits, pixels: unsafe.Slice((*byte)(bits), bandBytes)}, nil
}

func (d bandDIB) release() {
	procDeleteObject.Call(d.handle)
}

// stretchBand copies the first band.Rows rows of bits onto dc at (x, destY)
// scaled to destW x destH, returning the StretchDIBits result.
func stretchBand(dc uintptr, bits unsafe.Pointer, srcW int, band BandRange, x, destY, destW, destH int) int32 {
	bmi := bitmapInfoHeader{
		size: uint32(unsafe.Sizeof(bitmapInfoHeader{})), width: int32(srcW),
		height: -int32(band.Rows), planes: 1, bitCount: 32,
	}
	result, _, _ := procStretchDIBits.Call(dc, uintptr(x), uintptr(destY), uintptr(destW),
		uintptr(destH), 0, 0, uintptr(srcW), uintptr(band.Rows),
		uintptr(bits), uintptr(unsafe.Pointer(&bmi)), 0, 0x00cc0020)
	return int32(result)
}

func drawPage(ctx context.Context, dc uintptr, img image.Image, pageW, pageH, dpiX, dpiY, offsetX, offsetY int, monochrome bool) error {
	bounds := img.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	if srcW == 0 || srcH == 0 {
		return fmt.Errorf("empty page image")
	}
	x, y, dstW, dstH := pageDestination(srcW, srcH, pageW, pageH, dpiX, dpiY, offsetX, offsetY)
	if dstW <= 0 || dstH <= 0 {
		return fmt.Errorf("page is too small to print")
	}
	dib, err := newBandDIB(dc, srcW, srcH)
	if err != nil {
		return err
	}
	defer dib.release()
	result, _, errno := procStartPage.Call(dc)
	if int32(result) <= 0 {
		return gdiError("StartPage", errno)
	}
	for _, band := range BandRanges(uint32(srcH)) {
		if err := ctx.Err(); err != nil {
			return err
		}
		y0, y1, ok := BandDestination(band.Y0, band.Rows, uint32(srcH), uint32(dstH), 0)
		if !ok {
			continue
		}
		if err := fillBGRABand(ctx, dib.pixels, img, srcW, band, monochrome); err != nil {
			return err
		}
		if result := stretchBand(dc, dib.bits, srcW, band, x, int(y0)+y, dstW, int(y1-y0)); result <= 0 {
			return fmt.Errorf("StretchDIBits returned %d (band=%d+%d destination=%d,%d %dx%d page=%dx%d offset=%d,%d)",
				result, band.Y0, band.Rows, x, int(y0)+y, dstW, y1-y0, pageW, pageH, offsetX, offsetY)
		}
	}
	result, _, errno = procEndPage.Call(dc)
	if int32(result) <= 0 {
		return gdiError("EndPage", errno)
	}
	return nil
}
