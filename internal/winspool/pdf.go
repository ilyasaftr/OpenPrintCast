package winspool

import (
	"context"
	"fmt"
	"image"
	"io"
	"strings"
	"sync"
	"time"

	pdfium "github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/tetratelabs/wazero"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

var pdfPool struct {
	once sync.Once
	pool pdfium.Pool
	err  error
}

type pdfiumLogWriter struct{}

func (pdfiumLogWriter) Write(p []byte) (int, error) {
	if message := strings.TrimSpace(string(p)); message != "" {
		componentLogger(componentPDFium).Info("pdfium output", "message", message)
	}
	return len(p), nil
}

// renderPDF keeps only one rendered page in memory at a time. Images are
// borrowed from the WASM runtime and must not be retained by the callback.
func renderPDF(data []byte, width, height int, reverse bool, options core.PrintOptions, draw func(image.Image) error) error {
	return renderPDFPages(context.Background(), data, width, height, reverse, options, func(img image.Image, _, _ int) error {
		return draw(img)
	})
}

// acquirePDFInstance lazily starts the shared PDFium pool and borrows an
// instance from it; the caller must Close the instance.
func acquirePDFInstance() (pdfium.Pdfium, error) {
	pdfPool.once.Do(func() {
		pdfPool.pool, pdfPool.err = webassembly.Init(webassembly.Config{
			MaxTotal: 2,
			// PDF bytes are supplied in memory; the renderer needs no filesystem.
			FSConfig: wazero.NewFSConfig(),
			// Windows GUI apps may not have usable stdout or stderr handles.
			Stdout: io.Discard,
			Stderr: pdfiumLogWriter{},
		})
	})
	if pdfPool.err != nil {
		return nil, fmt.Errorf("initialize PDF renderer: %w", pdfPool.err)
	}
	instance, err := pdfPool.pool.GetInstance(30 * time.Second)
	if err != nil {
		return nil, fmt.Errorf("acquire PDF renderer: %w", err)
	}
	return instance, nil
}

// renderPDFPages opens data once. As soon as that same open document yields
// its page count, the count is reported through ctx, before any page is drawn.
func renderPDFPages(ctx context.Context, data []byte, width, height int, reverse bool, options core.PrintOptions, draw func(image.Image, int, int) error) error {
	if width <= 0 || height <= 0 || int64(width)*int64(height) > 50_000_000 {
		return fmt.Errorf("invalid PDF render dimensions %dx%d", width, height)
	}
	instance, err := acquirePDFInstance()
	if err != nil {
		return err
	}
	defer instance.Close()
	doc, err := instance.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		return fmt.Errorf("open PDF: %w", err)
	}
	defer instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
	count, err := instance.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		return fmt.Errorf("count PDF pages: %w", err)
	}
	if count.PageCount == 0 {
		return fmt.Errorf("PDF contains no pages")
	}
	core.ReportPageCount(ctx, count.PageCount)
	return drawPDFPages(instance, doc.Document, count.PageCount, width, height, reverse, options, draw)
}

// drawPDFPages renders and draws the pages selected by options, one at a
// time, optionally in reverse order.
func drawPDFPages(instance pdfium.Pdfium, document references.FPDF_DOCUMENT, pageCount, width, height int, reverse bool, options core.PrintOptions, draw func(image.Image, int, int) error) error {
	drawn := 0
	for i := 0; i < pageCount; i++ {
		index := i
		if reverse {
			index = pageCount - 1 - i
		}
		if !options.IncludesPage(index + 1) {
			continue
		}
		page, err := instance.RenderPageInPixels(&requests.RenderPageInPixels{
			Page:  requests.Page{ByIndex: &requests.PageByIndex{Document: document, Index: index}},
			Width: width, Height: height, RenderForm: true, Document: &document,
		})
		if err != nil {
			return fmt.Errorf("render PDF page %d: %w", index+1, err)
		}
		err = draw(page.Result.RenderedImage, index+1, pageCount)
		page.Cleanup()
		if err != nil {
			return fmt.Errorf("print PDF page %d: %w", index+1, err)
		}
		drawn++
	}
	if drawn == 0 {
		return fmt.Errorf("page ranges do not select any PDF pages")
	}
	return nil
}
