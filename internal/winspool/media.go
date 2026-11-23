package winspool

import "math"

// pixelsToHundredthsMM converts a device pixel count at dpi into hundredths
// of a millimetre.
func pixelsToHundredthsMM(pixels, dpi int) int {
	return int(math.Round(float64(pixels) * 2540 / float64(dpi)))
}

// sameSheetSize reports whether two paper sizes agree within one
// millimetre per side, ignoring orientation.
func sameSheetSize(w, h, width, height int) bool {
	if w > h {
		w, h = h, w
	}
	if width > height {
		width, height = height, width
	}
	return math.Abs(float64(w-width)) <= 100 && math.Abs(float64(h-height)) <= 100
}
