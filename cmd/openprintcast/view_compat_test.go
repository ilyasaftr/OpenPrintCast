package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// withCompat gives the app a monitor already holding the given results for
// printer IDs, as if the background check had run.
func withCompat(app *bridgeApp, gapsByID map[string][]string, unknown ...string) {
	src := newFakeCompat()
	for id, gaps := range gapsByID {
		src.set(id, gaps, true)
	}
	for _, id := range unknown {
		src.set(id, nil, false)
	}
	app.printerCompat = newCompatibilityMonitor(src, nil, nil)
	app.printerCompat.refresh(app.printers)
}

func epsonLabel(line string) string { return compatRowLabelHead + epson + ": " + line }

func TestCompatibilityTextsAreExact(t *testing.T) {
	const next = " Update or reinstall the driver, or try a different driver for this printer, then choose Scan Again."
	const lead = "AirPrint needs the printer driver to report paper sizes, page orientation and print resolution. This driver didn't report: "
	for _, tc := range []struct {
		gaps []string
		want string
	}{
		{[]string{"paper-sizes"}, lead + "paper sizes." + next},
		{[]string{"resolution", "orientation"}, lead + "page orientation and print resolution." + next},
		{[]string{"orientation", "paper-sizes"}, lead + "paper sizes and page orientation." + next},
		{[]string{"resolution", "orientation", "paper-sizes"}, lead + "paper sizes, page orientation and print resolution." + next},
	} {
		if got := compatibilityDetail(tc.gaps); got != tc.want {
			t.Errorf("compatibilityDetail(%v) =\n%q\nwant\n%q", tc.gaps, got, tc.want)
		}
	}
	if compatRowLine != "iPhone, iPad and Mac can't print here" {
		t.Errorf("row line = %q", compatRowLine)
	}
}

func TestJoinNaturally(t *testing.T) {
	for in, want := range map[string]string{
		"":      "",
		"a":     "a",
		"a|b":   "a and b",
		"a|b|c": "a, b and c",
	} {
		var items []string
		if in != "" {
			items = splitBar(in)
		}
		if got := joinNaturally(items); got != want {
			t.Errorf("joinNaturally(%q) = %q, want %q", in, got, want)
		}
	}
}

func splitBar(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == '|' {
			out, cur = append(out, cur), ""
			continue
		}
		cur += string(r)
	}
	return append(out, cur)
}

func TestMissingRequirementsIgnoresUnknownKeys(t *testing.T) {
	got := missingRequirements([]string{"resolution", "bogus", "paper-sizes"})
	if len(got) != 2 || got[0].title != "Paper sizes" || got[1].title != "Print resolution" {
		t.Fatalf("missing = %+v", got)
	}
}

func TestPrinterRowWarnsOnlyForSharedPrintersWithGaps(t *testing.T) {
	all := []string{"orientation", "paper-sizes", "resolution"}
	for _, tc := range []struct {
		name     string
		setup    func(*bridgeApp)
		wantLine int
		detail   string
	}{
		{"no monitor", func(*bridgeApp) {}, 0, ""},
		{"no gaps", func(a *bridgeApp) { withCompat(a, map[string][]string{"printer-epson": nil}) }, 0, ""},
		{"one gap", func(a *bridgeApp) { withCompat(a, map[string][]string{"printer-epson": {"paper-sizes"}}) }, 1, compatibilityDetail([]string{"paper-sizes"})},
		{"two gaps", func(a *bridgeApp) {
			withCompat(a, map[string][]string{"printer-epson": {"orientation", "paper-sizes"}})
		}, 1, compatibilityDetail([]string{"paper-sizes", "orientation"})},
		{"three gaps", func(a *bridgeApp) { withCompat(a, map[string][]string{"printer-epson": all}) }, 1, compatibilityDetail(all)},
		{"unknown", func(a *bridgeApp) { withCompat(a, nil, "printer-epson") }, 0, ""},
		{"not shared", func(a *bridgeApp) {
			a.printers[0].Enabled = false
			withCompat(a, map[string][]string{"printer-epson": all})
		}, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := newTestApp(t)
			tc.setup(app)
			tt := ui.NewTester(app.view, 860, 620)
			if got := countTexts(tt, compatRowLine); got != tc.wantLine {
				t.Fatalf("%d warning lines, want %d; texts %q", got, tc.wantLine, tt.Texts())
			}
			a, ok := tt.Access(epsonLabel(compatRowLine))
			if ok != (tc.wantLine == 1) {
				t.Fatalf("accessible warning present = %t", ok)
			}
			if ok && a.Description != tc.detail {
				t.Fatalf("description = %q, want %q", a.Description, tc.detail)
			}
		})
	}
}

func TestFolderRowNeverWarns(t *testing.T) {
	app := newTestApp(t)
	app.cfg.FolderEnabled = true
	withCompat(app, map[string][]string{"printer-epson": nil, "printer-pdf": nil})
	tt := ui.NewTester(app.view, 860, 620)
	if countTexts(tt, compatRowLine) != 0 {
		t.Fatal("folder destination must not warn")
	}
	folder := core.PrinterInfo{ID: "f", WindowsName: core.FolderPrinterName, Enabled: true}
	if state, _ := app.compatibilityOf(&folder); state != compatReady {
		t.Fatalf("folder state = %v", state)
	}
	if app.compatibilityNote(nil, &folder) != nil {
		t.Fatal("folder row must have no note")
	}
}

func TestSheetCompatibilitySection(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(*bridgeApp)
		want  []string
		not   []string
	}{
		{"pending shared", func(*bridgeApp) {}, []string{compatSheetTitle, compatChecking}, []string{compatNextStep}},
		{"unknown", func(a *bridgeApp) { withCompat(a, nil, "printer-epson") }, []string{compatNotChecked}, []string{compatChecking, compatNextStep}},
		{"pending not shared", func(a *bridgeApp) { a.printers[0].Enabled = false }, []string{compatNotChecked}, []string{compatChecking}},
		{"ready", func(a *bridgeApp) { withCompat(a, map[string][]string{"printer-epson": nil}) },
			[]string{"Paper sizes: reported", "Page orientation: reported", "Print resolution: reported"},
			[]string{compatNextStep, "Paper sizes: not reported"}},
		{"gaps", func(a *bridgeApp) {
			withCompat(a, map[string][]string{"printer-epson": {"orientation", "resolution"}})
		}, []string{"Paper sizes: reported", "Page orientation: not reported", "Print resolution: not reported", compatNextStep}, nil},
		{"gaps not shared", func(a *bridgeApp) {
			withCompat(a, map[string][]string{"printer-epson": {"paper-sizes"}})
			a.printers[0].Enabled = false
		}, []string{"Paper sizes: not reported", compatNextStep}, []string{compatRowLine}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := newTestApp(t)
			tc.setup(app)
			tt := ui.NewTester(app.view, 860, 620)
			if err := tt.Click("Name and location for " + epson); err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.want {
				if !tt.HasText(want) {
					t.Errorf("sheet lacks %q; texts %q", want, tt.Texts())
				}
			}
			for _, not := range tc.not {
				if countTexts(tt, not) != 0 {
					t.Errorf("sheet shows %q", not)
				}
			}
		})
	}
}

// rowControlsClear checks that the warning line of the EPSON row sits
// inside the row's text column: left of the status cell, info button and
// switch, and not above or below them by more than the row itself.
func rowControlsClear(t *testing.T, tt *ui.Tester, label string) {
	t.Helper()
	warn, ok := tt.Access(label)
	if !ok {
		t.Fatalf("warning %q missing", label)
	}
	for _, name := range []string{"Share " + epson, "Name and location for " + epson, "Status of " + epson} {
		other, ok := findByPrefix(tt, name)
		if !ok {
			t.Fatalf("%q missing", name)
		}
		if warn.Bounds.X+warn.Bounds.W > other.Bounds.X+0.5 {
			t.Errorf("warning %v reaches %q at %v", warn.Bounds, name, other.Bounds)
		}
	}
}

func findByPrefix(tt *ui.Tester, prefix string) (ui.AccessInfo, bool) {
	for _, a := range tt.AccessAll() {
		if len(a.Name) >= len(prefix) && a.Name[:len(prefix)] == prefix {
			return a, true
		}
	}
	return ui.AccessInfo{}, false
}

func TestWarningRowKeepsControlsOnOneEdgeAndClear(t *testing.T) {
	gaps := []string{"orientation", "paper-sizes", "resolution"}
	for _, size := range [][2]int{{860, 620}, {640, 460}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			plain := newTestApp(t)
			ptt := ui.NewTester(plain.view, size[0], size[1])
			app := newTestApp(t)
			withCompat(app, map[string][]string{"printer-epson": gaps})
			tt := ui.NewTester(app.view, size[0], size[1])
			rowControlsClear(t, tt, epsonLabel(compatRowLine))
			for _, name := range []string{"Share ", "Name and location for "} {
				a, _ := findByPrefix(ptt, name+epson)
				b, _ := findByPrefix(tt, name+epson)
				if a.Bounds.X != b.Bounds.X || a.Bounds.W != b.Bounds.W {
					t.Errorf("%s moved horizontally: %v -> %v", name, a.Bounds, b.Bounds)
				}
			}
			sw, _ := findByPrefix(tt, "Share "+epson)
			title, _ := tt.Find(epson)
			warn, _ := tt.Access(epsonLabel(compatRowLine))
			mid := sw.Bounds.Y + sw.Bounds.H/2
			if mid < title.Y || mid > warn.Bounds.Y+warn.Bounds.H {
				t.Errorf("switch %v lies outside the row spanning %v to %v", sw.Bounds, title, warn.Bounds)
			}
		})
	}
}

func TestUnaffectedRowsKeepTheirLayout(t *testing.T) {
	baseline := newTestApp(t)
	btt := ui.NewTester(baseline.view, 860, 620)
	for _, setup := range []func(*bridgeApp){
		func(a *bridgeApp) { withCompat(a, map[string][]string{"printer-epson": nil, "printer-pdf": nil}) },
		func(a *bridgeApp) { withCompat(a, nil, "printer-epson", "printer-pdf") },
		func(a *bridgeApp) { withCompat(a, map[string][]string{"printer-pdf": {"orientation"}}) }, // not shared
	} {
		app := newTestApp(t)
		setup(app)
		tt := ui.NewTester(app.view, 860, 620)
		for _, name := range []string{"Share " + epson, "Share " + pdf, "Name and location for " + pdf} {
			want, _ := btt.Find(name)
			if got, _ := tt.Find(name); got != want {
				t.Errorf("%s moved: %v -> %v", name, want, got)
			}
		}
	}
}

func TestCompatibilitySnapshots(t *testing.T) {
	app := newTestApp(t)
	withCompat(app, map[string][]string{"printer-epson": {"orientation", "paper-sizes", "resolution"}})
	tt := ui.NewTester(app.view, 860, 620)
	snapshot(t, tt, "compat-row")
	tt.SetDark(true)
	snapshot(t, tt, "compat-row-dark")
	tt.SetDark(false)
	tt.SetSize(640, 460)
	snapshot(t, tt, "compat-row-min")
	tt.SetDark(true)
	snapshot(t, tt, "compat-row-min-dark")
	tt.SetDark(false)
	tt.SetSize(860, 620)
	if err := tt.Click("Name and location for " + epson); err != nil {
		t.Fatal(err)
	}
	snapshot(t, tt, "compat-sheet")
	tt.SetDark(true)
	snapshot(t, tt, "compat-sheet-dark")
	tt.SetDark(false)
	tt.SetSize(640, 460)
	snapshot(t, tt, "compat-sheet-min")
	tt.SetDark(true)
	snapshot(t, tt, "compat-sheet-min-dark")
}

func TestSheetWithWarningStaysInsideSmallestWindow(t *testing.T) {
	app := newTestApp(t)
	withCompat(app, map[string][]string{"printer-epson": {"orientation", "paper-sizes", "resolution"}})
	tt := ui.NewTester(app.view, 640, 460)
	if err := tt.Click("Name and location for " + epson); err != nil {
		t.Fatal(err)
	}
	done, ok := tt.Access("Done")
	if !ok || done.Bounds.Y+done.Bounds.H > 460 {
		t.Fatalf("Done at %v (found %t) leaves the 460 high window", done.Bounds, ok)
	}
}
