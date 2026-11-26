//go:build windows

package winspool

import (
	"context"
	"fmt"
	"image"
	"runtime"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

var renderSlots = make(chan struct{}, 2)

// acquireRenderSlot waits for one of the concurrent render slots, or for
// ctx to end. The returned func releases the slot.
func acquireRenderSlot(ctx context.Context) (func(), error) {
	select {
	case renderSlots <- struct{}{}:
		return func() { <-renderSlots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// pageGeometry describes a printer DC: the printable area, its resolution,
// and the physical sheet that contains it.
type pageGeometry struct {
	width, height, dpiX, dpiY      int
	pageW, pageH, offsetX, offsetY int
}

// readPageGeometry queries dc; ok is false if the printable area or
// resolution is unusable. An inconsistent physical sheet falls back to the
// printable area.
func readPageGeometry(dc uintptr) (g pageGeometry, ok bool) {
	g.width, g.height = deviceCaps(dc, gdiHorzRes), deviceCaps(dc, gdiVertRes)
	g.dpiX, g.dpiY = deviceCaps(dc, gdiLogPixelsX), deviceCaps(dc, gdiLogPixelsY)
	if g.width <= 0 || g.height <= 0 || g.dpiX <= 0 || g.dpiY <= 0 {
		return g, false
	}
	g.pageW, g.pageH = deviceCaps(dc, gdiPhysicalWidth), deviceCaps(dc, gdiPhysicalHeight)
	g.offsetX, g.offsetY = deviceCaps(dc, gdiPhysicalOffsetX), deviceCaps(dc, gdiPhysicalOffsetY)
	if g.pageW < g.width || g.pageH < g.height || g.offsetX < 0 || g.offsetY < 0 ||
		g.offsetX+g.width > g.pageW || g.offsetY+g.height > g.pageH {
		g.pageW, g.pageH, g.offsetX, g.offsetY = g.width, g.height, 0, 0
	}
	return g, true
}

// gdiPrintJob is one GDI document on a printer DC. started and completed
// let abortIfUnfinished cancel a document that StartDocW opened but EndDoc
// never closed.
type gdiPrintJob struct {
	ctx               context.Context
	printer           string
	mime              string
	dc                uintptr
	settings          RenderSettings
	geometry          pageGeometry
	renderW, renderH  int
	defaultMonochrome bool
	pdfDriver         bool
	prerotated        bool
	info              docInfoW
	started           bool
	completed         bool
}

// prepareGDIPrintJob checks that the printer DC and driver can take the
// job and sizes the page rendering for it.
func prepareGDIPrintJob(ctx context.Context, printer, mime string, settings RenderSettings, dc uintptr, defaultMonochrome bool) (*gdiPrintJob, error) {
	geometry, ok := readPageGeometry(dc)
	if !ok {
		return nil, SpoolError(printer, "printer returned invalid page dimensions or resolution")
	}
	renderW, renderH, ok := pageRenderSize(geometry.pageW, geometry.pageH, geometry.dpiX, geometry.dpiY, settings.MaxDPI)
	if !ok {
		return nil, SpoolError(printer, "printer page has invalid render dimensions")
	}
	pdfDriver, err := microsoftPDFDriver(printer)
	if err != nil {
		return nil, err
	}
	title, _ := windows.UTF16PtrFromString("AirPrint job")
	return &gdiPrintJob{
		ctx:               ctx,
		printer:           printer,
		mime:              mime,
		dc:                dc,
		settings:          settings,
		geometry:          geometry,
		renderW:           renderW,
		renderH:           renderH,
		defaultMonochrome: defaultMonochrome,
		pdfDriver:         pdfDriver,
		prerotated:        mime == core.MIMEURF || mime == core.MIMEPWG,
		info:              docInfoW{size: int32(unsafe.Sizeof(docInfoW{})), name: title},
	}, nil
}

func (j *gdiPrintJob) abortIfUnfinished() {
	if j.started && !j.completed {
		procAbortDoc.Call(j.dc)
	}
}

// assignPDFOutput picks the file the Microsoft PDF driver will create.
func (j *gdiPrintJob) assignPDFOutput() error {
	output, err := allocatePDFJobOutput("")
	if err != nil {
		return err
	}
	j.info.output, err = windows.UTF16PtrFromString(output)
	if err != nil {
		return fmt.Errorf("encode PDF output path: %w", err)
	}
	componentLogger(componentSpool).Debug("microsoft print to pdf output requested", "path", output)
	return nil
}

func (j *gdiPrintJob) startDocument() error {
	if j.pdfDriver {
		if err := j.assignPDFOutput(); err != nil {
			return err
		}
	}
	result, _, errno := procStartDoc.Call(j.dc, uintptr(unsafe.Pointer(&j.info)))
	runtime.KeepAlive(j.info.output)
	if int32(result) <= 0 {
		return gdiError("StartDocW", errno)
	}
	j.started = true
	core.ReportProgress(j.ctx)
	return nil
}

// printPage prints one decoded page, opening the document on first use.
func (j *gdiPrintJob) printPage(img image.Image, page, total int) error {
	if err := j.ctx.Err(); err != nil {
		return err
	}
	// Rendering can fail before there is a page to print. In particular,
	// avoid creating an empty file on Windows virtual PDF printers.
	if !j.started {
		if err := j.startDocument(); err != nil {
			return err
		}
	}
	g := j.geometry
	img = orientForPrint(img, g.width, g.height, g.dpiX, g.dpiY, j.settings.options.Orientation,
		j.settings.AutoRotateToFit, j.prerotated)
	mono := j.settings.options.PageMonochrome(page, total, j.defaultMonochrome)
	if err := drawPage(j.ctx, j.dc, img, g.width, g.height, g.dpiX, g.dpiY, 0, 0, mono); err != nil {
		return err
	}
	core.ReportProgress(j.ctx)
	return nil
}

func (j *gdiPrintJob) endDocument() error {
	result, _, errno := procEndDoc.Call(j.dc)
	if int32(result) <= 0 {
		return gdiError("EndDoc", errno)
	}
	j.completed = true
	core.ReportProgress(j.ctx)
	return nil
}

// spoolRendered renders pages to a printer DC using the driver's default paper
// and orientation. Each DIB contains just one band, avoiding huge GDI records.
func spoolRendered(printer string, data []byte, mime string, settings RenderSettings) error {
	started := time.Now()
	ctx := settings.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	componentLogger(componentSpool).Info("print job rendering started", "printer", printer, "mime", mime, "bytes", len(data))
	release, err := acquireRenderSlot(ctx)
	if err != nil {
		return err
	}
	defer release()
	name, err := windows.UTF16PtrFromString(printer)
	if err != nil {
		return SpoolError(printer, "invalid printer name")
	}
	mode, defaultMonochrome, err := printDevMode(printer, name, settings.options)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	dc, errno := createPrinterDC(name, mode)
	if dc == 0 {
		return SpoolError(printer, gdiError("CreateDCW", errno).Error())
	}
	defer procDeleteDC.Call(dc)
	job, err := prepareGDIPrintJob(ctx, printer, mime, settings, dc, defaultMonochrome)
	if err != nil {
		return err
	}
	defer job.abortIfUnfinished()
	if err := job.run(data); err != nil {
		return err
	}
	// EndDoc has returned: Windows accepted every page into the queue, so
	// the job now lives or dies downstream of the bridge.
	componentLogger(componentSpool).Info("print job handed to the windows print queue",
		"printer", printer, "elapsed", time.Since(started).Round(time.Millisecond))
	return nil
}

// run renders data into the open DC and closes the document.
func (j *gdiPrintJob) run(data []byte) error {
	if err := j.ctx.Err(); err != nil {
		return err
	}
	if err := renderDocument(j.ctx, data, j.mime, j.settings, j.renderW, j.renderH, j.printPage); err != nil {
		return SpoolError(j.printer, err.Error())
	}
	if err := j.ctx.Err(); err != nil {
		return err
	}
	if err := j.endDocument(); err != nil {
		return SpoolError(j.printer, err.Error())
	}
	return nil
}
