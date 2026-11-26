package net

import (
	"context"
	"errors"
	"slices"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

type mediaProvider interface {
	DefaultMedia(printer string) (width, height int, ok bool)
}

type detailedMediaProvider interface {
	DefaultMediaDetails(printer string) (core.MediaDetails, bool)
}

type supportedMediaProvider interface {
	SupportedMedia(printer string) []core.MediaSize
}

type mediaReadyProvider interface {
	MediaReady(printer string) bool
}

type identifyProvider interface {
	IdentifyPrinter(printer string) error
}

type orientationProvider interface {
	OrientationSupported(printer string) bool
}

type statusProvider interface {
	IPPStatus(printer string) (state int, reasons []string, ok bool)
}

type printSpeedProvider interface {
	PagesPerMinute(printer string) (int, bool)
}

type colorPrintSpeedProvider interface {
	ColorPagesPerMinute(printer string) (int, bool)
}

type printQualityProvider interface {
	PrintQualities(printer string) (supported []int, def int, ok bool)
}

type duplexProvider interface {
	DuplexSupported(printer string) bool
}

// supplyProvider reports the toner and ink levels a printer's own network
// port allows reading. SupplyLevels never waits for the network; it gives no
// supplies when there is no data, and printerHost is the printer's own
// address when known ("" otherwise).
type supplyProvider interface {
	SupplyLevels(printer string) (supplies []core.Supply, printerHost string)
}

// deviceFactsProvider reports what the printer said about itself when read
// straight from the device: serial number, firmware version and error
// states. All are empty when the printer has not been read this way.
type deviceFactsProvider interface {
	DeviceFacts(printer string) (serial, firmware string, stateReasons []string)
}

type optionSpooler interface {
	SpoolJob(string, []byte, string, core.PrintOptions) error
}

type folderJobSpooler interface {
	SpoolFolderJobContext(ctx context.Context, data []byte, mime string, options core.PrintOptions, folder, jobName string) error
}

var (
	errNoFolderBackend  = errors.New("print backend does not support Print to Folder")
	errNoOptionsBackend = errors.New("print backend does not support job options")
)

// duplexSupported reports whether the print backend offers two-sided output
// for printer; without a provider the bridge stays one-sided.
func duplexSupported(shared *SharedState, printer core.PrinterInfo) bool {
	if shared == nil {
		return false
	}
	provider, ok := capabilitySource(shared, printer).(duplexProvider)
	return ok && provider.DuplexSupported(printer.WindowsName)
}

// deviceFacts returns what the print backend read straight from the
// printer itself; without a provider every fact is empty.
func deviceFacts(shared *SharedState, printer core.PrinterInfo) (serial, firmware string, stateReasons []string) {
	if shared == nil {
		return "", "", nil
	}
	provider, ok := capabilitySource(shared, printer).(deviceFactsProvider)
	if !ok {
		return "", "", nil
	}
	return provider.DeviceFacts(printer.WindowsName)
}

// supplyLevels returns the supplies the print backend can report for
// printer, and the printer's own host. Without a provider, or without data,
// there are none: the bridge never makes levels up.
func supplyLevels(shared *SharedState, printer core.PrinterInfo) ([]core.Supply, string) {
	if shared == nil {
		return nil, ""
	}
	provider, ok := capabilitySource(shared, printer).(supplyProvider)
	if !ok {
		return nil, ""
	}
	return provider.SupplyLevels(printer.WindowsName)
}

// IPP print-quality enum values (RFC 8011 section 5.2.13).
const (
	printQualityDraft  = 3
	printQualityNormal = 4
	printQualityHigh   = 5
)

// The bridge's normal quality is the unmodified driver default when the
// driver does not expose selectable quality levels.
func printQualities(shared *SharedState, printer core.PrinterInfo) (supported []int, def int, driverSelectable bool) {
	if shared != nil {
		if provider, ok := capabilitySource(shared, printer).(printQualityProvider); ok {
			supported, def, known := provider.PrintQualities(printer.WindowsName)
			if known && len(supported) > 0 && slices.Contains(supported, def) {
				return supported, def, true
			}
		}
	}
	return []int{printQualityNormal}, printQualityNormal, false
}
