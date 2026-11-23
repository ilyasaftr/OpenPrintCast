package winspool

import (
	"context"
	"fmt"
	"image"
)

// CUPS raster compression uses a row-repeat byte followed by runs of whole
// pixels. Unlike TIFF PackBits, 0x80 fills the rest of the row with white.
func decodeRasterPage(ctx context.Context, data []byte, page rasterPage, output bool) (int, image.Image, error) {
	offset := page.dataOffset
	var row []byte
	var img *image.RGBA
	if output {
		row = make([]byte, page.width*page.channels)
		img = image.NewRGBA(image.Rect(0, 0, page.width, page.height))
	}
	for y := 0; y < page.height; {
		if err := ctx.Err(); err != nil {
			return offset, nil, err
		}
		if offset >= len(data) {
			return offset, nil, fmt.Errorf("truncated row repeat")
		}
		repeat := int(data[offset]) + 1
		offset++
		if repeat > page.height-y {
			return offset, nil, fmt.Errorf("row repeat exceeds page")
		}
		var err error
		offset, err = decodeRasterRow(data, offset, page, row)
		if err != nil {
			return offset, nil, err
		}
		if output {
			storeRasterRow(img, y, repeat, row, page)
		}
		y += repeat
	}
	if !output {
		return offset, nil, nil
	}
	return offset, img, nil
}

// decodeRasterRow consumes one compressed row starting at offset. When row
// is non-nil the decoded samples are written into it; a nil row only
// validates the stream.
func decodeRasterRow(data []byte, offset int, page rasterPage, row []byte) (int, error) {
	rowBytes := page.width * page.channels
	for pos := 0; pos < rowBytes; {
		if offset >= len(data) {
			return offset, fmt.Errorf("truncated row")
		}
		control := data[offset]
		offset++
		if control == 128 {
			if row != nil {
				fillRasterRow(row[pos:], page.blackInk)
			}
			return offset, nil
		}
		pixels := int(control) + 1
		literal := control > 128
		if literal {
			pixels = 257 - int(control)
		}
		n := pixels * page.channels
		if n > rowBytes-pos {
			return offset, fmt.Errorf("raster run exceeds row")
		}
		inputBytes := page.channels
		if literal {
			inputBytes = n
		}
		if inputBytes > len(data)-offset {
			return offset, fmt.Errorf("truncated raster run")
		}
		if row != nil {
			copyRasterRun(row[pos:pos+n], data[offset:offset+inputBytes], page.channels, literal)
		}
		offset += inputBytes
		pos += n
	}
	return offset, nil
}

// fillRasterRow blanks the unwritten tail of a row.
func fillRasterRow(tail []byte, blackInk bool) {
	fill := byte(255)
	if blackInk {
		fill = 0
	}
	for i := range tail {
		tail[i] = fill
	}
}

// copyRasterRun expands one run into dst: literal runs are copied
// verbatim, repeat runs replicate a single pixel of channels samples.
func copyRasterRun(dst, src []byte, channels int, literal bool) {
	if literal {
		copy(dst, src)
		return
	}
	for i := 0; i < len(dst); i += channels {
		copy(dst[i:i+channels], src[:channels])
	}
}

// storeRasterRow writes a decoded row into img as opaque RGBA at y and
// replicates it for the following repeat-1 rows.
func storeRasterRow(img *image.RGBA, y, repeat int, row []byte, page rasterPage) {
	dest := img.Pix[y*img.Stride : (y+1)*img.Stride]
	for x := 0; x < page.width; x++ {
		if page.channels == 1 {
			v := row[x]
			if page.blackInk {
				v = 255 - v
			}
			dest[x*4], dest[x*4+1], dest[x*4+2] = v, v, v
		} else {
			copy(dest[x*4:x*4+3], row[x*3:x*3+3])
		}
		dest[x*4+3] = 255
	}
	for n := 1; n < repeat; n++ {
		copy(img.Pix[(y+n)*img.Stride:(y+n+1)*img.Stride], dest)
	}
}
