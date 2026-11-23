package winspool

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// onlyImagePage is the page count of every JPEG or PNG document.
const onlyImagePage = 1

// renderImage draws data as the single page it always is, reporting that
// page count once the image has decoded and before it is drawn.
func renderImage(ctx context.Context, data []byte, draw func(image.Image) error) error {
	img, err := decodeImage(data)
	if err != nil {
		return err
	}
	core.ReportPageCount(ctx, onlyImagePage)
	return draw(img)
}

// decodeImage decodes a JPEG or PNG document after checking, before any
// pixels are allocated, that its dimensions are within the pixel budget.
func decodeImage(data []byte) (image.Image, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("read image dimensions: %w", err)
	}
	if !withinPixelBudget(config.Width, config.Height) {
		return nil, fmt.Errorf("invalid or oversized image dimensions %dx%d", config.Width, config.Height)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	return img, nil
}

// withinPixelBudget reports whether a width x height image is non-empty and
// no larger than maxRasterPixels.
func withinPixelBudget(width, height int) bool {
	return width > 0 && height > 0 && uint64(width)*uint64(height) <= maxRasterPixels
}
