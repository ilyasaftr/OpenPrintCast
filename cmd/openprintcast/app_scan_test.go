package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
	"github.com/ilyasaftr/OpenPrintCast/internal/winspool"
)

// fakeScanner answers Scan with a fixed list and counts its calls.
type fakeScanner struct {
	printers []core.PrinterInfo
	err      error
	panics   bool
	calls    int
}

func (f *fakeScanner) Scan() ([]core.PrinterInfo, error) {
	f.calls++
	if f.panics {
		panic("spooler fault")
	}
	return append([]core.PrinterInfo(nil), f.printers...), f.err
}

// heldScan is a scanRunner that keeps the scan pending until the test
// finishes it, so the busy state can be examined without goroutines.
type heldScan struct{ work, done func() }

func (h *heldScan) run(work, done func()) { h.work, h.done = work, done }

func (h *heldScan) pending() bool { return h.work != nil }

// finish runs the held scan and its hop back to the UI thread.
func (h *heldScan) finish() {
	work, done := h.work, h.done
	h.work, h.done = nil, nil
	work()
	done()
}

// scanTestApp is the app with its two printers (EPSON shared, the PDF
// printer virtual) wired to a scripted scanner and a held runner.
func scanTestApp(t *testing.T, scanned ...core.PrinterInfo) (*bridgeApp, *fakeScanner, *heldScan) {
	t.Helper()
	app := newTestApp(t)
	scanner := &fakeScanner{printers: scanned}
	held := &heldScan{}
	app.scanner, app.runScan = scanner, held.run
	return app, scanner, held
}

func listed(names ...string) []core.PrinterInfo {
	printers := make([]core.PrinterInfo, len(names))
	for i, name := range names {
		printers[i] = core.NewPrinterInfo(printerIDPrefix+slugify(name), name, name+" (PC)", "", true)
		if name == pdf {
			printers[i].SetDetectedVirtual(true, "port PORTPROMPT:")
		}
	}
	return printers
}

func TestApplyScanCountsWhatChanged(t *testing.T) {
	tests := []struct {
		name    string
		scanned []core.PrinterInfo
		want    scanOutcome
	}{
		{"unchanged", listed(epson, pdf), scanOutcome{total: 2}},
		{"one added", listed(epson, pdf, "Brother HL"), scanOutcome{added: 1, total: 3}},
		{"one removed", listed(epson), scanOutcome{removed: 1, total: 1}},
		{"swapped", listed(epson, "Brother HL"), scanOutcome{added: 1, removed: 1, total: 2}},
		{"same name in another case", listed("epson l3210 series", pdf), scanOutcome{total: 2}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := newTestApp(t)
			if got := app.applyScan(tc.scanned, nil); got != tc.want {
				t.Fatalf("outcome = %+v, want %+v", got, tc.want)
			}
			if len(app.printers) != len(tc.scanned) {
				t.Fatalf("list has %d printers, want %d", len(app.printers), len(tc.scanned))
			}
		})
	}
}

func TestApplyScanCountsOnlyPrintersThePageLists(t *testing.T) {
	app := newTestApp(t)
	app.cfg.ShowVirtualPrinters = false
	app.printers[1].Enabled = false // the PDF printer is hidden
	scanned := listed(epson, pdf, "Hidden Virtual", "Brother HL")
	for _, i := range []int{1, 2} {
		scanned[i].SetDetectedVirtual(true, "")
		scanned[i].Enabled = false
	}
	got := app.applyScan(scanned, nil)
	if got.added != 1 || got.removed != 0 || got.total != 2 {
		t.Fatalf("outcome = %+v; only Brother HL is new on the page, the new virtual printer is hidden", got)
	}
}

func TestApplyScanKeepsSavedChoices(t *testing.T) {
	app := newTestApp(t)
	app.cfg.Printers = []core.PrinterEntry{{WindowsName: epson, AirPrintName: "Front Desk", Location: "Lobby", Enabled: false}}
	app.applyScan(listed(epson, pdf), nil)
	p := printerByName(t, app.printers, epson)
	if p.AirPrintName != "Front Desk" || p.Location != "Lobby" || p.Enabled {
		t.Fatalf("saved choices lost by the scan: %+v", p)
	}
}

func TestApplyScanFailureKeepsTheListAndTheSavedConfig(t *testing.T) {
	app := newTestApp(t)
	app.persistPrinters()
	before := append([]core.PrinterEntry(nil), app.cfg.Printers...)
	got := app.applyScan(nil, errors.New("spooler stopped"))
	if !got.failed || got.total != 2 || got.message() != scanFailedText {
		t.Fatalf("outcome = %+v", got)
	}
	if len(app.printers) != 2 || len(app.cfg.Printers) != len(before) {
		t.Fatalf("a failed scan changed the list: %d printers, %d saved", len(app.printers), len(app.cfg.Printers))
	}
}

func TestApplyScanBelievesASecondEmptyAnswer(t *testing.T) {
	app := newTestApp(t)
	if got := app.applyScan(nil, nil); !got.failed || len(app.printers) != 2 {
		t.Fatalf("the first empty answer was believed: %+v, %d printers", got, len(app.printers))
	}
	got := app.applyScan(nil, nil)
	if got.failed || got.removed != 2 || len(app.printers) != 0 || got.message() != "2 printers removed" {
		t.Fatalf("the second empty answer was not believed: %+v, %d printers", got, len(app.printers))
	}
}

func TestApplyScanNonEmptyAnswerResetsTheEmptyMemory(t *testing.T) {
	app := newTestApp(t)
	app.applyScan(nil, nil)
	app.applyScan(listed(epson, pdf), nil)
	if got := app.applyScan(nil, nil); !got.failed || len(app.printers) != 2 {
		t.Fatalf("an empty answer after a good one was believed: %+v", got)
	}
}

func TestApplyScanOnAnEmptyAppReportsNoPrinters(t *testing.T) {
	app := newTestApp(t)
	app.printers = nil
	if got := app.applyScan(nil, nil); got.failed || got.message() != scanNoneText {
		t.Fatalf("outcome = %+v", got)
	}
}

func TestStartScanAcknowledgesThenAppliesOnTheUIThread(t *testing.T) {
	app, scanner, held := scanTestApp(t, listed(epson, pdf, "Brother HL")...)
	if !app.startScan(scanEpoch) || !app.scanState().Busy() {
		t.Fatal("startScan did not enter the busy state")
	}
	if scanner.calls != 0 || len(app.printers) != 2 {
		t.Fatal("the scan ran or applied before the runner did")
	}
	held.finish()
	if scanner.calls != 1 || len(app.printers) != 3 {
		t.Fatalf("scan ran %d times and left %d printers", scanner.calls, len(app.printers))
	}
	app.scanState().Advance(scanEpoch.Add(scanMinBusy))
	if got := app.scanState().Status().Text; got != "1 new printer added" {
		t.Fatalf("result = %q", got)
	}
}

func TestStartScanIgnoresRepeatedRequests(t *testing.T) {
	app, scanner, held := scanTestApp(t, listed(epson, pdf)...)
	app.startScan(scanEpoch)
	for i := 0; i < 5; i++ {
		if app.startScan(scanEpoch.Add(time.Duration(i) * time.Millisecond)) {
			t.Fatal("a scan was started while one is running")
		}
	}
	held.finish()
	if scanner.calls != 1 {
		t.Fatalf("scanner ran %d times, want 1", scanner.calls)
	}
	if held.pending() {
		t.Fatal("a second scan is waiting")
	}
}

func TestStartScanTurnsAScannerPanicIntoAFailure(t *testing.T) {
	app, scanner, held := scanTestApp(t)
	scanner.panics = true
	app.startScan(scanEpoch)
	held.finish()
	app.scanState().Advance(scanEpoch.Add(scanMinBusy))
	if st := app.scanState().Status(); !st.Failed || st.Text != scanFailedText || len(app.printers) != 2 {
		t.Fatalf("status = %+v, %d printers", st, len(app.printers))
	}
}

func TestScanPicksUpChoicesMadeWhileItRan(t *testing.T) {
	app, _, held := scanTestApp(t, listed(epson, pdf)...)
	app.startScan(scanEpoch)
	app.printers[0].Enabled = false // the user flips the switch mid-scan
	app.printersChanged()
	held.finish()
	if printerByName(t, app.printers, epson).Enabled {
		t.Fatal("the scan put back a sharing choice the user had just changed")
	}
}

func TestFinishedScanNudgesTheStatusMonitor(t *testing.T) {
	app, _, held := scanTestApp(t, listed(epson, pdf)...)
	app.printerStatus = newStatusMonitor(newFakeSource(nil), nil)
	app.startScan(scanEpoch)
	select {
	case <-app.printerStatus.wake:
		t.Fatal("the monitor was nudged before the scan finished")
	default:
	}
	held.finish()
	select {
	case <-app.printerStatus.wake:
	default:
		t.Fatal("the monitor was not nudged after the scan")
	}
}

func TestSpoolerScannerNeedsABackend(t *testing.T) {
	if _, err := (spoolerScanner{}).Scan(); !errors.Is(err, errNoSpooler) {
		t.Fatalf("err = %v, want errNoSpooler", err)
	}
	printers, err := (spoolerScanner{backend: realWorldBackend(), host: "PC"}).Scan()
	if err != nil || len(printers) == 0 {
		t.Fatalf("scan = %d printers, %v", len(printers), err)
	}
}

func TestRescanPrintersAppliesOnTheCallingThread(t *testing.T) {
	app, _, _ := scanTestApp(t, listed(epson, pdf, "Brother HL")...)
	if got := app.rescanPrinters(); got.added != 1 || len(app.printers) != 3 {
		t.Fatalf("outcome = %+v, %d printers", got, len(app.printers))
	}
}

// TestNudgeMakesTheMonitorPollAtOnce checks the status refresh a scan
// asks for: with a poll interval of an hour, a nudge alone brings a
// second poll, and a nil monitor ignores it.
func TestNudgeMakesTheMonitorPollAtOnce(t *testing.T) {
	src := newFakeSource(map[string]winspool.PrinterStatus{"A": winspool.PrinterStatusReady})
	notified := make(chan struct{}, 4)
	m := newStatusMonitor(src, func() { notified <- struct{}{} })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		m.Run(ctx, func() []string { return []string{"A"} }, time.Hour)
		close(done)
	}()
	waitFor(t, notified, "the first poll")
	src.set("A", winspool.PrinterStatusPaused)
	m.Nudge()
	m.Nudge() // merges with the first
	waitFor(t, notified, "the poll a nudge asks for")
	cancel()
	<-done
	var none *statusMonitor
	none.Nudge()
}

// rescanPrinters scans and applies on the calling thread, as the Scan Again
// button does through startScan without holding the UI thread.
func (a *bridgeApp) rescanPrinters() scanOutcome {
	printers, err := safeScan(a.printerScanner())
	return a.applyScan(printers, err)
}
