package winspool

import "encoding/binary"

// TIFF tags and types read from the EXIF block of a JPEG file.
const (
	tiffTagXResolution    = 0x011A
	tiffTagYResolution    = 0x011B
	tiffTagResolutionUnit = 0x0128
	tiffTypeShort         = 3
	tiffTypeRational      = 5
	tiffUnitCm            = 3
	tiffEntrySize         = 12
	tiffHeaderSize        = 8
)

// exifDensity returns the resolution stored in the first image directory of
// an EXIF block (tiff is the block after its signature) as dots per inch, or
// zeros when it is missing or unusable.
func exifDensity(tiff []byte) (dpiX, dpiY float64) {
	var order binary.ByteOrder
	switch {
	case len(tiff) < tiffHeaderSize:
		return 0, 0
	case string(tiff[:2]) == "II":
		order = binary.LittleEndian
	case string(tiff[:2]) == "MM":
		order = binary.BigEndian
	default:
		return 0, 0
	}
	dir := int(order.Uint32(tiff[4:8]))
	if dir < tiffHeaderSize || dir+2 > len(tiff) {
		return 0, 0
	}
	count := int(order.Uint16(tiff[dir:]))
	scale := 1.0
	for i := 0; i < count && dir+2+(i+1)*tiffEntrySize <= len(tiff); i++ {
		entry := tiff[dir+2+i*tiffEntrySize : dir+2+(i+1)*tiffEntrySize]
		switch order.Uint16(entry) {
		case tiffTagXResolution:
			dpiX = tiffRational(order, tiff, entry)
		case tiffTagYResolution:
			dpiY = tiffRational(order, tiff, entry)
		case tiffTagResolutionUnit:
			if order.Uint16(entry[2:]) == tiffTypeShort && order.Uint16(entry[8:]) == tiffUnitCm {
				scale = cmPerInch
			}
		}
	}
	return dpiX * scale, dpiY * scale
}

// tiffRational reads the RATIONAL value an IFD entry points to.
func tiffRational(order binary.ByteOrder, tiff, entry []byte) float64 {
	offset := int(order.Uint32(entry[8:]))
	if order.Uint16(entry[2:]) != tiffTypeRational || offset < 0 || offset+8 > len(tiff) {
		return 0
	}
	numerator, denominator := order.Uint32(tiff[offset:]), order.Uint32(tiff[offset+4:])
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}
