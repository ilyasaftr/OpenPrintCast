//go:build windows

package winspool

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// This opt-in test prints only to a named PDF printer and a temporary file.
func TestWindowsPDFGDI(t *testing.T) {
	printer := os.Getenv("OPENPRINTCAST_TEST_PDF_PRINTER")
	if printer == "" {
		t.Skip("OPENPRINTCAST_TEST_PDF_PRINTER is not set")
	}
	name, err := windows.UTF16PtrFromString(printer)
	if err != nil {
		t.Fatal(err)
	}
	mode, _, err := printDevMode(printer, name, core.PrintOptions{})
	if err != nil {
		t.Fatal(err)
	}
	dc, _, errno := procCreateDC.Call(0, uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&mode[0])))
	runtime.KeepAlive(mode)
	if dc == 0 {
		t.Fatal(gdiError("CreateDCW", errno))
	}
	defer procDeleteDC.Call(dc)
	caps := func(index uintptr) int {
		value, _, _ := procGetDeviceCaps.Call(dc, index)
		return int(int32(value))
	}
	width, height, dpiX, dpiY := caps(110), caps(111), caps(88), caps(90)
	offsetX, offsetY := caps(112), caps(113)
	t.Logf("physical=%dx%d dpi=%dx%d offset=%d,%d printable=%dx%d",
		width, height, dpiX, dpiY, offsetX, offsetY, caps(8), caps(10))
	if width <= 0 || height <= 0 || dpiX <= 0 || dpiY <= 0 {
		t.Fatal("invalid printer dimensions")
	}
	output := filepath.Join(t.TempDir(), "gdi-output.pdf")
	path, _ := windows.UTF16PtrFromString(output)
	title, _ := windows.UTF16PtrFromString("AirPrint GDI test")
	info := docInfoW{size: int32(unsafe.Sizeof(docInfoW{})), name: title, output: path}
	result, _, errno := procStartDoc.Call(dc, uintptr(unsafe.Pointer(&info)))
	runtime.KeepAlive(path)
	runtime.KeepAlive(title)
	if int32(result) <= 0 {
		t.Fatal(gdiError("StartDocW", errno))
	}
	complete := false
	defer func() {
		if !complete {
			procAbortDoc.Call(dc)
		}
	}()
	renderW, renderH := width*300/dpiX, height*300/dpiY
	err = renderPDFPages(context.Background(), testPDF(), renderW, renderH, false, core.PrintOptions{}, func(img image.Image, page, total int) error {
		t.Logf("rendering page %d/%d: %v", page, total, img.Bounds())
		if err := drawPage(context.Background(), dc, img, width, height, dpiX, dpiY, offsetX, offsetY, false); err != nil {
			return fmt.Errorf("page %d: %w", page, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 248, 351))
	for y := 0; y < 351; y++ {
		for x := 0; x < 248; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	if err := drawPage(context.Background(), dc, img, width, height, dpiX, dpiY, offsetX, offsetY, false); err != nil {
		t.Fatalf("normal page: %v", err)
	}
	if err := drawPage(context.Background(), dc, img, width, height, dpiX, dpiY, offsetX, height*3/4, false); err != nil {
		t.Fatalf("page with clipped first band: %v", err)
	}
	result, _, errno = procEndDoc.Call(dc)
	if int32(result) <= 0 {
		t.Fatal(gdiError("EndDoc", errno))
	}
	complete = true
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(output)
		if err == nil && bytes.HasPrefix(data, []byte("%PDF-")) && bytes.Contains(data, []byte("%%EOF")) {
			t.Logf("created PDF with %d bytes", len(data))
			pages := 0
			err = renderPDF(data, 200, 200, false, core.PrintOptions{}, func(img image.Image) error {
				pages++
				if pages <= 2 {
					r, _, b, _ := img.At(img.Bounds().Dx()/2, img.Bounds().Dy()/2).RGBA()
					if pages == 1 && r <= b || pages == 2 && b <= r {
						return fmt.Errorf("page %d has incorrect center color: r=%d b=%d", pages, r, b)
					}
				}
				return nil
			})
			if err != nil || pages != 4 {
				t.Fatalf("PDF output pages=%d error=%v", pages, err)
			}
			return
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatal(fmt.Errorf("PDF output was not written to %s", output))
}

// Exercise the production path, including driver detection and output naming.
func TestWindowsPDFBridgeOutput(t *testing.T) {
	printer := os.Getenv("OPENPRINTCAST_TEST_PDF_PRINTER")
	if printer == "" {
		t.Skip("OPENPRINTCAST_TEST_PDF_PRINTER is not set")
	}
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", root)
	isPDF, err := microsoftPDFDriver(printer)
	if err != nil || !isPDF {
		t.Fatalf("expected Microsoft PDF driver: %t, %v", isPDF, err)
	}
	if err := NewSystemBackend().SpoolBuffer(printer, testPDF(), "application/pdf"); err != nil {
		t.Fatal(err)
	}
	pattern := filepath.Join(root, "OpenPrintCast", "PDFs", "job-*", "AirPrint.pdf")
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		paths, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		if len(paths) == 1 {
			data, err := os.ReadFile(paths[0])
			if err == nil && bytes.HasPrefix(data, []byte("%PDF-")) && bytes.Contains(data, []byte("%%EOF")) {
				pages := 0
				err = renderPDF(data, 200, 200, false, core.PrintOptions{}, func(image.Image) error {
					pages++
					return nil
				})
				if err != nil || pages != 2 {
					t.Fatalf("PDF output pages=%d error=%v", pages, err)
				}
				return
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatal("PDF output was not created by the production spool path")
}
