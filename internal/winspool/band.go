package winspool

import (
	"context"
	"image"
)

// SpoolBandRows is the row count per StretchDIBits call, ported from
// band_height = 256 in spool_to_printer: it keeps each GDI record near
// ~1.5 MB instead of ~90 MB on high-DPI inkjets.
const SpoolBandRows uint32 = 256

// BandRange is one horizontal slice of a source bitmap: Rows rows
// starting at source row Y0.
type BandRange struct {
	Y0   uint32
	Rows uint32
}

// bandRanges splits totalRows of source bitmap into BandRanges of at most
// SpoolBandRows rows — the Go form of the Python
// "for src_y0 in range(0, src_h, band_height)" loop.
func bandRanges(totalRows uint32) []BandRange {
	var bands []BandRange
	var y uint32
	for y < totalRows {
		rows := totalRows - y
		if rows > SpoolBandRows {
			rows = SpoolBandRows
		}
		bands = append(bands, BandRange{Y0: y, Rows: rows})
		y += rows
	}
	return bands
}

// BandRanges splits totalRows of source bitmap into bands of at most
// SpoolBandRows rows.
func BandRanges(totalRows uint32) []BandRange { return bandRanges(totalRows) }

// BandDestination maps a source band to destination Y coordinates for a
// dstH-tall page drawn from a srcH-tall bitmap with top offset yOffset.
//
// It ports the Python "dst_y0 = y_offset + round(src_y0 * dst_h / src_h)"
// computation so the band geometry stays unit-testable. The second return
// is false for degenerate empty bands (the Python
// "if dst_y1 <= dst_y0: continue" guard) and for an empty source.
func bandDestination(srcY0, bandRows, srcH, dstH, yOffset uint32) (dstY0, dstY1 uint32, ok bool) {
	if srcH == 0 {
		return 0, 0, false
	}
	scale := func(v uint64) uint64 {
		return (v*uint64(dstH) + uint64(srcH)/2) / uint64(srcH)
	}
	dstY0 = yOffset + uint32(scale(uint64(srcY0)))
	dstY1 = yOffset + uint32(scale(uint64(srcY0)+uint64(bandRows)))
	if dstY1 <= dstY0 {
		return 0, 0, false
	}
	return dstY0, dstY1, true
}

// BandDestination maps a source band to destination Y coordinates; ok is
// false for degenerate empty bands and for an empty source.
func BandDestination(srcY0, bandRows, srcH, dstH, yOffset uint32) (dstY0, dstY1 uint32, ok bool) {
	return bandDestination(srcY0, bandRows, srcH, dstH, yOffset)
}

// alignWidthToDword rounds a pixel width up to a multiple of 4 so 24-bit
// DIB scanlines carry no DWORD-alignment padding (Python issue #9).
func alignWidthToDword(width uint32) uint32 {
	return (width + 3) &^ 3
}

// AlignWidthToDword rounds width up to a multiple of 4.
func AlignWidthToDword(width uint32) uint32 { return alignWidthToDword(width) }

// fillBGRABand converts the rows of img covered by band into 32-bit BGRX
// pixels, one srcW-wide row after another, checking ctx between rows.
func fillBGRABand(ctx context.Context, pixels []byte, img image.Image, srcW int, band BandRange, monochrome bool) error {
	bounds := img.Bounds()
	for row := 0; row < int(band.Rows); row++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		for col := 0; col < srcW; col++ {
			r, g, b, a := img.At(bounds.Min.X+col, bounds.Min.Y+int(band.Y0)+row).RGBA()
			// RGBA returns premultiplied components; composite onto white.
			offset := (row*srcW + col) * 4
			red, green, blue := (r+0xffff-a)>>8, (g+0xffff-a)>>8, (b+0xffff-a)>>8
			if monochrome {
				red = (red*299 + green*587 + blue*114 + 500) / 1000
				green, blue = red, red
			}
			pixels[offset], pixels[offset+1], pixels[offset+2], pixels[offset+3] = byte(blue), byte(green), byte(red), 0
		}
	}
	return nil
}
