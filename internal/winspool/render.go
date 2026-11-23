package winspool

import (
	"context"
	"image"
	"math"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// reversesOrder reports whether pages are rendered last to first. Two-sided
// jobs keep document order: reversing would pair the wrong pages on each
// sheet.
func (s RenderSettings) reversesOrder() bool {
	return s.ReversePages && s.options.Sides == core.OneSided
}

// renderDocument decodes data according to mime and passes each selected
// page to draw. renderW and renderH only size PDF rasterization.
func renderDocument(ctx context.Context, data []byte, mime string, settings RenderSettings, renderW, renderH int, draw func(image.Image, int, int) error) error {
	handler, ok := handlerFor(mime)
	if !ok {
		return unsupportedRaster(mime)
	}
	return handler.render(ctx, data, core.NormalizeMIME(mime), settings, renderW, renderH, draw)
}

// pageRenderSize returns the pixel size at which a pageW x pageH device
// page is rasterized: capped by maxDPI and by the printer's resolution, and
// scaled down below the pixel budget. ok is false for degenerate pages.
func pageRenderSize(pageW, pageH, dpiX, dpiY, maxDPI int) (renderW, renderH int, ok bool) {
	renderDPI := math.Min(float64(maxDPI), math.Min(float64(dpiX), float64(dpiY)))
	renderW = int(math.Ceil(float64(pageW) * renderDPI / float64(dpiX)))
	renderH = int(math.Ceil(float64(pageH) * renderDPI / float64(dpiY)))
	if renderW <= 0 || renderH <= 0 {
		return 0, 0, false
	}
	if pixels := int64(renderW) * int64(renderH); pixels > 50_000_000 {
		scale := math.Sqrt(49_000_000 / float64(pixels))
		renderW = int(float64(renderW) * scale)
		renderH = int(float64(renderH) * scale)
	}
	return renderW, renderH, true
}
