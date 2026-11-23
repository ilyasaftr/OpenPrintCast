package winspool

import (
	"context"
	"image"
	"image/color"
	"testing"
)

func TestFillBGRABandCompositesOntoWhiteInBGROrder(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 2, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 200, G: 100, B: 50, A: 255})
	img.SetNRGBA(1, 0, color.NRGBA{A: 0})
	pixels := make([]byte, 2*4)
	if err := fillBGRABand(context.Background(), pixels, img, 2, BandRange{Y0: 0, Rows: 1}, false); err != nil {
		t.Fatal(err)
	}
	want := []byte{50, 100, 200, 0, 255, 255, 255, 0}
	for i := range want {
		if pixels[i] != want[i] {
			t.Fatalf("pixels = %v, want %v", pixels, want)
		}
	}
}

func TestFillBGRABandMonochromeUsesLuma(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, G: 0, B: 0, A: 255})
	pixels := make([]byte, 4)
	if err := fillBGRABand(context.Background(), pixels, img, 1, BandRange{Rows: 1}, true); err != nil {
		t.Fatal(err)
	}
	if pixels[0] != pixels[1] || pixels[1] != pixels[2] || pixels[2] != 76 {
		t.Fatalf("pure red in monochrome = %v, want gray 76", pixels[:3])
	}
}

func TestFillBGRABandStopsWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	if err := fillBGRABand(ctx, make([]byte, 4), img, 1, BandRange{Rows: 1}, false); err == nil {
		t.Fatal("a canceled context must stop the band conversion")
	}
}
