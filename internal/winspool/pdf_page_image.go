package winspool

import (
	"context"
	"image"
	"io"
	"math"
)

const (
	// defaultImageDPI is assumed for images that do not state a density.
	defaultImageDPI = 300
	// minImageDPI and maxImageDPI bound the densities taken at their word;
	// anything else is treated as missing.
	minImageDPI = 10
	maxImageDPI = 4800
	pointsPerIn = 72
	// maxPageDimensionPt is the largest page side PDF viewers accept (the
	// limit of PDF 1.4 implementation notes).
	maxPageDimensionPt = 14400
)

// pageSizePoints converts a pixel size at the given densities to points.
// Missing or absurd densities fall back to defaultImageDPI, and a page that
// would exceed maxPageDimensionPt is scaled down to fit.
func pageSizePoints(widthPx, heightPx int, dpiX, dpiY float64) (width, height float64) {
	if !plausibleDPI(dpiX) || !plausibleDPI(dpiY) {
		dpiX, dpiY = defaultImageDPI, defaultImageDPI
	}
	width = float64(widthPx) / dpiX * pointsPerIn
	height = float64(heightPx) / dpiY * pointsPerIn
	if over := math.Max(width, height) / maxPageDimensionPt; over > 1 {
		width, height = width/over, height/over
	}
	return width, height
}

func plausibleDPI(dpi float64) bool { return dpi >= minImageDPI && dpi <= maxImageDPI }

// jpegPageImage embeds the JPEG file as it is.
func jpegPageImage(data []byte, info jpegInfo) pdfImage {
	img := pdfImage{
		width: info.width, height: info.height, filter: pdfDCT,
		body: func(w io.Writer) error { _, err := w.Write(data); return err },
	}
	switch info.components {
	case 1:
		img.colorSpace = pdfDeviceGray
	case 3:
		img.colorSpace = pdfDeviceRGB
	default:
		img.colorSpace = pdfDeviceCMYK
		img.invertedDecode = info.adobe
	}
	return img
}

// decodedPageImage compresses a decoded page as gray or RGB samples.
func decodedPageImage(ctx context.Context, img image.Image, gray bool) pdfImage {
	bounds := img.Bounds()
	channels, space := 3, pdfDeviceRGB
	if gray {
		channels, space = 1, pdfDeviceGray
	}
	return pdfImage{
		width: bounds.Dx(), height: bounds.Dy(), colorSpace: space, filter: pdfFlate,
		body: flateImageBody(ctx, bounds.Dx()*channels, bounds.Dy(), sampleFiller(img, gray)),
	}
}

// isGrayImage reports whether img only carries gray levels by its type.
func isGrayImage(img image.Image) bool {
	switch img.(type) {
	case *image.Gray, *image.Gray16:
		return true
	}
	return false
}

// sampleFiller returns a func that writes row y of img as 8-bit samples:
// one per pixel for gray, three for RGB. Transparency is flattened onto
// white and 16-bit samples are reduced to 8 bits.
func sampleFiller(img image.Image, gray bool) func(y int, row []byte) {
	bounds := img.Bounds()
	if rgba, ok := img.(*image.RGBA); ok {
		return func(y int, row []byte) { fillFromRGBA(rgba, y, gray, row) }
	}
	return func(y int, row []byte) {
		for x := 0; x < bounds.Dx(); x++ {
			r, g, b, a := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			storeSample(row, x, gray, whiteBackdrop(r, a), whiteBackdrop(g, a), whiteBackdrop(b, a))
		}
	}
}

// whiteBackdrop composites an alpha-premultiplied 16-bit sample over white
// and returns 8 bits.
func whiteBackdrop(premultiplied, alpha uint32) byte {
	return byte((premultiplied + (0xffff - alpha)) >> 8)
}

func fillFromRGBA(img *image.RGBA, y int, gray bool, row []byte) {
	pix := img.Pix[y*img.Stride : y*img.Stride+img.Rect.Dx()*4]
	for x := 0; x < len(pix)/4; x++ {
		p := pix[x*4 : x*4+4]
		white := 255 - p[3]
		storeSample(row, x, gray, p[0]+white, p[1]+white, p[2]+white)
	}
}

func storeSample(row []byte, x int, gray bool, r, g, b byte) {
	if gray {
		row[x] = r
		return
	}
	row[x*3], row[x*3+1], row[x*3+2] = r, g, b
}
