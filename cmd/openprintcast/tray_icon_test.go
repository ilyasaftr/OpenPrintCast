package main

import (
	"bytes"
	"image/png"
	"testing"
)

func TestTrayIconIsVisible32PxPNG(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(trayIcon()))
	if err != nil {
		t.Fatalf("tray icon is not a PNG: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != 32 || b.Dy() != 32 {
		t.Fatalf("tray icon is %dx%d, want 32x32", b.Dx(), b.Dy())
	}
	opaque := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a == 0xffff {
				opaque++
			}
		}
	}
	if total := b.Dx() * b.Dy(); opaque < total/2 || opaque == total {
		t.Fatalf("%d of %d pixels are opaque, want a tile with rounded transparent corners", opaque, total)
	}
	if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
		t.Error("corner pixel is not transparent")
	}
}
