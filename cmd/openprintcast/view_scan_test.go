package main

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// scanViewApp is a Printers page whose scan finds one new printer.
func scanViewApp(t *testing.T) (*bridgeApp, *fakeScanner, *heldScan) {
	t.Helper()
	return scanTestApp(t, listed(epson, pdf, "Brother HL")...)
}

func scanButtonInfo(t *testing.T, tt *ui.Tester, name string) ui.AccessInfo {
	t.Helper()
	info, ok := tt.AccessByRole(ui.RoleButton, name)
	if !ok {
		t.Fatalf("no button named %q; texts: %q", name, tt.Texts())
	}
	return info
}

func TestScanButtonIdleIsAnAvailableButton(t *testing.T) {
	app, _, _ := scanViewApp(t)
	tt := ui.NewTester(app.view, 860, 620)
	info := scanButtonInfo(t, tt, "Scan Again")
	if info.Disabled || info.Busy || !tt.HasText("Scan Again") || tt.HasText("Scanning…") {
		t.Fatalf("idle button = %+v; texts: %q", info, tt.Texts())
	}
	if info.Description == "" {
		t.Fatal("the idle button has no tooltip")
	}
	snapshot(t, tt, "scan-idle")
}

func TestScanButtonAcknowledgesTheClickAtOnce(t *testing.T) {
	for _, size := range [][2]int{{860, 620}, {640, 460}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			app, scanner, held := scanViewApp(t)
			tt := ui.NewTester(app.view, size[0], size[1])
			idle := scanButtonInfo(t, tt, "Scan Again")
			if err := tt.Click("Scan Again"); err != nil {
				t.Fatal(err)
			}
			busy := scanButtonInfo(t, tt, "Scanning for printers")
			if !busy.Busy || !busy.Disabled {
				t.Fatalf("the button is not busy and unavailable right after the click: %+v", busy)
			}
			if size[0] >= widePrintersWidth && !tt.HasText("Scanning…") {
				t.Fatalf("no Scanning… label; texts: %q", tt.Texts())
			}
			if busy.Bounds != idle.Bounds {
				t.Fatalf("the toolbar shifted: %v -> %v", idle.Bounds, busy.Bounds)
			}
			if !held.pending() || scanner.calls != 0 {
				t.Fatal("the scan did not wait for its runner")
			}
			if !tt.HasText("Scanning for printers…") {
				t.Fatalf("no inline status; texts: %q", tt.Texts())
			}
			snapshot(t, tt, fmt.Sprintf("scan-busy-%d", size[0]))
			tt.SetDark(true)
			snapshot(t, tt, fmt.Sprintf("scan-busy-dark-%d", size[0]))
		})
	}
}

func TestScanButtonKeepsTheKeyboardFocusWhileBusy(t *testing.T) {
	app, _, _ := scanViewApp(t)
	tt := ui.NewTester(app.view, 860, 620)
	if err := tt.Click("Scan Again"); err != nil {
		t.Fatal(err)
	}
	if info := scanButtonInfo(t, tt, "Scanning for printers"); !info.Focusable || !info.Focused {
		t.Fatalf("the busy button lost the focus: %+v", info)
	}
}

func TestRepeatedClicksWhileScanningStartOneScan(t *testing.T) {
	app, scanner, held := scanViewApp(t)
	tt := ui.NewTester(app.view, 860, 620)
	for i := 0; i < 4; i++ {
		_ = tt.Click("Scanning for printers")
		if i == 0 {
			_ = tt.Click("Scan Again")
		}
	}
	tt.Key(0, ui.KeyF5)
	completeScan(tt, held)
	if scanner.calls != 1 {
		t.Fatalf("scanner ran %d times, want 1", scanner.calls)
	}
}

func TestScanResultAppearsOnlyAfterTheMinimumBusyTime(t *testing.T) {
	app, _, held := scanViewApp(t)
	tt := ui.NewTester(app.view, 860, 620)
	if err := tt.Click("Scan Again"); err != nil {
		t.Fatal(err)
	}
	completeScan(tt, held) // the spooler answers in no time
	if !scanButtonInfoBusy(tt) || tt.HasText("1 new printer added") {
		t.Fatalf("a scan that took 0 ms showed its result at once; texts: %q", tt.Texts())
	}
	if !tt.HasText("Brother HL") {
		t.Fatal("the list did not update as soon as the scan had its answer")
	}
	tt.Advance(scanMinBusy - 10*time.Millisecond)
	if !scanButtonInfoBusy(tt) {
		t.Fatal("the busy state ended before the minimum time")
	}
	tt.Advance(10 * time.Millisecond)
	if scanButtonInfoBusy(tt) || !tt.HasText("1 new printer added") {
		t.Fatalf("no result after the minimum time; texts: %q", tt.Texts())
	}
	snapshot(t, tt, "scan-result")
	tt.SetDark(true)
	snapshot(t, tt, "scan-result-dark")
	tt.SetSize(640, 460)
	snapshot(t, tt, "scan-result-dark-640")
	tt.SetDark(false)
	snapshot(t, tt, "scan-result-640")
}

// completeScan finishes the held scan and draws the frame that the app's
// invalidateWindow asks for, as the real window would.
func completeScan(tt *ui.Tester, held *heldScan) {
	held.finish()
	tt.Frame()
}

func scanButtonInfoBusy(tt *ui.Tester) bool {
	info, ok := tt.AccessByRole(ui.RoleButton, "Scanning for printers")
	return ok && info.Busy
}

func TestScanResultFadesButAFailureStays(t *testing.T) {
	app, scanner, held := scanViewApp(t)
	tt := ui.NewTester(app.view, 860, 620)
	_ = tt.Click("Scan Again")
	completeScan(tt, held)
	tt.Advance(scanMinBusy)
	tt.Advance(scanResultVisible)
	if tt.HasText("1 new printer added") {
		t.Fatalf("the result never faded; texts: %q", tt.Texts())
	}

	scanner.err = fmt.Errorf("spooler stopped")
	_ = tt.Click("Scan Again")
	completeScan(tt, held)
	tt.Advance(scanMinBusy)
	tt.Advance(time.Hour)
	if !tt.HasText(scanFailedText) {
		t.Fatalf("the failure faded; texts: %q", tt.Texts())
	}
	snapshot(t, tt, "scan-failed")
	tt.SetDark(true)
	snapshot(t, tt, "scan-failed-dark")
	tt.SetSize(640, 460)
	snapshot(t, tt, "scan-failed-dark-640")
}

func TestScanKeepsSearchTextAndOtherState(t *testing.T) {
	app, _, held := scanViewApp(t)
	tt := ui.NewTester(app.view, 860, 620)
	if err := tt.Click("Find a printer"); err != nil {
		t.Fatal(err)
	}
	tt.Type("brother")
	if !tt.HasText("No printers match your search. Try another name or location.") {
		t.Fatalf("setup: Brother is not listed yet; texts: %q", tt.Texts())
	}
	_ = tt.Click("Scan Again")
	completeScan(tt, held)
	tt.Advance(scanMinBusy)
	if app.printerQuery != "brother" || !tt.HasText("Brother HL") || tt.HasText("EPSON L3210 Series") {
		t.Fatalf("scan lost the search %q; texts: %q", app.printerQuery, tt.Texts())
	}
	if !tt.HasText("1 new printer added") {
		t.Fatalf("no result next to the filtered list; texts: %q", tt.Texts())
	}
}

func TestScanAnnouncesToScreenReaders(t *testing.T) {
	app, _, held := scanViewApp(t)
	tt := ui.NewTester(app.view, 860, 620)
	_ = tt.Click("Scan Again")
	if got := tt.Announcements(); !slices.Equal(got, []string{scanBusyText}) {
		t.Fatalf("announced %q when the scan started", got)
	}
	completeScan(tt, held)
	if got := tt.Announcements(); len(got) != 0 {
		t.Fatalf("announced %q before the result was shown", got)
	}
	tt.Advance(scanMinBusy)
	if got := tt.Announcements(); !slices.Equal(got, []string{"1 new printer added"}) {
		t.Fatalf("announced %q when the result showed", got)
	}
}

func TestScanFailureIsShownAndTheListStays(t *testing.T) {
	app, scanner, held := scanViewApp(t)
	scanner.err = fmt.Errorf("spooler stopped")
	tt := ui.NewTester(app.view, 860, 620)
	_ = tt.Click("Scan Again")
	completeScan(tt, held)
	tt.Advance(scanMinBusy)
	if !tt.HasText(scanFailedText) || !tt.HasText("EPSON L3210 Series") || len(app.printers) != 2 {
		t.Fatalf("failure not shown or the list changed; texts: %q", tt.Texts())
	}
	if got := tt.Announcements(); !slices.Contains(got, scanFailedText) {
		t.Fatalf("failure not announced: %q", got)
	}
}

func TestScanShortcuts(t *testing.T) {
	for name, press := range map[string]func(*ui.Tester){
		"F5":     func(tt *ui.Tester) { tt.Key(0, ui.KeyF5) },
		"Ctrl+R": func(tt *ui.Tester) { tt.Key(ui.Cmd, ui.KeyR) },
	} {
		t.Run(name, func(t *testing.T) {
			app, _, held := scanViewApp(t)
			tt := ui.NewTester(app.view, 860, 620)
			press(tt)
			if !held.pending() || !app.scanState().Busy() {
				t.Fatalf("%s did not start a scan", name)
			}
		})
	}
	app, _, _ := scanViewApp(t)
	tt := ui.NewTester(app.view, 860, 620)
	info := scanButtonInfo(t, tt, "Scan Again")
	if len(info.KeyShortcuts) == 0 {
		t.Fatalf("the button does not announce its shortcuts: %+v", info)
	}
}

func TestCtrlFFocusesTheSearchField(t *testing.T) {
	app, _, _ := scanViewApp(t)
	tt := ui.NewTester(app.view, 860, 620)
	if tt.Focused("Find a printer") {
		t.Fatal("setup: the search field already has the focus")
	}
	tt.Key(ui.Cmd, ui.KeyF)
	if !tt.Focused("Find a printer") {
		t.Fatal("Ctrl+F did not focus the search field")
	}
	tt.Type("epson")
	if app.printerQuery != "epson" {
		t.Fatalf("typing after Ctrl+F gave %q", app.printerQuery)
	}
}

func TestScanTooltipsTellTheState(t *testing.T) {
	app, _, _ := scanViewApp(t)
	tt := ui.NewTester(app.view, 860, 620)
	_ = tt.Click("Scan Again")
	busy := scanButtonInfo(t, tt, "Scanning for printers")
	if busy.Description != scanButtonBusyTip {
		t.Fatalf("busy tooltip = %q", busy.Description)
	}
}

func TestNewPrinterAppearsWithAStatusAfterAScan(t *testing.T) {
	app, _, held := scanViewApp(t)
	app.printerStatus = newStatusMonitor(fakeStatusSource{}, nil)
	tt := ui.NewTester(app.view, 860, 620)
	_ = tt.Click("Scan Again")
	completeScan(tt, held)
	if _, ok := app.printerStatus.Condition("Brother HL"); ok {
		t.Fatal("setup: the monitor already knows the printer")
	}
	select {
	case <-app.printerStatus.wake:
	default:
		t.Fatal("the scan did not ask the monitor to poll")
	}
	if !slices.Contains(windowsNames(app.printers), "Brother HL") {
		t.Fatalf("printers = %v", windowsNames(app.printers))
	}
	_ = core.PrinterInfo{}
}
