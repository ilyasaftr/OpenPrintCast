package winspool

import "encoding/binary"

const (
	pngSignatureSize = 8
	pngChunkHeader   = 8  // length and type
	pngChunkOverhead = 12 // length, type and CRC
	pngPHYsSize      = 9
	pngUnitMeter     = 1
	metersPerInch    = 0.0254
)

// pngDensity returns the pixel density of a PNG file from its pHYs chunk as
// dots per inch, or zeros when the file has none or only an aspect ratio.
func pngDensity(data []byte) (dpiX, dpiY float64) {
	for pos := pngSignatureSize; pos+pngChunkOverhead <= len(data); {
		size := int(binary.BigEndian.Uint32(data[pos:]))
		kind := string(data[pos+4 : pos+pngChunkHeader])
		body := pos + pngChunkHeader
		if kind == "IDAT" || size < 0 || body+size > len(data) {
			return 0, 0
		}
		if kind == "pHYs" && size == pngPHYsSize && data[body+8] == pngUnitMeter {
			x := float64(binary.BigEndian.Uint32(data[body:]))
			y := float64(binary.BigEndian.Uint32(data[body+4:]))
			return x * metersPerInch, y * metersPerInch
		}
		pos += pngChunkOverhead + size
	}
	return 0, 0
}
