package winspool

import (
	"image"
	"image/color"
	"math"
)

type rotatedPage struct {
	image.Image
}

type rotatedHalfPage struct{ image.Image }

func (p rotatedHalfPage) Bounds() image.Rectangle {
	b := p.Image.Bounds()
	return image.Rect(0, 0, b.Dx(), b.Dy())
}

func (p rotatedHalfPage) At(x, y int) color.Color {
	b := p.Image.Bounds()
	return p.Image.At(b.Max.X-1-x, b.Max.Y-1-y)
}

func (p rotatedPage) ColorModel() color.Model { return p.Image.ColorModel() }
func (p rotatedPage) Bounds() image.Rectangle {
	b := p.Image.Bounds()
	return image.Rect(0, 0, b.Dy(), b.Dx())
}
func (p rotatedPage) At(x, y int) color.Color {
	b := p.Image.Bounds()
	return p.Image.At(b.Min.X+y, b.Max.Y-1-x)
}

func rotateToFit(img image.Image, width, height, dpiX, dpiY int) image.Image {
	b := img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 || dpiX <= 0 || dpiY <= 0 {
		return img
	}
	paperW := float64(width) / float64(dpiX)
	paperH := float64(height) / float64(dpiY)
	currentScale := math.Min(paperW/float64(b.Dx()), paperH/float64(b.Dy()))
	rotatedScale := math.Min(paperW/float64(b.Dy()), paperH/float64(b.Dx()))
	if rotatedScale > currentScale {
		return rotatedPage{Image: img}
	}
	return img
}

func orientForPrint(img image.Image, width, height, dpiX, dpiY, orientation int, autoRotate, prerotated bool) image.Image {
	if orientation != 0 || autoRotate {
		img = rotateToFit(img, width, height, dpiX, dpiY)
	}
	if (orientation == 5 || orientation == 6) && !prerotated {
		img = rotatedHalfPage{Image: img}
	}
	return img
}
