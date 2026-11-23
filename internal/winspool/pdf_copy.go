package winspool

import (
	"context"
	"io"
	"strconv"
	"strings"

	pdfium "github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// copyPDFPages writes the pages of the PDF in data selected by options to
// out as a new document. Copying the pages into a new document, instead of
// forwarding the sender's file, leaves behind its document-level scripts,
// actions and attachments. That is hygiene, not a safety guarantee: page
// content, annotations and links are copied as they are.
func copyPDFPages(ctx context.Context, data []byte, options core.PrintOptions, out io.Writer) error {
	instance, err := acquirePDFInstance()
	if err != nil {
		return err
	}
	defer instance.Close()
	source, err := instance.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		componentLogger(componentPDF).Warn("open pdf for saving failed", "err", err)
		return errPDFUnreadable
	}
	defer instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: source.Document})
	count, err := instance.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: source.Document})
	if err != nil || count.PageCount == 0 {
		return errPDFUnreadable
	}
	core.ReportPageCount(ctx, count.PageCount)
	pageRange, ok := pdfPageRange(count.PageCount, options)
	if !ok {
		return errNoPagesSelected
	}
	return importAndSave(ctx, instance, source.Document, pageRange, out)
}

// importAndSave copies the pages named by pageRange (all when nil) from
// source into a new document and saves that document to out.
func importAndSave(ctx context.Context, instance pdfium.Pdfium, source references.FPDF_DOCUMENT, pageRange *string, out io.Writer) error {
	created, err := instance.FPDF_CreateNewDocument(&requests.FPDF_CreateNewDocument{})
	if err != nil {
		return errPDFCopyFailed
	}
	defer instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: created.Document})
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err = instance.FPDF_ImportPages(&requests.FPDF_ImportPages{Source: source, Destination: created.Document, PageRange: pageRange})
	if err != nil {
		componentLogger(componentPDF).Warn("import pdf pages failed", "err", err)
		return errPDFCopyFailed
	}
	_, err = instance.FPDF_SaveAsCopy(&requests.FPDF_SaveAsCopy{
		Document: created.Document, Flags: requests.SaveFlagNoIncremental, FileWriter: contextWriter{ctx, out},
	})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return errPDFCopyFailed
	}
	return nil
}

// pdfPageRange builds the page-range string FPDF_ImportPages takes ("1-3,5")
// from the selected pages of a pageCount-page document. It is nil when every
// page is selected, and ok is false when none is.
func pdfPageRange(pageCount int, options core.PrintOptions) (rangeText *string, ok bool) {
	var parts []string
	for first := 1; first <= pageCount; first++ {
		if !options.IncludesPage(first) {
			continue
		}
		last := first
		for last < pageCount && options.IncludesPage(last+1) {
			last++
		}
		parts = append(parts, pdfRangePart(first, last))
		first = last
	}
	switch {
	case len(parts) == 0:
		return nil, false
	case len(options.PageRanges) == 0:
		return nil, true
	}
	text := strings.Join(parts, ",")
	return &text, true
}

func pdfRangePart(first, last int) string {
	if first == last {
		return strconv.Itoa(first)
	}
	return strconv.Itoa(first) + "-" + strconv.Itoa(last)
}
