package winspool

import (
	"context"
	"fmt"
	"image"
	"io"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// renderFunc decodes a document and passes each selected page to draw. mime
// is the canonical type; renderW and renderH only size PDF rasterization.
type renderFunc func(ctx context.Context, data []byte, mime string, settings RenderSettings, renderW, renderH int, draw func(image.Image, int, int) error) error

// saveFunc writes a document's selected pages as one PDF on out.
type saveFunc func(ctx context.Context, data []byte, mime string, options core.PrintOptions, out io.Writer) error

// documentHandler is how one registered document format is printed on a
// Windows printer (render) and saved to a folder (save).
type documentHandler struct {
	render renderFunc
	save   saveFunc
}

// documentHandlers has one entry per format of the core registry
// (core.DocumentFormats); TestEveryRegisteredFormatHasHandlers enforces it.
// It is the only place that maps a format to code, so supporting a format
// means adding an entry here and in the registry.
var documentHandlers = map[string]documentHandler{
	core.MIMEPDF:  {renderPDFDocument, savePDF},
	core.MIMEURF:  {renderRasterDocument, saveRaster},
	core.MIMEPWG:  {renderRasterDocument, saveRaster},
	core.MIMEJPEG: {renderImageDocument, saveJPEG},
	core.MIMEPNG:  {renderImageDocument, savePNG},
}

// handlerFor returns the handler of a registered format by its canonical
// type, an alias or a type with parameters.
func handlerFor(mime string) (documentHandler, bool) {
	format, ok := core.LookupDocumentFormat(mime)
	if !ok {
		return documentHandler{}, false
	}
	handler, ok := documentHandlers[format.MIME]
	return handler, ok
}

func renderPDFDocument(ctx context.Context, data []byte, _ string, settings RenderSettings, renderW, renderH int, draw func(image.Image, int, int) error) error {
	return renderPDFPages(ctx, data, renderW, renderH, settings.reversesOrder(), settings.options, draw)
}

func renderRasterDocument(ctx context.Context, data []byte, mime string, settings RenderSettings, _, _ int, draw func(image.Image, int, int) error) error {
	return renderRasterPagesContext(ctx, data, mime, settings.reversesOrder(), settings.options, draw)
}

func renderImageDocument(ctx context.Context, data []byte, _ string, settings RenderSettings, _, _ int, draw func(image.Image, int, int) error) error {
	if !settings.options.IncludesPage(onlyImagePage) {
		return fmt.Errorf("page ranges do not select the image")
	}
	return renderImage(ctx, data, func(img image.Image) error { return draw(img, onlyImagePage, onlyImagePage) })
}

func savePDF(ctx context.Context, data []byte, _ string, options core.PrintOptions, out io.Writer) error {
	return copyPDFPages(ctx, data, options, out)
}

func saveJPEG(ctx context.Context, data []byte, _ string, options core.PrintOptions, out io.Writer) error {
	return embedJPEG(ctx, data, options, out)
}
