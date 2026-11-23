package winspool

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

func testPDF() []byte {
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Count 2 /Kids [3 0 R 5 0 R] >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 72 144] /Contents 4 0 R >>",
		"<< /Length 24 >>\nstream\n1 0 0 rg 0 0 72 144 re f\nendstream",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 144 72] /Contents 6 0 R >>",
		"<< /Length 24 >>\nstream\n0 0 1 rg 0 0 144 72 re f\nendstream",
	}
	offsets := make([]int, len(objects)+1)
	for i, object := range objects {
		offsets[i+1] = out.Len()
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return out.Bytes()
}

func TestRenderPDFWithoutStandardHandles(t *testing.T) {
	const childEnv = "OPENPRINTCAST_TEST_CLOSED_STDIO"
	if os.Getenv(childEnv) == "1" {
		_ = os.Stdout.Close()
		_ = os.Stderr.Close()
		err := renderPDF(testPDF(), 200, 200, false, core.PrintOptions{}, func(image.Image) error { return nil })
		if err != nil {
			_ = os.WriteFile(os.Getenv("OPENPRINTCAST_TEST_ERROR_PATH"), []byte(err.Error()), 0o600)
			t.Fail()
		}
		return
	}
	errorPath := filepath.Join(t.TempDir(), "renderer-error")
	cmd := exec.Command(os.Args[0], "-test.run=^TestRenderPDFWithoutStandardHandles$")
	cmd.Env = append(os.Environ(), childEnv+"=1", "OPENPRINTCAST_TEST_ERROR_PATH="+errorPath)
	if err := cmd.Run(); err != nil {
		detail, _ := os.ReadFile(errorPath)
		t.Fatalf("render without standard handles: %v: %s", err, detail)
	}
}

func TestRenderPDFReversePages(t *testing.T) {
	var sizes []image.Point
	err := renderPDF(testPDF(), 200, 200, true, core.PrintOptions{}, func(img image.Image) error {
		sizes = append(sizes, img.Bounds().Size())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sizes) != 2 || sizes[0].X <= sizes[0].Y || sizes[1].Y <= sizes[1].X {
		t.Fatalf("unexpected page order: %v", sizes)
	}
}

func TestRenderPDFPageRanges(t *testing.T) {
	options := core.PrintOptions{PageRanges: []core.PageRange{{First: 2, Last: 2}}}
	var sizes []image.Point
	err := renderPDF(testPDF(), 200, 200, true, options, func(img image.Image) error {
		sizes = append(sizes, img.Bounds().Size())
		return nil
	})
	if err != nil || len(sizes) != 1 || sizes[0].X <= sizes[0].Y {
		t.Fatalf("selected sizes %v, error %v", sizes, err)
	}
}

func TestRenderPDFPageNumbersInReverse(t *testing.T) {
	var pages []int
	err := renderPDFPages(context.Background(), testPDF(), 200, 200, true, core.PrintOptions{}, func(_ image.Image, page, total int) error {
		if total != 2 {
			t.Fatalf("total = %d", total)
		}
		pages = append(pages, page)
		return nil
	})
	if err != nil || len(pages) != 2 || pages[0] != 2 || pages[1] != 1 {
		t.Fatalf("pages = %v, err = %v", pages, err)
	}
}

func TestRotateToFit(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 0, color.RGBA{B: 255, A: 255})
	rotated := rotateToFit(img, 100, 200, 100, 100)
	if rotated.Bounds().Size() != (image.Point{X: 1, Y: 2}) {
		t.Fatalf("size = %v", rotated.Bounds().Size())
	}
	r, _, b, _ := rotated.At(0, 0).RGBA()
	if r <= b {
		t.Fatal("rotation lost or rearranged the first pixel")
	}
	if rotateToFit(img, 200, 100, 100, 100).Bounds().Size() != img.Bounds().Size() {
		t.Fatal("should not rotate a page already fitting the paper")
	}
}

func TestReverseOrientation(t *testing.T) {
	img := image.NewRGBA(image.Rect(5, 7, 7, 8))
	img.Set(5, 7, color.RGBA{R: 255, A: 255})
	img.Set(6, 7, color.RGBA{B: 255, A: 255})
	reversed := rotatedHalfPage{Image: img}
	if reversed.Bounds() != image.Rect(0, 0, 2, 1) {
		t.Fatalf("bounds = %v", reversed.Bounds())
	}
	r, _, b, _ := reversed.At(0, 0).RGBA()
	if b <= r {
		t.Fatal("page was not rotated 180 degrees")
	}
}

func TestExplicitOrientationFitsDocument(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 3))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	landscape := orientForPrint(img, 300, 200, 300, 300, 4, false, false)
	if landscape.Bounds().Dx() != 3 || landscape.Bounds().Dy() != 2 {
		t.Fatalf("landscape dimensions = %v", landscape.Bounds())
	}
	reverse := orientForPrint(img, 300, 200, 300, 300, 5, false, false)
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			if reverse.At(x, y) != landscape.At(2-x, 1-y) {
				t.Fatal("reverse landscape did not rotate the fitted page")
			}
		}
	}
	prerotated := orientForPrint(landscape, 300, 200, 300, 300, 5, false, true)
	if prerotated.At(2, 0) != landscape.At(2, 0) {
		t.Fatal("already-oriented raster was rotated a second time")
	}
}

func TestRenderPDFPages(t *testing.T) {
	var dimensions []image.Point
	err := renderPDF(testPDF(), 200, 200, false, core.PrintOptions{}, func(img image.Image) error {
		dimensions = append(dimensions, img.Bounds().Size())
		r, _, b, _ := img.At(img.Bounds().Dx()/2, img.Bounds().Dy()/2).RGBA()
		if len(dimensions) == 1 && r <= b || len(dimensions) == 2 && b <= r {
			t.Errorf("page %d has unexpected center color: r=%d b=%d", len(dimensions), r, b)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(dimensions) != 2 || dimensions[0].Y <= dimensions[0].X || dimensions[1].X <= dimensions[1].Y {
		t.Fatalf("unexpected rendered page sizes: %v", dimensions)
	}
}

func TestRenderPDFRejectsInvalidDocument(t *testing.T) {
	err := renderPDF([]byte("not a PDF"), 100, 100, false, core.PrintOptions{}, func(image.Image) error {
		t.Fatal("invalid document should not produce a page")
		return nil
	})
	if err == nil {
		t.Fatal("expected invalid PDF to fail")
	}
}
