package winspool

import (
	"bytes"
	"encoding/binary"
)

// JPEG markers the PDF embedding looks at.
const (
	jpegMarkerPrefix = 0xFF
	jpegSOI          = 0xD8
	jpegEOI          = 0xD9
	jpegSOS          = 0xDA
	jpegAPP0         = 0xE0
	jpegAPP1         = 0xE1
	jpegAPP14        = 0xEE
	jpegSOFBaseline  = 0xC0
	jpegSOFExtended  = 0xC1
	jpegSOFProgress  = 0xC2
)

const (
	// jpegSamplePrecision is the only sample size DCTDecode can carry.
	jpegSamplePrecision = 8
	jfifUnitsInch       = 1
	jfifUnitsCm         = 2
	cmPerInch           = 2.54
)

// Reasons a JPEG file cannot be embedded.
const (
	errJPEGNotImage    = userError("The JPEG file is damaged")
	errJPEGTruncated   = userError("The JPEG file is incomplete")
	errJPEGEmpty       = userError("The JPEG file has no size")
	errJPEGUnsupported = userError("The JPEG file uses a coding that PDF cannot embed (arithmetic, lossless or hierarchical)")
	errJPEGPrecision   = userError("The JPEG file does not have 8-bit samples")
	errJPEGComponents  = userError("The JPEG file does not have 1, 3 or 4 color components")
)

var (
	jfifSignature       = []byte("JFIF\x00")
	exifSignature       = []byte("Exif\x00\x00")
	adobeSignature      = []byte("Adobe")
	jpegFrameKindsKnown = map[byte]bool{jpegSOFBaseline: true, jpegSOFExtended: true, jpegSOFProgress: true}
)

// jpegInfo is what the PDF embedding needs to know about a JPEG file.
type jpegInfo struct {
	width, height, components int
	// adobe is set when an Adobe marker is present; Adobe applications
	// store CMYK samples inverted.
	adobe bool
	// dpiX and dpiY are zero when the file does not say.
	dpiX, dpiY float64
}

// parseJPEG reads the markers up to the frame header. It checks only what
// the embedding depends on; the compressed data itself passes through.
func parseJPEG(data []byte) (jpegInfo, error) {
	var info jpegInfo
	if len(data) < 4 || data[0] != jpegMarkerPrefix || data[1] != jpegSOI {
		return info, errJPEGNotImage
	}
	var jfifX, jfifY float64
	for pos := 2; ; {
		marker, segment, next, err := nextJPEGSegment(data, pos)
		if err != nil {
			return info, err
		}
		pos = next
		switch {
		case marker == jpegAPP0 && bytes.HasPrefix(segment, jfifSignature):
			jfifX, jfifY = jfifDensity(segment)
		case marker == jpegAPP1 && bytes.HasPrefix(segment, exifSignature):
			info.dpiX, info.dpiY = exifDensity(segment[len(exifSignature):])
		case marker == jpegAPP14 && bytes.HasPrefix(segment, adobeSignature):
			info.adobe = true
		case isJPEGFrame(marker):
			if err := readJPEGFrame(marker, segment, &info); err != nil {
				return info, err
			}
			if jfifX > 0 && jfifY > 0 {
				info.dpiX, info.dpiY = jfifX, jfifY
			}
			return info, checkJPEGEnd(data)
		}
	}
}

// nextJPEGSegment returns the marker at pos and its payload.
func nextJPEGSegment(data []byte, pos int) (marker byte, segment []byte, next int, err error) {
	for pos < len(data) && data[pos] != jpegMarkerPrefix {
		pos++
	}
	for pos < len(data) && data[pos] == jpegMarkerPrefix {
		pos++
	}
	if pos+3 > len(data) {
		return 0, nil, pos, errJPEGTruncated
	}
	marker = data[pos]
	if marker == jpegEOI || marker == jpegSOS {
		return 0, nil, pos, errJPEGNotImage
	}
	length := int(binary.BigEndian.Uint16(data[pos+1 : pos+3]))
	if length < 2 || pos+1+length > len(data) {
		return 0, nil, pos, errJPEGTruncated
	}
	return marker, data[pos+3 : pos+1+length], pos + 1 + length, nil
}

// isJPEGFrame reports whether marker starts a frame header (SOF0-SOF15
// except the DHT, JPG and DAC markers that sit between them).
func isJPEGFrame(marker byte) bool {
	return marker >= 0xC0 && marker <= 0xCF && marker != 0xC4 && marker != 0xC8 && marker != 0xCC
}

func readJPEGFrame(marker byte, segment []byte, info *jpegInfo) error {
	if !jpegFrameKindsKnown[marker] {
		return errJPEGUnsupported
	}
	if len(segment) < 6 {
		return errJPEGTruncated
	}
	if segment[0] != jpegSamplePrecision {
		return errJPEGPrecision
	}
	info.height = int(binary.BigEndian.Uint16(segment[1:3]))
	info.width = int(binary.BigEndian.Uint16(segment[3:5]))
	info.components = int(segment[5])
	if info.width == 0 || info.height == 0 {
		return errJPEGEmpty
	}
	if info.components != 1 && info.components != 3 && info.components != 4 {
		return errJPEGComponents
	}
	return nil
}

// checkJPEGEnd requires the end-of-image marker, apart from padding, which
// catches files that were cut short.
func checkJPEGEnd(data []byte) error {
	tail := bytes.TrimRight(data, "\x00")
	if len(tail) < 2 || tail[len(tail)-2] != jpegMarkerPrefix || tail[len(tail)-1] != jpegEOI {
		return errJPEGTruncated
	}
	return nil
}

// jfifDensity returns the JFIF density as dots per inch, or zeros when the
// header gives only an aspect ratio.
func jfifDensity(segment []byte) (dpiX, dpiY float64) {
	if len(segment) < 12 {
		return 0, 0
	}
	x := float64(binary.BigEndian.Uint16(segment[8:10]))
	y := float64(binary.BigEndian.Uint16(segment[10:12]))
	switch segment[7] {
	case jfifUnitsInch:
		return x, y
	case jfifUnitsCm:
		return x * cmPerInch, y * cmPerInch
	}
	return 0, 0
}
