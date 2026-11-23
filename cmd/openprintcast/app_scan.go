package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/egoist/mygo"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
	"github.com/ilyasaftr/OpenPrintCast/internal/winspool"
)

// errNoSpooler is the scan failure when the app has no spooler to ask.
var errNoSpooler = errors.New("no spooler backend")

// printerScanner reads the printers Windows lists right now, as freshly
// seeded records (saved names and choices are applied later, on the UI
// thread). It may be slow, so it runs off the UI thread.
type printerScanner interface {
	Scan() ([]core.PrinterInfo, error)
}

// spoolerScanner is the production printerScanner over the spooler backend.
type spoolerScanner struct {
	backend winspool.Backend
	host    string
}

// Scan enumerates the spooler.
func (s spoolerScanner) Scan() ([]core.PrinterInfo, error) {
	if s.backend == nil {
		return nil, errNoSpooler
	}
	return seedPrinters(s.backend, s.host), nil
}

// scanRunner runs work off the UI thread and then done on it.
type scanRunner func(work, done func())

// runOffUIThread is the production scanRunner: work on a goroutine, done
// back on the main thread, where the UI-owned state may be touched.
func runOffUIThread(work, done func()) {
	go func() {
		work()
		mygo.RunOnMain(done)
	}()
}

// scanState is the app's scan state machine, created on first use.
func (a *bridgeApp) scanState() *scanState {
	if a.scans == nil {
		a.scans = newScanState()
	}
	return a.scans
}

// printerScanner is the scanner in use: the injected one, else the spooler.
func (a *bridgeApp) printerScanner() printerScanner {
	if a.scanner != nil {
		return a.scanner
	}
	return spoolerScanner{backend: a.backend, host: a.host}
}

// scanRunner is the runner in use: the injected one, else a goroutine.
func (a *bridgeApp) scanRunner() scanRunner {
	if a.runScan != nil {
		return a.runScan
	}
	return runOffUIThread
}

// startScan is the entry point of Scan Again, on the UI thread at now. It
// acknowledges at once by entering the busy state, reads the spooler off
// the UI thread and applies the result back on it. It reports false when a
// scan is already running, in which case nothing happens.
func (a *bridgeApp) startScan(now time.Time) bool {
	if !a.scanState().Begin(now) {
		return false
	}
	a.requestNetCheck(now, false)
	var (
		printers []core.PrinterInfo
		err      error
	)
	scanner := a.printerScanner()
	a.scanRunner()(func() { printers, err = safeScan(scanner) }, func() { a.finishScan(printers, err) })
	return true
}

// safeScan runs the scanner and turns a panic into a failure, so a fault
// in the spooler enumeration cannot take the app down from its goroutine.
func safeScan(scanner printerScanner) (printers []core.PrinterInfo, err error) {
	defer func() {
		if r := recover(); r != nil {
			printers, err = nil, fmt.Errorf("printer scan panicked: %v", r)
		}
	}()
	return scanner.Scan()
}

// finishScan applies a scan's result on the UI thread, records the outcome
// for the page, and asks the status monitor to poll the new list now, so a
// new printer does not read "Status unknown" until the next poll; the
// compatibility monitor checks again too, as Scan Again is the next step its
// warning names.
func (a *bridgeApp) finishScan(printers []core.PrinterInfo, err error) {
	outcome := a.applyScan(printers, err)
	a.scanState().Finish(outcome)
	a.printerStatus.Nudge()
	a.printerCompat.Nudge()
	a.invalidateWindow()
}

// applyScan replaces the printer list with a scan's result and reports what
// changed. A failed scan, or a first empty answer after printers were
// listed (the spooler reports its own failures as an empty list), leaves
// the list and the saved choices alone; a second empty answer in a row is
// believed.
func (a *bridgeApp) applyScan(scanned []core.PrinterInfo, err error) scanOutcome {
	if a.rejectScan(scanned, err) {
		return scanOutcome{failed: true, total: len(a.visibleNames(a.printers))}
	}
	next := applySavedPrinters(scanned, a.cfg.Printers)
	orderPrinters(next)
	before, after := a.visibleNames(a.printers), a.visibleNames(next)
	a.printers = next
	a.printersChanged()
	return diffNames(before, after)
}

// rejectScan reports whether a scan result must not replace the list, and
// remembers whether the scan came back empty.
func (a *bridgeApp) rejectScan(scanned []core.PrinterInfo, err error) bool {
	if err != nil {
		componentLogger(componentApp).Warn("printer scan failed", "err", err)
		return true
	}
	empty := len(scanned) == 0
	suspicious := empty && len(a.printers) > 0 && !a.emptyScanSeen
	a.emptyScanSeen = empty
	if suspicious {
		componentLogger(componentApp).Warn("printer scan came back empty; keeping the current list")
	}
	return suspicious
}

// visibleNames is the set of printers the Printers page would list,
// keyed by case-insensitive Windows name.
func (a *bridgeApp) visibleNames(printers []core.PrinterInfo) map[string]struct{} {
	names := make(map[string]struct{}, len(printers))
	for _, p := range printers {
		if a.visiblePrinter(p) {
			names[windowsNameKey(p.WindowsName)] = struct{}{}
		}
	}
	return names
}

// diffNames counts the names that appeared and that went away.
func diffNames(before, after map[string]struct{}) scanOutcome {
	outcome := scanOutcome{total: len(after)}
	for name := range after {
		if _, ok := before[name]; !ok {
			outcome.added++
		}
	}
	for name := range before {
		if _, ok := after[name]; !ok {
			outcome.removed++
		}
	}
	return outcome
}
