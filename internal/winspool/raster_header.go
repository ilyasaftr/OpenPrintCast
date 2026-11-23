package winspool

import (
	"encoding/binary"
	"fmt"
)

func readRasterHeader(data []byte, offset int, mime string) (rasterPage, int, error) {
	var page rasterPage
	size := 32
	if mime == "image/pwg-raster" {
		size = 1796
	}
	if offset > len(data)-size {
		return page, offset, fmt.Errorf("truncated raster page header")
	}
	h := data[offset : offset+size]
	var err error
	if mime == "image/urf" {
		err = parseURFPageHeader(h, &page)
	} else {
		err = parsePWGPageHeader(h, &page)
	}
	if err != nil {
		return page, offset, err
	}
	if page.width <= 0 || page.height <= 0 || uint64(page.width)*uint64(page.height) > maxRasterPixels {
		return page, offset, fmt.Errorf("invalid or oversized raster dimensions %dx%d", page.width, page.height)
	}
	return page, offset + size, nil
}

func headerUint32(h []byte, i int) uint32 { return binary.BigEndian.Uint32(h[i : i+4]) }

func parseURFPageHeader(h []byte, page *rasterPage) error {
	page.width, page.height = int(headerUint32(h, 12)), int(headerUint32(h, 16))
	switch {
	case h[0] == 8 && (h[1] == 0 || h[1] == 4):
		page.channels = 1
	case h[0] == 24 && (h[1] == 1 || h[1] == 5):
		page.channels = 3
	default:
		return fmt.Errorf("unsupported URF color space %d / %d bits", h[1], h[0])
	}
	if headerUint32(h, 20) == 0 {
		return fmt.Errorf("invalid URF resolution")
	}
	page.dpiX = int(headerUint32(h, 20))
	page.dpiY = page.dpiX
	return nil
}

func parsePWGPageHeader(h []byte, page *rasterPage) error {
	u32 := func(i int) uint32 { return headerUint32(h, i) }
	page.width, page.height = int(u32(372)), int(u32(376))
	bits, bpp, stride, order, space := u32(384), u32(388), u32(392), u32(396), u32(400)
	switch {
	case bits == 8 && bpp == 8 && (space == 0 || space == 18 || space == 3):
		page.channels = 1
		page.blackInk = space == 3
	case bits == 8 && bpp == 24 && (space == 1 || space == 19):
		page.channels = 3
	default:
		return fmt.Errorf("unsupported PWG color space %d / %d bits", space, bpp)
	}
	if order != 0 || uint64(stride) != uint64(page.width)*uint64(page.channels) {
		return fmt.Errorf("unsupported PWG color order or row stride")
	}
	if u32(276) == 0 || u32(276) != u32(280) {
		return fmt.Errorf("unsupported PWG resolution")
	}
	page.dpiX, page.dpiY = int(u32(276)), int(u32(280))
	return nil
}
