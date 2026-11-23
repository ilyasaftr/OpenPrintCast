package core

import (
	"strconv"
	"sync/atomic"
	"time"
)

// Job lifecycle states, serialized lowercase to match the history file.
type JobStatus string

const (
	JobPending   JobStatus = "pending"
	JobPrinting  JobStatus = "printing"
	JobCompleted JobStatus = "completed"
	JobFailed    JobStatus = "failed"
	JobCancelled JobStatus = "cancelled"
)

// Finished reports whether the job will never change state again.
func (s JobStatus) Finished() bool {
	return s == JobCompleted || s == JobFailed || s == JobCancelled
}

// PrinterEntry is the persisted per-printer record: the Windows name is
// the stable identity (Windows printer names are case-insensitive);
// everything else is user customisation that must survive restarts.
type PrinterEntry struct {
	WindowsName  string `json:"windows_name"`
	AirPrintName string `json:"airprint_name"`
	Location     string `json:"location"`
	Enabled      bool   `json:"enabled"`
	// VirtualOverride is the user's own virtual/physical verdict; nil
	// leaves the decision to automatic detection.
	VirtualOverride *bool `json:"virtual_override,omitempty"`
}

// PrinterInfo describes one Windows printer exposed over AirPrint.
type PrinterInfo struct {
	// Stable bridge-side identifier (mDNS rp path and job foreign key).
	ID string `json:"id"`
	// Name as known to Windows (win32print).
	WindowsName string `json:"windows_name"`
	// Name advertised to AirPrint clients.
	AirPrintName string `json:"airprint_name"`
	// Human-readable location, shown in the TXT `note` key.
	Location string `json:"location"`
	// Whether the printer currently accepts jobs.
	Enabled bool `json:"enabled"`
	// Nil means the Windows driver did not report its color capabilities.
	ColorCapable *bool `json:"-"`
	// Nil means the Windows driver did not report duplex support. It is true
	// only when the driver both reports duplex and accepts a duplex setting.
	DuplexCapable *bool `json:"-"`
	// Driver and classification are discovered at runtime, not persisted.
	DriverName string `json:"-"`
	// IsVirtual is the effective verdict: VirtualOverride when set, else
	// DetectedVirtual. Change it only through SetDetectedVirtual and
	// SetVirtualOverride.
	IsVirtual bool `json:"-"`
	IsRemote  bool `json:"-"`
	// IsDefault marks the Windows default printer.
	IsDefault bool `json:"-"`
	// DetectedVirtual and VirtualReason are what automatic detection
	// found and why.
	DetectedVirtual bool   `json:"-"`
	VirtualReason   string `json:"-"`
	// VirtualOverride is the user's verdict; nil means automatic.
	VirtualOverride *bool `json:"-"`
	// FolderPath marks the bridge-owned destination that saves PDF files.
	FolderPath string `json:"-"`
}

// MediaDetails describes the active Windows driver paper and imageable area
// in hundredths of a millimeter.
type MediaDetails struct {
	Width, Height            int
	Left, Right, Top, Bottom int
	DPIX, DPIY               int
	MarginsKnown             bool
}

// MediaSize identifies a driver-supported paper size in hundredths of a mm.
type MediaSize struct {
	Width, Height            int
	PaperID                  int
	Left, Right, Top, Bottom int
	MarginsKnown             bool
}

func (p PrinterInfo) SupportsColor() bool {
	return p.ColorCapable != nil && *p.ColorCapable
}

// SupportsDuplex reports whether two-sided output may be offered.
func (p PrinterInfo) SupportsDuplex() bool {
	return p.DuplexCapable != nil && *p.DuplexCapable
}

// NewPrinterInfo builds a PrinterInfo record.
func NewPrinterInfo(id, windowsName, airPrintName, location string, enabled bool) PrinterInfo {
	return PrinterInfo{
		ID:           id,
		WindowsName:  windowsName,
		AirPrintName: airPrintName,
		Location:     location,
		Enabled:      enabled,
	}
}

// SetDetectedVirtual records the automatic verdict and its reason, then
// refreshes the effective IsVirtual.
func (p *PrinterInfo) SetDetectedVirtual(virtual bool, reason string) {
	p.DetectedVirtual, p.VirtualReason = virtual, reason
	p.resolveVirtual()
}

// SetVirtualOverride sets the user's verdict (nil for automatic) and
// refreshes the effective IsVirtual. The pointer is copied.
func (p *PrinterInfo) SetVirtualOverride(override *bool) {
	p.VirtualOverride = nil
	if override != nil {
		value := *override
		p.VirtualOverride = &value
	}
	p.resolveVirtual()
}

// resolveVirtual makes IsVirtual the override when set, else the detected
// verdict.
func (p *PrinterInfo) resolveVirtual() {
	p.IsVirtual = p.DetectedVirtual
	if p.VirtualOverride != nil {
		p.IsVirtual = *p.VirtualOverride
	}
}

// FolderPrinterName is the WindowsName of the Print to Folder destination.
// No Windows queue stands behind that destination, so the name must never
// match one: local Windows printer names cannot contain a backslash, and
// printer connections start with two of them (\\server\name), so a name
// with a single leading backslash is neither.
const FolderPrinterName = `\Print to Folder`

// folderModelName is how the Print to Folder destination presents itself
// where other printers show their Windows name.
const folderModelName = "Print to Folder"

// IsFolderDestination reports whether p is the Print to Folder destination,
// which has no Windows printer behind it.
func (p PrinterInfo) IsFolderDestination() bool { return p.WindowsName == FolderPrinterName }

// ModelName is the product name advertised for the printer: its Windows name,
// or a readable name for the destination that has none.
func (p PrinterInfo) ModelName() string {
	if p.IsFolderDestination() {
		return folderModelName
	}
	return p.WindowsName
}

// DisplayName is the AirPrint-visible name, falling back to Windows name.
func (p PrinterInfo) DisplayName() string {
	if p.AirPrintName != "" {
		return p.AirPrintName
	}
	return p.WindowsName
}

var jobCounter atomic.Uint64

// JobEntry is one print job tracked by the bridge.
type JobEntry struct {
	// Unique id (`job-<epoch_ms>-<counter>`).
	ID string `json:"id"`
	// PrinterInfo.ID of the target printer.
	PrinterID string `json:"printer_id"`
	// User or client that submitted the job.
	User string `json:"user"`
	// Original document file name.
	Filename string `json:"filename"`
	// Total page count (0 when unknown).
	Pages int `json:"pages"`
	// Current lifecycle state.
	Status JobStatus `json:"status"`
	// Creation time as Unix epoch seconds.
	CreatedAt int64 `json:"created_at"`
	// Short human-readable failure or cancel reason ("" when none).
	Reason string `json:"reason,omitempty"`
}

// NewJobEntry creates a pending job, stamping id and creation time.
func NewJobEntry(printerID, user, filename string, pages int) JobEntry {
	now := time.Now()
	return JobEntry{
		ID:        "job-" + strconv.FormatInt(now.UnixMilli(), 10) + "-" + strconv.FormatUint(jobCounter.Add(1), 10),
		PrinterID: printerID,
		User:      user,
		Filename:  filename,
		Pages:     pages,
		Status:    JobPending,
		CreatedAt: now.Unix(),
	}
}

// SetStatus moves the job into a new state.
func (j *JobEntry) SetStatus(status JobStatus) { j.Status = status }
