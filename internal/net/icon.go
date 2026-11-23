package net

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
)

// printerIcon is a generic transparent printer icon for clients. It does
// not purport to be a manufacturer-provided image of the physical device.
var printerIcon = func() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 128, 128))
	rect := func(x0, y0, x1, y1 int, c color.NRGBA) {
		draw.Draw(img, image.Rect(x0, y0, x1, y1), image.NewUniform(c), image.Point{}, draw.Src)
	}
	dark := color.NRGBA{R: 44, G: 57, B: 64, A: 255}
	light := color.NRGBA{R: 230, G: 235, B: 233, A: 255}
	paper := color.NRGBA{R: 250, G: 251, B: 247, A: 255}
	accent := color.NRGBA{R: 28, G: 143, B: 113, A: 255}
	rect(32, 9, 96, 58, dark)
	rect(36, 13, 92, 58, paper)
	rect(45, 23, 83, 26, light)
	rect(45, 31, 83, 34, light)
	rect(45, 39, 73, 42, light)
	rect(12, 47, 116, 98, dark)
	rect(16, 51, 112, 91, light)
	rect(89, 58, 105, 63, accent)
	rect(25, 75, 103, 110, dark)
	rect(31, 78, 97, 116, paper)
	rect(40, 90, 88, 93, light)
	rect(40, 98, 88, 101, light)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err) // bytes.Buffer writes cannot fail
	}
	return buf.Bytes()
}()
