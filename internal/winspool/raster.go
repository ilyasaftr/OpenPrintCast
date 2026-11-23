package winspool

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

const maxRasterPixels = 50_000_000

type rasterPage struct {
	width, height int
	channels      int
	blackInk      bool
	dpiX, dpiY    int
	dataOffset    int
}

// renderRaster validates the complete stream before printing. It records
// compressed page offsets so reverse order never holds multiple images.
func renderRaster(data []byte, mime string, reverse bool, options core.PrintOptions, draw func(image.Image) error) error {
	return renderRasterPages(data, mime, reverse, options, func(img image.Image, _, _ int) error {
		return draw(img)
	})
}

func renderRasterPages(data []byte, mime string, reverse bool, options core.PrintOptions, draw func(image.Image, int, int) error) error {
	return renderRasterPagesContext(context.Background(), data, mime, reverse, options, draw)
}

func renderRasterPagesContext(ctx context.Context, data []byte, mime string, reverse bool, options core.PrintOptions, draw func(image.Image, int, int) error) error {
	pages, err := openRasterDocument(ctx, data, mime)
	if err != nil {
		return err
	}
	core.ReportPageCount(ctx, len(pages))
	return drawRasterPages(ctx, data, pages, reverse, options, draw)
}

// openRasterDocument validates the whole stream and returns where each of
// its pages starts, without keeping decoded pixels.
func openRasterDocument(ctx context.Context, data []byte, mime string) ([]rasterPage, error) {
	offset, pageCount, err := readRasterStreamHeader(data, mime)
	if err != nil {
		return nil, err
	}
	pages, err := scanRasterPages(ctx, data, mime, offset)
	if err != nil {
		return nil, err
	}
	if mime == "image/urf" && pageCount != 0 && pageCount != ^uint32(0) && pageCount != uint32(len(pages)) {
		return nil, fmt.Errorf("URF page count does not match document")
	}
	return pages, nil
}

// readRasterStreamHeader validates the stream signature and returns the
// offset of the first page header plus the URF page count (zero for PWG).
func readRasterStreamHeader(data []byte, mime string) (offset int, pageCount uint32, err error) {
	switch mime {
	case "image/urf":
		if len(data) < 12 || !bytes.Equal(data[:8], []byte("UNIRAST\x00")) {
			return 0, 0, fmt.Errorf("invalid URF header")
		}
		return 12, binary.BigEndian.Uint32(data[8:12]), nil
	case "image/pwg-raster":
		if len(data) < 4 || string(data[:4]) != "RaS2" {
			return 0, 0, fmt.Errorf("invalid PWG raster header")
		}
		return 4, 0, nil
	default:
		return 0, 0, fmt.Errorf("unsupported raster format %q", mime)
	}
}

// scanRasterPages walks every page header and compressed body from offset,
// recording where each page starts without keeping decoded pixels.
func scanRasterPages(ctx context.Context, data []byte, mime string, offset int) ([]rasterPage, error) {
	var pages []rasterPage
	for offset < len(data) {
		var page rasterPage
		var err error
		page, offset, err = readRasterHeader(data, offset, mime)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", len(pages)+1, err)
		}
		page.dataOffset = offset
		offset, _, err = decodeRasterPage(ctx, data, page, false)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", len(pages)+1, err)
		}
		pages = append(pages, page)
		if len(pages) > 10000 {
			return nil, fmt.Errorf("too many raster pages")
		}
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("raster document contains no pages")
	}
	return pages, nil
}

// drawRasterPages decodes and draws the pages selected by options, one at a
// time, optionally in reverse order.
func drawRasterPages(ctx context.Context, data []byte, pages []rasterPage, reverse bool, options core.PrintOptions, draw func(image.Image, int, int) error) error {
	drawn := 0
	for i := range pages {
		index := i
		if reverse {
			index = len(pages) - 1 - i
		}
		if !options.IncludesPage(index + 1) {
			continue
		}
		_, img, err := decodeRasterPage(ctx, data, pages[index], true)
		if err != nil {
			return fmt.Errorf("decode raster page %d: %w", index+1, err)
		}
		if err := draw(img, index+1, len(pages)); err != nil {
			return fmt.Errorf("print raster page %d: %w", index+1, err)
		}
		drawn++
	}
	if drawn == 0 {
		return fmt.Errorf("page ranges do not select any raster pages")
	}
	return nil
}
