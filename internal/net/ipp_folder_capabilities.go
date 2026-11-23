package net

import "github.com/ilyasaftr/OpenPrintCast/internal/core"

const (
	// folderResolutionDPI is the resolution the folder destination reports.
	folderResolutionDPI = 300
	// DMPAPER ids of the two sheets the folder destination lists.
	folderPaperLetter = 1
	folderPaperA4     = 9
)

// folderCapabilities answers the capability questions for the Print to
// Folder destination, which has no Windows printer behind it. It
// implements the same narrow provider interfaces as the print backend, but
// with fixed answers: A4 and Letter without margins, 300 dpi, no duplex, no
// selectable quality, no orientation support, no supplies, always ready.
// Providers it does not implement (identify, print speed) are simply
// absent, so the matching attributes and operations are not offered.
type folderCapabilities struct{}

// capabilitySource returns what answers capability questions about printer:
// the fixed answers of the folder destination, or the print backend. It is
// the one place that keeps the folder destination away from Windows APIs.
func capabilitySource(shared *SharedState, printer core.PrinterInfo) any {
	if printer.IsFolderDestination() {
		return folderCapabilities{}
	}
	if shared == nil || shared.Spool == nil {
		return nil
	}
	return shared.Spool
}

func folderMedia(width, height, paperID int) core.MediaSize {
	return core.MediaSize{Width: width, Height: height, PaperID: paperID, MarginsKnown: true}
}

func (folderCapabilities) DefaultMediaDetails(string) (core.MediaDetails, bool) {
	return core.MediaDetails{
		Width: 21000, Height: 29700, MarginsKnown: true,
		DPIX: folderResolutionDPI, DPIY: folderResolutionDPI,
	}, true
}

func (folderCapabilities) SupportedMedia(string) []core.MediaSize {
	return []core.MediaSize{folderMedia(21000, 29700, folderPaperA4), folderMedia(21590, 27940, folderPaperLetter)}
}

func (folderCapabilities) MediaReady(string) bool { return true }

// OrientationSupported is true because IPP/2.0 requires it: a printer that
// cannot say it handles orientation is served as IPP/1.1 only, and iOS and
// macOS, which speak 2.0, then get server-error-version-not-supported for
// every request. The folder keeps each page as the sender made it, so a
// requested orientation is accepted and has no effect on the saved PDF.
func (folderCapabilities) OrientationSupported(string) bool { return true }

func (folderCapabilities) DuplexSupported(string) bool { return false }

// PrintQualities reports no selectable levels, as for a driver without them.
func (folderCapabilities) PrintQualities(string) ([]int, int, bool) { return nil, 0, false }

func (folderCapabilities) SupplyLevels(string) ([]core.Supply, string) { return nil, "" }

func (folderCapabilities) IPPStatus(string) (int, []string, bool) { return printerStateIdle, nil, true }
