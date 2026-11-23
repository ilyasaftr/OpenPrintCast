package winspool

// PrinterStatus is the live state of a printer queue.
type PrinterStatus string

// Printer queue states. The zero value is PrinterStatusUnknown so a
// printer whose state could not be determined is never mistaken for a
// ready queue.
const (
	// PrinterStatusReady is idle and accepting jobs.
	PrinterStatusReady PrinterStatus = "ready"
	// PrinterStatusPrinting is actively printing or processing a job.
	PrinterStatusPrinting PrinterStatus = "printing"
	// PrinterStatusPaused is a queue paused by the user or driver.
	PrinterStatusPaused PrinterStatus = "paused"
	// PrinterStatusOffline is offline, jammed, out of paper/toner, or
	// otherwise errored.
	PrinterStatusOffline PrinterStatus = "offline"
	// PrinterStatusUnknown means the printer is unknown or its state
	// could not be determined.
	PrinterStatusUnknown PrinterStatus = "unknown"
)

// AsCode returns the stable lowercase code for logging and core mapping.
func (s PrinterStatus) AsCode() string { return string(s) }

// AcceptsJobs reports whether the queue is expected to accept a new job
// right now.
func (s PrinterStatus) AcceptsJobs() bool {
	return s == PrinterStatusReady || s == PrinterStatusPrinting
}

// WinSpool PRINTER_STATUS_* bitmask values (from WINEPRINT.H). Kept as
// local constants so the mapping stays testable off Windows.
const (
	statusPaused           uint32 = 0x00000001
	statusError            uint32 = 0x00000002
	statusPendingDeletion  uint32 = 0x00000004
	statusPaperJam         uint32 = 0x00000008
	statusPaperOut         uint32 = 0x00000010
	statusManualFeed       uint32 = 0x00000020
	statusPaperProblem     uint32 = 0x00000040
	statusOffline          uint32 = 0x00000080
	statusIOActive         uint32 = 0x00000100
	statusBusy             uint32 = 0x00000200
	statusPrinting         uint32 = 0x00000400
	statusOutputBinFull    uint32 = 0x00000800
	statusNotAvailable     uint32 = 0x00001000
	statusWaiting          uint32 = 0x00002000
	statusProcessing       uint32 = 0x00004000
	statusInitializing     uint32 = 0x00008000
	statusWarmingUp        uint32 = 0x00010000
	statusTonerLow         uint32 = 0x00020000
	statusNoToner          uint32 = 0x00040000
	statusPagePunt         uint32 = 0x00080000
	statusUserIntervention uint32 = 0x00100000
	statusOutOfMemory      uint32 = 0x00200000
	statusDoorOpen         uint32 = 0x00400000
	statusServerUnknown    uint32 = 0x00800000
	statusPowerSave        uint32 = 0x01000000
)

// faultBits are hard-fault bits: the device cannot print until the
// condition clears. Includes pending-deletion (the queue is going away)
// and manual-feed (the job is blocked until an operator feeds paper).
const faultBits uint32 = statusError |
	statusPendingDeletion |
	statusPaperJam |
	statusPaperOut |
	statusManualFeed |
	statusPaperProblem |
	statusOffline |
	statusOutputBinFull |
	statusNotAvailable |
	statusNoToner |
	statusPagePunt |
	statusUserIntervention |
	statusOutOfMemory |
	statusDoorOpen |
	statusServerUnknown

// activeBits are busy bits: the device is working but otherwise healthy.
const activeBits uint32 = statusPrinting |
	statusIOActive |
	statusBusy |
	statusProcessing |
	statusWaiting

// StatusFromWinspoolBits maps a WinSpool PRINTER_INFO_2.Status bitmask to
// a PrinterStatus.
//
// Precedence mirrors operator intent: paused beats everything (the queue
// holds jobs regardless of device state), hard faults beat activity, and
// a zero bitmask is idle-ready. Degraded-but-printable states (warming
// up, toner low, power save) and unknown future bits stay
// PrinterStatusReady: the queue accepts jobs.
func StatusFromWinspoolBits(bits uint32) PrinterStatus {
	if bits&statusPaused != 0 {
		return PrinterStatusPaused
	}
	if bits&faultBits != 0 {
		return PrinterStatusOffline
	}
	if bits&activeBits != 0 {
		return PrinterStatusPrinting
	}
	return PrinterStatusReady
}

// statusFromWinspoolBits is the package-private alias of
// StatusFromWinspoolBits used by the platform query paths.
func statusFromWinspoolBits(bits uint32) PrinterStatus {
	return StatusFromWinspoolBits(bits)
}

// IPP printer-state enum values (RFC 8011 section 5.4.11).
const (
	ippStateIdle       = 3
	ippStateProcessing = 4
	ippStateStopped    = 5
)

// IPPStateFromWinspoolBits maps known Windows queue conditions to IPP
// printer-state and printer-state-reasons.
func IPPStateFromWinspoolBits(bits uint32) (int, []string) {
	state := ippStateIdle
	switch StatusFromWinspoolBits(bits) {
	case PrinterStatusPaused, PrinterStatusOffline:
		state = ippStateStopped
	case PrinterStatusPrinting:
		state = ippStateProcessing
	}
	var reasons []string
	add := func(mask uint32, reason string) {
		if bits&mask != 0 {
			reasons = append(reasons, reason)
		}
	}
	add(statusPaused, "paused")
	add(statusPaperOut, "media-empty")
	add(statusPaperOut|statusManualFeed, "media-needed")
	add(statusPaperJam, "media-jam")
	add(statusDoorOpen, "door-open")
	add(statusNoToner, "marker-supply-empty")
	add(statusTonerLow, "marker-supply-low-warning")
	add(statusOutputBinFull, "output-area-full")
	add(statusOffline|statusNotAvailable|statusServerUnknown, "offline")
	add(statusError|statusPendingDeletion|statusPaperProblem|statusPagePunt|statusUserIntervention|statusOutOfMemory, "other")
	if len(reasons) == 0 {
		reasons = append(reasons, "none")
	}
	return state, reasons
}
