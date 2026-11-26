package printerdev

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// errNoProbeSource says no probe can read this printer: it is not a USB
// device the transport can see, or no vendor probe recognizes it.
var errNoProbeSource = errors.New("no probe for this printer")

// Facts is what one probe of a printer learned. Every field is best
// effort: a printer that answers part of the questions still reports the
// rest empty.
type Facts struct {
	// Supplies are the consumable levels in the same shape the SNMP
	// reader reports them: ink that is used up and waste containers that
	// fill up.
	Supplies []core.Supply
	// Serial is the printer's serial number (printer-serial-number).
	Serial string
	// Firmware is the firmware version string
	// (printer-firmware-string-version).
	Firmware string
	// Model is the model the device claims in its IEEE 1284 device ID.
	Model string
	// StateReasons are printer-state-reasons keywords the device's error
	// bits imply (media-jam, cover-open...), without severity suffixes
	// the state pipeline adds.
	StateReasons []string
	// Page counters the device reports; a negative field means the
	// printer does not count that kind of sheet (the wire encoding of
	// "unsupported" is -1, so zero is a real count).
	PagesColor      int
	PagesMonochrome int
	PagesBlank      int
}

// TotalPages sums the per-kind page counters the printer reports into
// the total sheets fed. The second result is false when the printer
// counts nothing — a model without counters must not publish zero.
func (f Facts) TotalPages() (int, bool) {
	if f.PagesColor < 0 && f.PagesMonochrome < 0 && f.PagesBlank < 0 {
		return 0, false
	}
	total := 0
	for _, n := range []int{f.PagesColor, f.PagesMonochrome, f.PagesBlank} {
		if n > 0 {
			total += n
		}
	}
	return total, true
}

// device is an open USB printer stream: writes go to the printer's bulk
// output, reads come from its input. Close releases the handle.
type device interface {
	io.Reader
	io.Writer
	io.Closer
}

// deviceHandle is one USB printing device the transport sees: its open
// stream and its IEEE 1284 device ID, which names the manufacturer and
// model before any vendor protocol runs.
type deviceHandle struct {
	dev      device
	deviceID string
}

// prober is one vendor's implementation. matches tells the printer's from
// the IEEE 1284 device ID without touching the device; probe reads the
// facts over the open stream.
type prober interface {
	matches(deviceID string) bool
	probe(ctx context.Context, rw io.ReadWriter, deviceID string) (Facts, error)
}

// probeChain is the ordered list of vendor probes; Epson first, and the
// only one so far. A probe that matches but fails does not fall through
// to the next: the device is the vendor's, the failure is real.
var probeChain = []prober{epsonProber{}}

// probeMutex serializes probing machine-wide: two queues can share one
// physical printer (a queue and its copy), and simultaneous conversations
// on one cable would corrupt each other. Probes are seconds at most and
// rare, so the serialization costs nothing.
var probeMutex sync.Mutex

// Probe reads printer, which the Windows spooler reaches over a USB port.
// It finds the USB printing device the queue belongs to, offers it to the
// vendor probes in order and returns what the matching one reads.
// errNoProbeSource means no device matches or no probe recognizes it.
func Probe(ctx context.Context, printer string) (Facts, error) {
	probeMutex.Lock()
	defer probeMutex.Unlock()
	handle, err := openPrinterDevice(printer)
	if err != nil {
		return Facts{}, err
	}
	defer handle.dev.Close()
	for _, p := range probeChain {
		if !p.matches(handle.deviceID) {
			continue
		}
		return p.probe(ctx, handle.dev, handle.deviceID)
	}
	return Facts{}, errNoProbeSource
}

// field1284 is one semicolon-separated key of an IEEE 1284 device ID, as
// "MDL" of "MFG:EPSON;MDL:L3210 Series;...". Keys are matched without
// case; the common short and long spellings are one key.
func field1284(deviceID, key string) string {
	for _, field := range strings.Split(deviceID, ";") {
		name, value, found := strings.Cut(field, ":")
		if found && strings.EqualFold(strings.TrimSpace(name), key) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// normalize squashes a name for comparison: lowercase, letters and digits
// only, so "EPSON L3210 Series" and "l3210series" compare equal.
func normalize(text string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(text) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// deviceNameMatches reports whether the normalized queue name fits the
// device ID: the model of the ID inside the queue name, or the
// description and the queue name containing each other.
func deviceNameMatches(want, deviceID string) bool {
	if want == "" {
		return false
	}
	for _, key := range []string{"MDL", "DES"} {
		field := normalize(field1284(deviceID, key))
		if field == "" {
			continue
		}
		if strings.Contains(want, field) || strings.Contains(field, want) {
			return true
		}
	}
	return false
}
