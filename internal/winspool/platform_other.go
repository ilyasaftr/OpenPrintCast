//go:build !windows

package winspool

import (
	"fmt"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// platformList has no spooler to reach off Windows, so it reports no
// printers and lets Linux CI pass.
func platformList() []PrinterDescriptor { return nil }

func platformDefaultMedia(printer string) (int, int, bool) { return 0, 0, false }

func platformDefaultMediaDetails(printer string) (core.MediaDetails, bool) {
	return core.MediaDetails{}, false
}

func platformSupportedMedia(printer string) []core.MediaSize { return nil }

func platformColorSupported(printer string) (bool, bool) { return false, false }

func platformPagesPerMinute(printer string) (int, bool) { return 0, false }

func platformSupportsOrientation(printer string) bool { return false }

func platformSupportsDuplex(printer string) bool { return false }

func platformPrintQualities(printer string) ([]int, int, bool) { return nil, 0, false }

func platformIdentifyPrinter(printer string) error {
	return UnsupportedError("printer identification requires Windows")
}

// platformSpool has no spooler to reach off Windows, so spooling always
// reports KindUnsupported and lets Linux CI pass.
func platformSpool(printer string, data []byte, mime string, settings RenderSettings) error {
	return UnsupportedError(fmt.Sprintf(
		"spoolBuffer('%s', %d bytes, '%s') requires Windows", printer, len(data), mime))
}

// platformStatus has no spooler to reach off Windows, so the state is
// always unknown and Linux CI passes.
func platformStatus(printer string) PrinterStatus { return PrinterStatusUnknown }

func platformStatusBits(printer string) (uint32, bool) { return 0, false }
