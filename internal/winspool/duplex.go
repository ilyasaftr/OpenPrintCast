package winspool

import "github.com/ilyasaftr/OpenPrintCast/internal/core"

// DEVMODE dmDuplex values (WINGDI.H; [MS-RPRN] 2.2.2.1 DEVMODE).
const (
	// dmdupSimplex is "Normal (non-duplex) printing".
	dmdupSimplex int16 = 1
	// dmdupVertical is "Long-edge binding; that is, the long edge of the
	// page is vertical".
	dmdupVertical int16 = 2
	// dmdupHorizontal is "Short-edge binding; that is, the long edge of the
	// page is horizontal".
	dmdupHorizontal int16 = 3
)

// devModeDuplex maps an IPP sides value to the DEVMODE dmDuplex value.
//
// The mapping does not depend on dmOrientation: Windows drivers apply
// dmDuplex to the sheet, so DMDUP_VERTICAL always binds the sheet's long
// edge and DMDUP_HORIZONTAL its short edge, in portrait and in landscape
// alike. Evidence: Microsoft's Word support note (a duplex binding setting
// "does not change when the page orientation changes", so mixed-orientation
// jobs print one orientation upside down), Chromium's DEVMODE writer
// (LONG_EDGE -> DMDUP_VERTICAL, SHORT_EDGE -> DMDUP_HORIZONTAL whatever the
// orientation), and Mozilla bug 1692316 (the native constants are
// "page-orientation-agnostic"). IPP two-sided-long-edge likewise names the
// long edge of the medium. The bridge never changes dmOrientation to
// compensate.
func devModeDuplex(sides core.Sides) int16 {
	switch sides {
	case core.TwoSidedLongEdge:
		return dmdupVertical
	case core.TwoSidedShortEdge:
		return dmdupHorizontal
	default:
		return dmdupSimplex
	}
}
