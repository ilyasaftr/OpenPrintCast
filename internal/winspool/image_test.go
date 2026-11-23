package winspool

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestRenderCommonImages(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 3, 2))
	src.Set(1, 1, color.RGBA{R: 255, A: 255})
	for name, encode := range map[string]func(*bytes.Buffer) error{
		"png":  func(b *bytes.Buffer) error { return png.Encode(b, src) },
		"jpeg": func(b *bytes.Buffer) error { return jpeg.Encode(b, src, nil) },
	} {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := encode(&buf); err != nil {
				t.Fatal(err)
			}
			called := false
			if err := renderImage(context.Background(), buf.Bytes(), func(img image.Image) error {
				called = true
				if img.Bounds().Size() != src.Bounds().Size() {
					t.Fatalf("dimensions = %v", img.Bounds())
				}
				return nil
			}); err != nil || !called {
				t.Fatalf("render error = %v, called = %t", err, called)
			}
		})
	}
	if err := renderImage(context.Background(), []byte("not an image"), func(image.Image) error { return nil }); err == nil {
		t.Fatal("invalid image accepted")
	}
}

func TestJPEGMaximumDimension(t *testing.T) {
	for _, size := range []image.Point{{X: 65535, Y: 1}, {X: 1, Y: 65535}} {
		var data bytes.Buffer
		if err := jpeg.Encode(&data, image.NewGray(image.Rectangle{Max: size}), nil); err != nil {
			t.Fatal(err)
		}
		if err := renderImage(context.Background(), data.Bytes(), func(img image.Image) error {
			if img.Bounds().Size() != size {
				t.Errorf("decoded dimensions = %v, want %v", img.Bounds().Size(), size)
			}
			return nil
		}); err != nil {
			t.Fatalf("decode %v JPEG: %v", size, err)
		}
	}
}
