//go:build windows

package winspool

import (
	"math"
	"os"
	"runtime"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// This test only creates printer DCs; it does not submit a print job.
func TestWindowsMediaSettings(t *testing.T) {
	printer := os.Getenv("OPENPRINTCAST_TEST_PDF_PRINTER")
	if printer == "" {
		t.Skip("OPENPRINTCAST_TEST_PDF_PRINTER is not set")
	}
	name, err := windows.UTF16PtrFromString(printer)
	if err != nil {
		t.Fatal(err)
	}
	sizes := platformSupportedMedia(printer)
	if len(sizes) == 0 {
		t.Fatal("driver did not report any validated paper sizes")
	}
	for _, size := range sizes {
		mode, _, err := printDevMode(printer, name, core.PrintOptions{PaperID: size.PaperID})
		if err != nil {
			t.Fatalf("paper %d: %v", size.PaperID, err)
		}
		dc, _, errno := procCreateDC.Call(0, uintptr(unsafe.Pointer(name)), 0, uintptr(unsafe.Pointer(&mode[0])))
		runtime.KeepAlive(mode)
		if dc == 0 {
			t.Fatalf("paper %d: %v", size.PaperID, gdiError("CreateDCW", errno))
		}
		caps := func(index uintptr) int {
			value, _, _ := procGetDeviceCaps.Call(dc, index)
			return int(int32(value))
		}
		width, height, dx, dy := caps(110), caps(111), caps(88), caps(90)
		procDeleteDC.Call(dc)
		if dx <= 0 || dy <= 0 {
			t.Fatalf("paper %d: invalid resolution", size.PaperID)
		}
		gotW := int(math.Round(float64(width) * 2540 / float64(dx)))
		gotH := int(math.Round(float64(height) * 2540 / float64(dy)))
		wantW, wantH := size.Width, size.Height
		if gotW > gotH {
			gotW, gotH = gotH, gotW
		}
		if wantW > wantH {
			wantW, wantH = wantH, wantW
		}
		if math.Abs(float64(gotW-wantW)) > 100 || math.Abs(float64(gotH-wantH)) > 100 {
			t.Fatalf("paper %d: DC %dx%d, driver reports %dx%d", size.PaperID, gotW, gotH, wantW, wantH)
		}
	}
}
