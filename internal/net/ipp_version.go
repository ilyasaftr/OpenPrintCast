package net

import (
	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// NegotiateVersion returns the nearest implemented version for a request.
// Per-printer IPP/2.0 availability is checked separately.
func NegotiateVersion(client goipp.Version) (serve, echo goipp.Version) {
	if client.Major() == 2 {
		return ipp20, ipp20
	}
	return ipp11, ipp11
}

// ipp20Gap is one capability a printer's driver must report before the
// printer can be served as IPP/2.0.
type ipp20Gap int

const (
	gapOrientation ipp20Gap = iota
	gapPaperSizes
	gapResolution
)

// Stable machine keys of the gaps, for callers outside this package.
const (
	GapKeyOrientation = "orientation"
	GapKeyPaperSizes  = "paper-sizes"
	GapKeyResolution  = "resolution"
)

// key is the stable machine name of the gap.
func (g ipp20Gap) key() string {
	switch g {
	case gapOrientation:
		return GapKeyOrientation
	case gapPaperSizes:
		return GapKeyPaperSizes
	}
	return GapKeyResolution
}

// A printer cannot claim IPP/2.0 when mandatory media, resolution or
// orientation capabilities cannot be established from its driver.
func supportsIPP20(shared *SharedState, printer core.PrinterInfo) bool {
	gaps, known := ipp20Gaps(shared, printer)
	return known && len(gaps) == 0
}

// ipp20Gaps lists the IPP/2.0 requirements the printer's driver does not
// meet. known is false when the capabilities cannot be asked at all (no
// shared state or no orientation provider); the server then treats the
// printer as not ready, while a caller that only informs the user must not.
func ipp20Gaps(shared *SharedState, printer core.PrinterInfo) (gaps []ipp20Gap, known bool) {
	if shared == nil {
		return nil, false
	}
	provider, ok := capabilitySource(shared, printer).(orientationProvider)
	if !ok {
		return nil, false
	}
	if !provider.OrientationSupported(printer.WindowsName) {
		gaps = append(gaps, gapOrientation)
	}
	attrs := mediaAttrs(shared, printer)
	if missingAny(attrs, "media-default", "media-supported") {
		gaps = append(gaps, gapPaperSizes)
	}
	if missingAny(attrs, "printer-resolution-default", "printer-resolution-supported") {
		gaps = append(gaps, gapResolution)
	}
	return gaps, true
}

// missingAny reports whether any of the named attributes is absent or empty.
func missingAny(attrs goipp.Attributes, names ...string) bool {
	for _, name := range names {
		if opAttrString(attrs, name) == "" {
			return true
		}
	}
	return false
}

// IPP20Gaps reports which IPP/2.0 requirements the printer's driver does not
// meet, as the stable keys GapKeyOrientation, GapKeyPaperSizes and
// GapKeyResolution. known is false when that could not be determined; gaps
// is then empty and must not be shown as a problem. It may query the
// driver, so call it off the UI thread.
func IPP20Gaps(shared *SharedState, printer core.PrinterInfo) (gaps []string, known bool) {
	found, known := ipp20Gaps(shared, printer)
	for _, g := range found {
		gaps = append(gaps, g.key())
	}
	return gaps, known
}

// versionResponse picks the version to echo and reports whether the client's
// major version can be served.
func versionResponse(client goipp.Version, shared *SharedState, printer core.PrinterInfo) (goipp.Version, bool) {
	major := client.Major()
	if major == 1 {
		return ipp11, true
	}
	if major < 2 || !supportsIPP20(shared, printer) {
		return ipp11, false
	}
	return ipp20, major == 2
}
