package winspool

import (
	"errors"
	"strings"
	"testing"
)

func TestPageDestinationUsesPhysicalSheet(t *testing.T) {
	x, y, width, height := pageDestination(2480, 3508, 2480, 3508, 300, 300, 24, 36)
	if x != -24 || y != -36 || width != 2480 || height != 3508 {
		t.Fatalf("destination = %d,%d %dx%d", x, y, width, height)
	}
	x, y, width, height = pageDestination(100, 100, 2400, 3600, 600, 600, 20, 30)
	if x != -20 || y != 570 || width != 2400 || height != 2400 {
		t.Fatalf("square destination = %d,%d %dx%d", x, y, width, height)
	}
}

func TestRasterMIMEDispatch(t *testing.T) {
	for _, mime := range []string{
		"application/pdf", "image/urf; charset=binary", "IMAGE/PNG",
		"image/pwg-raster", "image/jpeg",
	} {
		if !needsRasterPath(mime) {
			t.Fatalf("needsRasterPath(%q) = false, want true", mime)
		}
	}
	for _, mime := range []string{
		"application/octet-stream", "application/postscript", "", "text/plain",
	} {
		if needsRasterPath(mime) {
			t.Fatalf("needsRasterPath(%q) = true, want false", mime)
		}
	}
	if !NeedsRasterPath("application/pdf") || NeedsRasterPath("application/octet-stream") {
		t.Fatal("exported NeedsRasterPath disagrees")
	}
}

func TestSpoolDPIUsesCoreClamp(t *testing.T) {
	for in, want := range map[int]int{0: 72, 72: 72, 300: 300, 1200: 1200, 9999: 1200} {
		if got := ClampSpoolDPI(in); got != want {
			t.Fatalf("ClampSpoolDPI(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestUnsupportedRasterReportsUnsupported(t *testing.T) {
	err := unsupportedRaster("image/foobar")
	var target *BackendError
	if !errors.As(err, &target) || target.Kind != KindUnsupported {
		t.Fatalf("unsupportedRaster err = %v", err)
	}
	if !strings.Contains(err.Error(), "image/foobar") {
		t.Fatalf("error should name the unsupported format: %v", err)
	}
}
