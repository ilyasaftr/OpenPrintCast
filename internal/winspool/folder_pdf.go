package winspool

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// userError is a reason shown to the user as it is, in the job log, so it is
// a full sentence in sentence case and not a Go-style error string.
type userError string

func (e userError) Error() string { return string(e) }

// Reasons a Print to Folder job fails.
const (
	errPDFUnreadable   = userError("The PDF could not be read (it may be password-protected or damaged)")
	errPDFCopyFailed   = userError("The PDF pages could not be saved")
	errNoPagesSelected = userError("The page ranges do not select any pages")
)

// folderJob is one document to save as a PDF file. Only its page ranges are
// applied; color, copies, orientation and the other options are ignored.
type folderJob struct {
	data    []byte
	mime    string
	options core.PrintOptions
	folder  string
	name    string
}

// SpoolFolderJobContext saves a document as a PDF file in folder. It needs no
// Windows printer, so it works on every platform. The file is named after
// jobName and appears in folder only once complete.
func (b SystemBackend) SpoolFolderJobContext(ctx context.Context, data []byte, mime string, options core.PrintOptions, folder, jobName string) error {
	job := folderJob{data: data, mime: canonicalMIME(mime), options: options, folder: folder, name: jobName}
	if err := job.validate(); err != nil {
		return err
	}
	return job.save(ctx)
}

func (j folderJob) validate() error {
	switch {
	case !filepath.IsAbs(j.folder):
		return errors.New("PDF output folder must be an absolute path")
	case !needsRasterPath(j.mime):
		return UnsupportedError("document format cannot be saved as PDF")
	case len(j.data) == 0:
		return errors.New("empty document buffer")
	case !j.options.Valid():
		return errors.New("invalid print options")
	}
	return nil
}

// save writes the PDF into a staging directory inside the folder and
// publishes it under its final name. A job that fails or is cancelled
// leaves nothing in the folder.
func (j folderJob) save(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	staged := &stagedPDF{folder: j.folder}
	defer staged.discard()
	if err := j.write(ctx, staged); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	saved, err := staged.publish(j.name)
	if err != nil {
		return err
	}
	componentLogger(componentSpool).Info("print to folder output saved", "path", saved)
	return nil
}

// write converts the document into a PDF on out, reporting the page count
// before the first byte is written.
func (j folderJob) write(ctx context.Context, out io.Writer) error {
	handler, ok := handlerFor(j.mime)
	if !ok {
		return unsupportedRaster(j.mime)
	}
	return handler.save(ctx, j.data, j.mime, j.options, out)
}

// writeImagePage writes a PDF with the one page img.
func writeImagePage(out io.Writer, img pdfImage, widthPt, heightPt float64) error {
	pdf, err := newPDFWriter(out)
	if err != nil {
		return err
	}
	if err := pdf.addPage(img, widthPt, heightPt); err != nil {
		return err
	}
	return pdf.finish()
}

// reportImagePage reports the page count of a one-image document, or fails
// when the page ranges leave the image out.
func reportImagePage(ctx context.Context, options core.PrintOptions) error {
	if !options.IncludesPage(onlyImagePage) {
		return errNoPagesSelected
	}
	core.ReportPageCount(ctx, onlyImagePage)
	return nil
}

// embedJPEG stores the JPEG file as it is in a one-page PDF.
func embedJPEG(ctx context.Context, data []byte, options core.PrintOptions, out io.Writer) error {
	info, err := parseJPEG(data)
	if err != nil {
		return err
	}
	if !withinPixelBudget(info.width, info.height) {
		return userError(fmt.Sprintf("The JPEG file is too large (%dx%d pixels)", info.width, info.height))
	}
	if err := reportImagePage(ctx, options); err != nil {
		return err
	}
	width, height := pageSizePoints(info.width, info.height, info.dpiX, info.dpiY)
	return writeImagePage(out, jpegPageImage(data, info), width, height)
}

// savePNG decodes the PNG file and stores it as a one-page PDF.
func savePNG(ctx context.Context, data []byte, _ string, options core.PrintOptions, out io.Writer) error {
	img, err := decodeImage(data)
	if err != nil {
		return err
	}
	if err := reportImagePage(ctx, options); err != nil {
		return err
	}
	bounds := img.Bounds()
	dpiX, dpiY := pngDensity(data)
	width, height := pageSizePoints(bounds.Dx(), bounds.Dy(), dpiX, dpiY)
	return writeImagePage(out, decodedPageImage(ctx, img, isGrayImage(img)), width, height)
}

// saveRaster stores the selected pages of a URF or PWG raster document, one
// compressed page at a time.
func saveRaster(ctx context.Context, data []byte, mime string, options core.PrintOptions, out io.Writer) error {
	pages, err := openRasterDocument(ctx, data, mime)
	if err != nil {
		return err
	}
	core.ReportPageCount(ctx, len(pages))
	selected := selectedRasterPages(pages, options)
	if len(selected) == 0 {
		return errNoPagesSelected
	}
	pdf, err := newPDFWriter(out)
	if err != nil {
		return err
	}
	for _, page := range selected {
		if err := addRasterPage(ctx, pdf, data, page); err != nil {
			return err
		}
	}
	return pdf.finish()
}

func selectedRasterPages(pages []rasterPage, options core.PrintOptions) []rasterPage {
	var selected []rasterPage
	for i, page := range pages {
		if options.IncludesPage(i + 1) {
			selected = append(selected, page)
		}
	}
	return selected
}

func addRasterPage(ctx context.Context, pdf *pdfWriter, data []byte, page rasterPage) error {
	_, img, err := decodeRasterPage(ctx, data, page, true)
	if err != nil {
		return fmt.Errorf("decode raster page: %w", err)
	}
	width, height := pageSizePoints(page.width, page.height, float64(page.dpiX), float64(page.dpiY))
	return pdf.addPage(decodedPageImage(ctx, img, page.channels == 1), width, height)
}
