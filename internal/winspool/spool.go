package winspool

import (
	"math"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// pageDestination places a document on the physical sheet, translated into
// the printable-area coordinates used by a printer device context.
func pageDestination(srcW, srcH, pageW, pageH, dpiX, dpiY, offsetX, offsetY int) (x, y, width, height int) {
	if srcW <= 0 || srcH <= 0 || pageW <= 0 || pageH <= 0 || dpiX <= 0 || dpiY <= 0 {
		return 0, 0, 0, 0
	}
	scale := math.Min(float64(pageW)/float64(srcW*dpiX), float64(pageH)/float64(srcH*dpiY))
	width = int(math.Round(float64(srcW) * scale * float64(dpiX)))
	height = int(math.Round(float64(srcH) * scale * float64(dpiY)))
	return (pageW-width)/2 - offsetX, (pageH-height)/2 - offsetY, width, height
}

// RawChunkSize bounds a single WritePrinter call so arbitrarily large
// buffers never need a second copy.
const RawChunkSize = 64 * 1024

// ClampSpoolDPI constrains a requested raster DPI into the shared
// supported range. The limit of record lives in core; this package only
// applies it.
func ClampSpoolDPI(dpi int) int { return core.ClampRenderDPI(dpi) }

// needsRasterPath reports whether mime needs the banded-GDI raster path
// (true) or can use RAW spooler passthrough (false): every registered
// document format is rendered. Parameters after ';' are ignored, matching is
// case-insensitive and aliases count.
func needsRasterPath(mime string) bool {
	_, ok := handlerFor(mime)
	return ok
}

// canonicalMIME is the registry's canonical type for an accepted mime or
// alias, and mime without its parameters otherwise.
func canonicalMIME(mime string) string {
	if format, ok := core.LookupDocumentFormat(mime); ok {
		return format.MIME
	}
	return core.NormalizeMIME(mime)
}

// NeedsRasterPath reports whether mime needs the banded-GDI raster path.
func NeedsRasterPath(mime string) bool { return needsRasterPath(mime) }

func unsupportedRaster(mime string) error {
	return UnsupportedError("document format '" + mime + "' is not supported")
}
