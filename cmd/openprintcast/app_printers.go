package main

import (
	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// sharedPrinterCount counts the printers currently selected for sharing.
func (a *bridgeApp) sharedPrinterCount() int {
	return a.countPrinters(func(p core.PrinterInfo) bool { return p.Enabled }) + a.folderDestinationCount()
}

// sharedListedCount counts the shared printers the Printers list shows,
// which, unlike sharedPrinterCount, leaves out the Print to Folder
// destination that has its own group.
func (a *bridgeApp) sharedListedCount() int {
	return a.countPrinters(func(p core.PrinterInfo) bool { return p.Enabled && a.visiblePrinter(p) })
}

func (a *bridgeApp) destinationCount() int {
	return a.countPrinters(a.visiblePrinter) + a.folderDestinationCount()
}

// countPrinters counts the Windows printers matching keep.
func (a *bridgeApp) countPrinters(keep func(core.PrinterInfo) bool) int {
	n := 0
	for _, p := range a.printers {
		if keep(p) {
			n++
		}
	}
	return n
}

// folderDestinationCount is 1 when Print to Folder is an active
// destination and 0 otherwise.
func (a *bridgeApp) folderDestinationCount() int {
	if _, ok := a.folderPrinter(); ok {
		return 1
	}
	return 0
}

// publishPrinters pushes a snapshot copy to the server thread so the UI
// slice is never shared across goroutines.
func (a *bridgeApp) publishPrinters() {
	snapshot := append([]core.PrinterInfo(nil), a.printers...)
	if folder, ok := a.folderPrinter(); ok {
		snapshot = append(snapshot, folder)
	}
	a.shared.SetPrinters(snapshot)
	a.printerNames.Set(windowsNames(a.printers))
	a.printerList.Set(a.printers)
	a.printerCompat.Nudge()
}

// windowsNames lists the Windows names of the printers.
func windowsNames(printers []core.PrinterInfo) []string {
	names := make([]string, len(printers))
	for i, p := range printers {
		names[i] = p.WindowsName
	}
	return names
}

// printersChanged applies an edit to the printer list everywhere: the
// server thread (which re-advertises), the config file and the tray tip.
func (a *bridgeApp) printersChanged() {
	a.publishPrinters()
	a.persistPrinters()
	a.syncTray()
}

// setPrinterShared turns sharing of the printer with windowsName on or off
// (the tray menu's entry point), then applies it everywhere and redraws.
func (a *bridgeApp) setPrinterShared(windowsName string, shared bool) {
	for i := range a.printers {
		if a.printers[i].WindowsName == windowsName {
			a.printers[i].Enabled = shared
		}
	}
	a.printersChanged()
	a.invalidateWindow()
}
