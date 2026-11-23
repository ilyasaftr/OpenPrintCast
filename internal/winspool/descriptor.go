package winspool

import (
	"sort"
	"strings"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// PrinterDescriptor is a discovered Windows printer in portable form.
type PrinterDescriptor struct {
	// Display name as reported by EnumPrinters (e.g. "Office Laser").
	Name string
	// DriverName is the Windows spooler's driver name, independent of the
	// user-configurable queue name.
	DriverName string
	// PortName is the spooler port the queue prints to.
	PortName string
	// Whether this is the user's default printer.
	IsDefault bool
	// IsRemote indicates a connection to another print server.
	IsRemote bool
	// Whether this is a virtual/non-physical printer, as ClassifyPrinter
	// judged it.
	IsVirtual bool
	// VirtualReason is the evidence behind IsVirtual, for display.
	VirtualReason string
}

// NewPrinterDescriptorFromEvidence builds a descriptor from the spooler
// evidence, classifying it with ClassifyPrinter.
func NewPrinterDescriptorFromEvidence(e PrinterEvidence, isDefault bool) PrinterDescriptor {
	verdict := ClassifyPrinter(e)
	return PrinterDescriptor{
		Name:          e.Name,
		DriverName:    e.DriverName,
		PortName:      e.PortName,
		IsDefault:     isDefault,
		IsRemote:      e.IsNetwork,
		IsVirtual:     verdict.Virtual,
		VirtualReason: verdict.Reason,
	}
}

// NewPrinterDescriptor builds a descriptor that knows only the name, for
// backends without driver or port data.
func NewPrinterDescriptor(name string, isDefault bool) PrinterDescriptor {
	return NewPrinterDescriptorFromEvidence(PrinterEvidence{Name: name}, isDefault)
}

// IsVirtualPrinter reports whether the name alone identifies a software
// printer; it cannot see the port or driver.
func IsVirtualPrinter(name string) bool {
	return ClassifyPrinter(PrinterEvidence{Name: name}).Virtual
}

// sortKey orders physical printers before virtual ones and the default
// printer first within each group, mirroring the Python preference for
// physical printers over built-in virtual ones. Default-ness outranks
// physicality so an explicit default always wins.
func (d PrinterDescriptor) sortKey() (notDefault, virtual bool, folded string) {
	return !d.IsDefault, d.IsVirtual, strings.ToLower(d.Name)
}

// lessPrinters reports whether a sorts before b under sortKey ordering.
func lessPrinters(a, b PrinterDescriptor) bool {
	an, av, af := a.sortKey()
	bn, bv, bf := b.sortKey()
	if an != bn {
		return !an
	}
	if av != bv {
		return !av
	}
	return af < bf
}

// sortPrinters orders descriptors default-first, physical-first, then by
// case-insensitive name.
func sortPrinters(printers []PrinterDescriptor) {
	sort.SliceStable(printers, func(i, j int) bool {
		return lessPrinters(printers[i], printers[j])
	})
}

// SortPrinters orders descriptors default-first, physical-first, then by
// case-insensitive name. It sorts in place.
func SortPrinters(printers []PrinterDescriptor) { sortPrinters(printers) }

// ToCore converts the descriptor into the shared core.PrinterInfo record.
// The caller supplies the bridge-side id, advertised AirPrint name and
// location; enabled should come from PrinterStatus.AcceptsJobs for the
// live queue state. This is the dependency-inversion seam: core never
// sees WinSpool types.
func (d PrinterDescriptor) ToCore(id, airPrintName, location string, enabled bool) core.PrinterInfo {
	return core.NewPrinterInfo(id, d.Name, airPrintName, location, enabled)
}
