package winspool

import (
	"context"
	"strings"
	"sync"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// Backend abstracts the OS print spooler so UI and network layers depend
// on it without platform coupling. It mirrors the Rust PrinterBackend
// trait: implementations are safe to share across goroutines.
type Backend interface {
	// ListPrinters returns installed printers (local + connections),
	// default first.
	ListPrinters() []PrinterDescriptor
	// SpoolBuffer spools an in-memory document (data, e.g. PDF or raster
	// bytes, described by MIME type mime) to printer.
	SpoolBuffer(printer string, data []byte, mime string) error
	// QueryStatus returns the best-effort live state of printer; it
	// never fails — unknown printers report PrinterStatusUnknown.
	QueryStatus(printer string) PrinterStatus
}

// SystemBackend is the production Backend: the real Windows spooler on
// windows, graceful stubs elsewhere. The zero value is ready to use.
// Instances are shareable across goroutines.
type SystemBackend struct {
	settings *renderSettingsState
	supplies *supplyStore
}

type RenderSettings struct {
	MaxDPI          int
	ReversePages    bool
	AutoRotateToFit bool
	options         core.PrintOptions
	ctx             context.Context
}

type renderSettingsState struct {
	mu          sync.RWMutex
	mediaMu     sync.Mutex
	settings    RenderSettings
	orientation capabilityCache[bool]
	duplex      capabilityCache[bool]
	qualities   capabilityCache[qualityCapability]
	media       capabilityCache[[]core.MediaSize]
}

// NewSystemBackend builds a backend instance.
func NewSystemBackend() SystemBackend {
	return SystemBackend{
		settings: &renderSettingsState{settings: RenderSettings{MaxDPI: core.DefaultRenderDPI}},
		supplies: newSupplyStore(fetchSupplies),
	}
}

func (b SystemBackend) SetRenderSettings(settings RenderSettings) {
	if b.settings == nil {
		return
	}
	settings.MaxDPI = core.ClampRenderDPI(settings.MaxDPI)
	b.settings.mu.Lock()
	b.settings.settings = settings
	b.settings.mu.Unlock()
}

func (b SystemBackend) renderSettings() RenderSettings {
	if b.settings == nil {
		return RenderSettings{MaxDPI: core.DefaultRenderDPI}
	}
	b.settings.mu.RLock()
	defer b.settings.mu.RUnlock()
	return b.settings.settings
}

// Compile-time check that SystemBackend implements Backend.
var _ Backend = SystemBackend{}

// ListPrinters enumerates installed printers (local + connections),
// default first.
func (b SystemBackend) ListPrinters() []PrinterDescriptor {
	printers := platformList()
	sortPrinters(printers)
	return printers
}

// SpoolBuffer validates its inputs without touching the spooler, then
// delegates to the platform spool path.
func (b SystemBackend) SpoolBuffer(printer string, data []byte, mime string) error {
	return b.SpoolJob(printer, data, mime, core.PrintOptions{})
}

// SpoolJob applies document-specific IPP options without changing global settings.
func (b SystemBackend) SpoolJob(printer string, data []byte, mime string, options core.PrintOptions) error {
	return b.SpoolJobContext(context.Background(), printer, data, mime, options)
}

// SpoolJobContext permits cancellation before the Windows spool job is closed.
func (b SystemBackend) SpoolJobContext(ctx context.Context, printer string, data []byte, mime string, options core.PrintOptions) error {
	return b.spoolJobContext(ctx, printer, data, mime, options)
}

func (b SystemBackend) spoolJobContext(ctx context.Context, printer string, data []byte, mime string, options core.PrintOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(printer) == 0 || len(trimSpace(printer)) == 0 {
		return NotFoundError("<empty printer name>")
	}
	if len(data) == 0 {
		return SpoolError(printer, "empty document buffer")
	}
	if !options.Valid() {
		return SpoolError(printer, "invalid print options")
	}
	settings := b.renderSettings()
	settings.options = options
	settings.ctx = ctx
	return platformSpool(printer, data, mime, settings)
}

// DefaultMedia returns the driver's current paper size in hundredths of a
// millimetre. A false result means the driver did not report a usable size.
func (b SystemBackend) DefaultMedia(printer string) (int, int, bool) {
	return platformDefaultMedia(printer)
}

func (b SystemBackend) DefaultMediaDetails(printer string) (core.MediaDetails, bool) {
	return platformDefaultMediaDetails(printer)
}

// SupportedMedia reports driver-validated paper sizes without changing defaults.
func (b SystemBackend) SupportedMedia(printer string) []core.MediaSize {
	if b.settings == nil {
		return platformSupportedMedia(printer)
	}
	// Enumerating and validating driver forms can involve slow Windows calls.
	// Coalesce concurrent AirPrint capability queries.
	b.settings.mediaMu.Lock()
	defer b.settings.mediaMu.Unlock()
	if cached, ok := b.settings.media.lookup(&b.settings.mu, printer); ok {
		return append([]core.MediaSize(nil), cached...)
	}
	sizes := platformSupportedMedia(printer)
	b.settings.media.store(&b.settings.mu, printer, append([]core.MediaSize(nil), sizes...))
	return sizes
}

func (b SystemBackend) MediaReady(printer string) bool {
	return b.QueryStatus(printer).AcceptsJobs()
}

func (b SystemBackend) OrientationSupported(printer string) bool {
	if b.settings == nil {
		return platformSupportsOrientation(printer)
	}
	return probeCached(&b.settings.mu, &b.settings.orientation, printer, platformSupportsOrientation)
}

// DuplexSupported reports whether the driver both reports duplex printing and
// accepts two-sided settings; false when either check fails.
func (b SystemBackend) DuplexSupported(printer string) bool {
	if b.settings == nil {
		return platformSupportsDuplex(printer)
	}
	return probeCached(&b.settings.mu, &b.settings.duplex, printer, platformSupportsDuplex)
}

func (b SystemBackend) PrintQualities(printer string) ([]int, int, bool) {
	if b.settings == nil {
		return platformPrintQualities(printer)
	}
	if cached, ok := b.settings.qualities.lookup(&b.settings.mu, printer); ok {
		return append([]int(nil), cached.supported...), cached.def, cached.known
	}
	supported, def, known := platformPrintQualities(printer)
	b.settings.qualities.store(&b.settings.mu, printer, qualityCapability{supported: append([]int(nil), supported...), def: def, known: known})
	return supported, def, known
}

// SetReadSupplyLevels switches reading ink and toner levels over SNMP on or
// off at once. It starts off: until the application enables it from its
// settings, the backend sends no SNMP traffic.
func (b SystemBackend) SetReadSupplyLevels(on bool) {
	if b.supplies != nil {
		b.supplies.SetEnabled(on)
	}
}

// SupplyLevels returns the printer's last known supplies and the address of
// the printer itself (for its web page; "" when unknown). It never waits
// for the network: the answer comes from a cache that refreshes in the
// background. Printers whose levels cannot be read, and every printer
// while reading is off, give no supplies.
func (b SystemBackend) SupplyLevels(printer string) ([]core.Supply, string) {
	if b.supplies == nil {
		return nil, ""
	}
	reading := b.supplies.Get(printer)
	return append([]core.Supply(nil), reading.Supplies...), reading.Host
}

// DeviceFacts returns what a read straight from the device learned beyond
// the supplies: the serial number, the firmware version and the device's
// own error states as printer-state-reasons keywords. They come from the
// same cache the supplies do, so they are as fresh — and network printers,
// which are read over SNMP, carry none.
func (b SystemBackend) DeviceFacts(printer string) (serial, firmware string, stateReasons []string) {
	if b.supplies == nil {
		return "", "", nil
	}
	reading := b.supplies.Get(printer)
	return reading.Serial, reading.Firmware, reading.StateReasons
}

// ColorSupported reports the driver's color capability, if available.
func (b SystemBackend) ColorSupported(printer string) (bool, bool) {
	return platformColorSupported(printer)
}

// PagesPerMinute reports a driver-supplied nominal speed, if available.
func (b SystemBackend) PagesPerMinute(printer string) (int, bool) {
	return platformPagesPerMinute(printer)
}

// IdentifyPrinter audibly identifies the Windows bridge hosting the printer.
func (b SystemBackend) IdentifyPrinter(printer string) error {
	return platformIdentifyPrinter(printer)
}

// QueryStatus returns the best-effort live state of printer; unknown
// names and query failures report PrinterStatusUnknown instead of
// erroring.
func (b SystemBackend) QueryStatus(printer string) PrinterStatus {
	if len(trimSpace(printer)) == 0 {
		return PrinterStatusUnknown
	}
	return platformStatus(printer)
}

func (b SystemBackend) IPPStatus(printer string) (int, []string, bool) {
	bits, ok := platformStatusBits(printer)
	if !ok {
		return 0, nil, false
	}
	state, reasons := IPPStateFromWinspoolBits(bits)
	return state, reasons, true
}

// ListPrinters is a package-level convenience wrapper over
// SystemBackend.ListPrinters.
func ListPrinters() []PrinterDescriptor {
	return SystemBackend{}.ListPrinters()
}

// QueryStatus is a package-level convenience wrapper over
// SystemBackend.QueryStatus.
func QueryStatus(printer string) PrinterStatus {
	return SystemBackend{}.QueryStatus(printer)
}

// trimSpace trims surrounding whitespace for blank-name detection.
func trimSpace(s string) string { return strings.TrimSpace(s) }
