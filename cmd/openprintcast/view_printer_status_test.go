package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/winspool"
)

// withStatuses gives the app a monitor already holding the given
// conditions, as if the background poll had run.
func withStatuses(app *bridgeApp, conds map[string]printerCondition) {
	src := fakeStatusSource(conds)
	app.printerStatus = newStatusMonitor(src, nil)
	app.printerStatus.refresh(windowsNames(app.printers))
}

// fakeStatusSource serves fixed conditions through the optional reason
// interface, like the real backend.
type fakeStatusSource map[string]printerCondition

func (f fakeStatusSource) QueryStatus(name string) winspool.PrinterStatus { return f[name].status }

func (f fakeStatusSource) IPPStatus(name string) (int, []string, bool) {
	return 0, f[name].reasons, true
}

const (
	epson = "EPSON L3210 Series"
	pdf   = "Microsoft Print to PDF"
)

func TestPrinterRowsShowTheirStatus(t *testing.T) {
	app := newTestApp(t)
	tt := ui.NewTester(app.view, 860, 620)
	if got := countTexts(tt, "Status unknown"); got != 2 {
		t.Fatalf("printers without a monitor show %d unknown labels, want 2", got)
	}

	withStatuses(app, map[string]printerCondition{
		epson: {status: winspool.PrinterStatusOffline, reasons: []string{"media-empty"}},
		pdf:   {status: winspool.PrinterStatusReady},
	})
	tt = ui.NewTester(app.view, 860, 620)
	for _, want := range []string{"Paper out", "Ready"} {
		if countTexts(tt, want) != 1 {
			t.Errorf("status %q missing; texts: %q", want, tt.Texts())
		}
	}
	if a, ok := tt.Access("Status of " + epson + ": Paper out"); !ok || a.Description == "" {
		t.Errorf("status has no accessible label with a tooltip: %+v, %t", a, ok)
	}
	snapshot(t, tt, "printers-status")
	tt.SetDark(true)
	snapshot(t, tt, "printers-status-dark")
	tt.SetDark(false)
	tt.SetSize(640, 460)
	snapshot(t, tt, "printers-status-min")
}

func countTexts(tt *ui.Tester, want string) int {
	n := 0
	for _, text := range tt.Texts() {
		if text == want {
			n++
		}
	}
	return n
}

func TestFolderDestinationHasNoStatus(t *testing.T) {
	app := newTestApp(t)
	app.printers = app.printers[:1]
	tt := ui.NewTester(app.view, 860, 620)
	if got := countTexts(tt, "Status unknown"); got != 1 {
		t.Fatalf("%d status labels with one printer and the folder row, want 1", got)
	}
}

func TestPrinterStatusKeepsControlsOnOneEdge(t *testing.T) {
	states := []printerCondition{
		{},
		{status: winspool.PrinterStatusReady},
		{status: winspool.PrinterStatusPrinting},
		{status: winspool.PrinterStatusPaused},
		{status: winspool.PrinterStatusOffline},
		{status: winspool.PrinterStatusOffline, reasons: []string{"media-empty"}},
		{status: winspool.PrinterStatusReady, reasons: []string{"marker-supply-low-warning"}},
		{status: winspool.PrinterStatusOffline, reasons: []string{"door-open", "other"}},
	}
	for _, size := range [][2]int{{860, 620}, {640, 460}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			var edge, infoEdge float32
			for i, cond := range states {
				app := newTestApp(t)
				withStatuses(app, map[string]printerCondition{epson: cond, pdf: states[(i+1)%len(states)]})
				tt := ui.NewTester(app.view, size[0], size[1])
				for _, name := range []string{epson, pdf} {
					sw, ok := tt.Find("Share " + name)
					info, ok2 := tt.Find("Name and location for " + name)
					if !ok || !ok2 {
						t.Fatalf("state %d: controls of %s missing", i, name)
					}
					if i == 0 && name == epson {
						edge, infoEdge = sw.X, info.X
					}
					if sw.X != edge || info.X != infoEdge {
						t.Fatalf("state %d %s: switch at %v, info at %v; want %v and %v", i, name, sw.X, info.X, edge, infoEdge)
					}
				}
			}
		})
	}
}

func TestCompactWindowShowsStatusSymbolWithAccessibleLabel(t *testing.T) {
	app := newTestApp(t)
	withStatuses(app, map[string]printerCondition{
		epson: {status: winspool.PrinterStatusOffline},
		pdf:   {status: winspool.PrinterStatusReady},
	})
	tt := ui.NewTester(app.view, 640, 460)
	if countTexts(tt, "Offline") != 0 || countTexts(tt, "Ready") != 0 {
		t.Fatalf("compact window should show the symbol only; texts: %q", tt.Texts())
	}
	a, ok := tt.Access("Status of " + epson + ": Offline")
	if !ok || a.Bounds.W <= 0 || a.Description != "Offline. Check that the printer is turned on, connected and has paper and toner." {
		t.Fatalf("symbol lacks an accessible label or tooltip: %+v, %t", a, ok)
	}
	info, _ := tt.Find("Name and location for " + epson)
	if a.Bounds.X+a.Bounds.W > info.X {
		t.Fatalf("status %v overlaps the info button %v", a.Bounds, info)
	}
}

func TestPrinterStatusLongestLabelsFit(t *testing.T) {
	app := newTestApp(t)
	withStatuses(app, map[string]printerCondition{
		epson: {status: winspool.PrinterStatusReady, reasons: []string{"other"}},
		pdf:   {status: winspool.PrinterStatusPaused},
	})
	tt := ui.NewTester(app.view, 860, 620)
	info, _ := tt.Find("Name and location for " + epson)
	for _, label := range []string{"Needs attention", "Paused"} {
		r, ok := tt.Find(label)
		if !ok || r.X+r.W > info.X {
			t.Errorf("%q at %v does not fit before the info button at %v", label, r, info)
		}
	}
	snapshot(t, tt, "printers-status-attention")
	tt.SetDark(true)
	snapshot(t, tt, "printers-status-attention-dark")
}
