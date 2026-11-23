package winspool

import (
	"context"
	"encoding/binary"
	"errors"
	"image"
	"os"
	"path/filepath"
	"testing"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

func TestRasterValidationCanBeCanceled(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "cups-large.pwg"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = renderRasterPagesContext(ctx, data, "image/pwg-raster", false, core.PrintOptions{}, func(image.Image, int, int) error {
		t.Fatal("canceled job must not print")
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("validation error = %v", err)
	}
}

func TestCUPSRasterFixtures(t *testing.T) {
	for _, tc := range []struct{ file, mime string }{
		{"cups-color.urf", "image/urf"},
		{"cups-gray.urf", "image/urf"},
		{"cups-color.pwg", "image/pwg-raster"},
		{"cups-gray.pwg", "image/pwg-raster"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", tc.file))
			if err != nil {
				t.Fatal(err)
			}
			var first []uint32
			err = renderRaster(data, tc.mime, false, core.PrintOptions{}, func(img image.Image) error {
				if img.Bounds().Dx() != 4 || img.Bounds().Dy() != 3 {
					t.Fatalf("size = %v", img.Bounds())
				}
				r, _, b, _ := img.At(0, 0).RGBA()
				first = append(first, r<<16|b)
				return nil
			})
			if err != nil || len(first) != 2 || first[0] == first[1] {
				t.Fatalf("pages = %v, err = %v", first, err)
			}
			var reversed []uint32
			err = renderRaster(data, tc.mime, true, core.PrintOptions{}, func(img image.Image) error {
				r, _, b, _ := img.At(0, 0).RGBA()
				reversed = append(reversed, r<<16|b)
				return nil
			})
			if err != nil || len(reversed) != 2 || reversed[0] != first[1] || reversed[1] != first[0] {
				t.Fatalf("reversed = %v, err = %v", reversed, err)
			}
			options := core.PrintOptions{PageRanges: []core.PageRange{{First: 2, Last: 2}}}
			var selected []uint32
			err = renderRaster(data, tc.mime, true, options, func(img image.Image) error {
				r, _, b, _ := img.At(0, 0).RGBA()
				selected = append(selected, r<<16|b)
				return nil
			})
			if err != nil || len(selected) != 1 || selected[0] != first[1] {
				t.Fatalf("selected = %v, err = %v", selected, err)
			}
			if err := renderRaster(data[:len(data)-1], tc.mime, false, core.PrintOptions{}, func(image.Image) error {
				t.Fatal("truncated document must not print")
				return nil
			}); err == nil {
				t.Fatal("truncated raster accepted")
			}
		})
	}
}

func TestRasterWhiteFillAndInvalidRuns(t *testing.T) {
	header := make([]byte, 44)
	copy(header, "UNIRAST\x00")
	binary.BigEndian.PutUint32(header[8:12], 1)
	header[12] = 8
	binary.BigEndian.PutUint32(header[24:28], 4)
	binary.BigEndian.PutUint32(header[28:32], 3)
	binary.BigEndian.PutUint32(header[32:36], 300)
	data := append(header, 2, 128)
	if err := renderRaster(data, "image/urf", false, core.PrintOptions{}, func(img image.Image) error {
		r, g, b, _ := img.At(3, 2).RGBA()
		if r != 65535 || g != 65535 || b != 65535 {
			t.Fatalf("fill = %d,%d,%d", r, g, b)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range [][]byte{
		append(append([]byte{}, header...), 3, 128),
		append(append([]byte{}, header...), 0, 4, 255),
		append(append([]byte{}, header...), 0, 255, 1),
	} {
		if err := renderRaster(bad, "image/urf", false, core.PrintOptions{}, func(image.Image) error { return nil }); err == nil {
			t.Fatal("malformed raster accepted")
		}
	}
}

func TestCUPSFullSizeRaster(t *testing.T) {
	for _, tc := range []struct{ file, mime string }{
		{"cups-large.urf", "image/urf"},
		{"cups-large.pwg", "image/pwg-raster"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", tc.file))
			if err != nil {
				t.Fatal(err)
			}
			options := core.PrintOptions{PageRanges: []core.PageRange{{First: 2, Last: 2}}}
			pages := 0
			err = renderRaster(data, tc.mime, false, options, func(img image.Image) error {
				pages++
				if img.Bounds().Size() != (image.Point{X: 2480, Y: 3508}) {
					t.Fatalf("dimensions = %v", img.Bounds())
				}
				r, _, b, _ := img.At(100, 100).RGBA()
				if b <= r {
					t.Fatalf("expected blue on page 2, got r=%d b=%d", r, b)
				}
				r, g, b, _ := img.At(2000, 3400).RGBA()
				if r != 65535 || g != 65535 || b != 65535 {
					t.Fatalf("expected white, got r=%d g=%d b=%d", r, g, b)
				}
				return nil
			})
			if err != nil || pages != 1 {
				t.Fatalf("pages=%d error=%v", pages, err)
			}
		})
	}
}
