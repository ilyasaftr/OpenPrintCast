package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
	"github.com/ilyasaftr/OpenPrintCast/internal/winspool"
)

const (
	folderPrinterID = "bridge-print-to-folder"
	printerIDPrefix = "printer-"
)

// folderPrinter is the Print to Folder destination, present once it is
// shared and a folder is chosen. No Windows printer stands behind it: it
// takes the fixed name core.FolderPrinterName and states its own capabilities.
func (a *bridgeApp) folderPrinter() (core.PrinterInfo, bool) {
	if !a.cfg.FolderEnabled || !filepath.IsAbs(a.cfg.FolderPath) {
		return core.PrinterInfo{}, false
	}
	color, duplex := true, false
	folder := core.NewPrinterInfo(folderPrinterID, core.FolderPrinterName,
		fmt.Sprintf("Print to Folder (%s)", a.host), "", true)
	folder.ColorCapable = &color
	folder.DuplexCapable = &duplex
	folder.FolderPath = a.cfg.FolderPath
	return folder, true
}

func (a *bridgeApp) visiblePrinter(p core.PrinterInfo) bool {
	return !p.IsVirtual || a.cfg.ShowVirtualPrinters || p.Enabled
}

// seedPrinters converts the spooler's descriptors into bridge printer
// records, mirroring the Rust shell: stable "printer-<index>" ids, an
// AirPrint name carrying the PC host label so same-named printers on
// several PCs stay distinguishable on iOS, physical printers shared and
// built-in virtual ones off. It takes the Backend interface (not the
// system spooler) so seeding stays unit-testable off Windows.
func seedPrinters(backend winspool.Backend, host string) []core.PrinterInfo {
	descriptors := backend.ListPrinters()
	printers := make([]core.PrinterInfo, 0, len(descriptors))
	for _, descriptor := range descriptors {
		printers = append(printers, seedPrinter(backend, descriptor, host))
	}
	return printers
}

// seedPrinter builds the bridge record for one spooler printer.
func seedPrinter(backend winspool.Backend, descriptor winspool.PrinterDescriptor, host string) core.PrinterInfo {
	printer := descriptor.ToCore(
		// Stable slug ids, not enumeration indexes: indexes shift
		// when printers are added/removed, orphaning the UUIDs and
		// rp paths iOS already cached and causing stale entries.
		printerIDPrefix+slugify(descriptor.Name),
		fmt.Sprintf("%s (%s)", descriptor.Name, host),
		"",
		!descriptor.IsVirtual,
	)
	printer.DriverName = descriptor.DriverName
	printer.IsRemote = descriptor.IsRemote
	printer.IsDefault = descriptor.IsDefault
	printer.SetDetectedVirtual(descriptor.IsVirtual, descriptor.VirtualReason)
	if color, known := colorSupport(backend, descriptor.Name); known {
		printer.ColorCapable = &color
	}
	if duplex, known := duplexSupport(backend, descriptor.Name); known {
		printer.DuplexCapable = &duplex
	}
	return printer
}

// colorSupport asks backends that can tell whether a printer prints in
// color; known is false when the backend cannot say.
func colorSupport(backend winspool.Backend, name string) (color, known bool) {
	provider, ok := backend.(interface{ ColorSupported(string) (bool, bool) })
	if !ok {
		return false, false
	}
	return provider.ColorSupported(name)
}

// duplexSupport asks backends that can probe duplex whether a printer
// accepts two-sided output; known is false when the backend cannot say.
func duplexSupport(backend winspool.Backend, name string) (duplex, known bool) {
	provider, ok := backend.(interface{ DuplexSupported(string) bool })
	if !ok {
		return false, false
	}
	return provider.DuplexSupported(name), true
}

// discoverPrinters enumerates the spooler, overlays the saved choices and
// orders the result by the effective virtual state.
func discoverPrinters(backend winspool.Backend, host string, saved []core.PrinterEntry) []core.PrinterInfo {
	printers := applySavedPrinters(seedPrinters(backend, host), saved)
	orderPrinters(printers)
	return printers
}

// orderPrinters puts the default printer first, then physical printers,
// then virtual ones, by the effective verdict (so a user override moves a
// printer between the groups). The sort is stable: the spooler's name
// order survives inside each group.
func orderPrinters(printers []core.PrinterInfo) {
	sort.SliceStable(printers, func(i, j int) bool {
		a, b := printers[i], printers[j]
		if a.IsDefault != b.IsDefault {
			return a.IsDefault
		}
		return !a.IsVirtual && b.IsVirtual
	})
}

// slugify maps a Windows printer name to a DNS-safe id fragment.
func slugify(name string) string {
	var sb strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			sb.WriteRune(r)
			dash = false
		} else if !dash {
			sb.WriteRune('-')
			dash = true
		}
	}
	slug := strings.Trim(sb.String(), "-")
	if slug == "" {
		return "printer"
	}
	return slug
}

// applySavedPrinters overlays the persisted per-printer customisation onto
// freshly enumerated printers, matched by Windows name (case-insensitive:
// Windows printer names are). Saved printers that no longer exist are
// dropped; new ones keep the seeded defaults.
func applySavedPrinters(printers []core.PrinterInfo, saved []core.PrinterEntry) []core.PrinterInfo {
	byName := make(map[string]core.PrinterEntry, len(saved))
	for _, entry := range saved {
		byName[windowsNameKey(entry.WindowsName)] = entry
	}
	for i := range printers {
		if entry, ok := byName[windowsNameKey(printers[i].WindowsName)]; ok {
			overlaySaved(&printers[i], entry)
		}
	}
	return printers
}

// windowsNameKey is the case-insensitive lookup key for a Windows name.
func windowsNameKey(name string) string { return strings.ToLower(name) }

// overlaySaved applies one saved entry's customisation to a printer.
func overlaySaved(printer *core.PrinterInfo, entry core.PrinterEntry) {
	if entry.AirPrintName != "" {
		printer.AirPrintName = entry.AirPrintName
	}
	printer.Location = entry.Location
	printer.Enabled = entry.Enabled
	printer.SetVirtualOverride(entry.VirtualOverride)
}
