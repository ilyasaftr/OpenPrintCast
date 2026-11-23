package main

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

const (
	pdfPrinter     = "Microsoft Print to PDF"
	pdfSwitchLabel = "Treat Microsoft Print to PDF as a virtual printer"
	epsonSwitch    = "Treat EPSON L3210 Series as a virtual printer"
	automaticLabel = "Use Automatic Detection"
)

func virtualSwitch(t *testing.T, tt *ui.Tester, label string) ui.AccessInfo {
	t.Helper()
	info, ok := tt.AccessByRole(ui.RoleSwitch, label)
	if !ok {
		t.Fatalf("switch %q missing; texts: %q", label, tt.Texts())
	}
	return info
}

func savedEntry(t *testing.T, app *bridgeApp, name string) core.PrinterEntry {
	t.Helper()
	saved, err := core.LoadConfig(app.store.Location())
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range saved.Printers {
		if entry.WindowsName == name {
			return entry
		}
	}
	t.Fatalf("no saved entry for %q in %+v", name, saved.Printers)
	return core.PrinterEntry{}
}

func TestVirtualCaptionTexts(t *testing.T) {
	for _, tc := range []struct {
		overridden bool
		reason     string
		want       string
	}{
		{false, "port PORTPROMPT:", "Detected automatically: port PORTPROMPT:."},
		{true, "port PORTPROMPT:", "Set by you. Detected: port PORTPROMPT:."},
		{false, "", "Detected automatically."},
		{true, "", "Set by you."},
	} {
		if got := virtualCaption(tc.overridden, tc.reason); got != tc.want {
			t.Errorf("virtualCaption(%t, %q) = %q, want %q", tc.overridden, tc.reason, got, tc.want)
		}
	}
}

func TestVirtualOverrideForStoresOnlyDifferences(t *testing.T) {
	for _, tc := range []struct {
		chosen, detected bool
		want             string
	}{
		{true, true, "nil"}, {false, false, "nil"}, {true, false, "true"}, {false, true, "false"},
	} {
		got := "nil"
		if o := virtualOverrideFor(tc.chosen, tc.detected); o != nil {
			got = map[bool]string{true: "true", false: "false"}[*o]
		}
		if got != tc.want {
			t.Errorf("virtualOverrideFor(%t, %t) = %s, want %s", tc.chosen, tc.detected, got, tc.want)
		}
	}
}

func TestSheetShowsVirtualSwitchAndCaption(t *testing.T) {
	app := newTestApp(t)
	app.printers[0].SetDetectedVirtual(false, "physical TCP/IP port 192.168.0.9")
	tt := ui.NewTester(app.view, 860, 620)

	openSheet(t, tt, pdfPrinter)
	if sw := virtualSwitch(t, tt, pdfSwitchLabel); !sw.Checked {
		t.Fatal("virtual printer must open switched on")
	}
	for _, want := range []string{"Virtual printer", "Detected automatically: port PORTPROMPT:."} {
		if !tt.HasText(want) {
			t.Errorf("sheet missing %q; texts: %q", want, tt.Texts())
		}
	}
	if _, ok := tt.AccessByRole(ui.RoleButton, automaticLabel); ok {
		t.Fatal("automatic detection button must be hidden while nothing is overridden")
	}
	snapshot(t, tt, "printer-virtual-auto")

	if sw := toggleSwitch(t, tt, pdfSwitchLabel); sw.Checked {
		t.Fatal("switch did not turn off")
	}
	if !tt.HasText("Set by you. Detected: port PORTPROMPT:.") {
		t.Errorf("overridden caption missing; texts: %q", tt.Texts())
	}
	if _, ok := tt.AccessByRole(ui.RoleButton, automaticLabel); !ok {
		t.Fatal("automatic detection button missing while overridden")
	}
	tt.Advance(time.Second) // let the switch finish sliding before the snapshot
	snapshot(t, tt, "printer-virtual-overridden")
	tt.SetDark(true)
	snapshot(t, tt, "printer-virtual-overridden-dark")
	tt.SetDark(false)
	tt.SetSize(640, 460)
	snapshot(t, tt, "printer-virtual-overridden-min")
	tt.SetDark(true)
	snapshot(t, tt, "printer-virtual-overridden-min-dark")
	tt.SetDark(false)
	tt.SetSize(860, 620)
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
}

func TestSheetVirtualSwitchPersistsOnDone(t *testing.T) {
	app := newTestApp(t)
	app.printers[0].SetDetectedVirtual(false, "physical TCP/IP port 192.168.0.9")
	tt := ui.NewTester(app.view, 860, 620)

	openSheet(t, tt, pdfPrinter)
	toggleSwitch(t, tt, pdfSwitchLabel)
	if app.printers[1].VirtualOverride != nil || !app.printers[1].IsVirtual {
		t.Fatalf("switch applied before Done: %+v", app.printers[1])
	}
	if err := tt.Click("Done"); err != nil {
		t.Fatal(err)
	}
	pdf := printerByName(t, app.printers, pdfPrinter)
	if pdf.IsVirtual || pdf.VirtualOverride == nil || *pdf.VirtualOverride || !pdf.DetectedVirtual {
		t.Fatalf("override not applied: %+v", pdf)
	}
	if entry := savedEntry(t, app, pdfPrinter); entry.VirtualOverride == nil || *entry.VirtualOverride {
		t.Fatalf("override not persisted: %+v", entry)
	}
	if got := app.printers[0].WindowsName; got != "EPSON L3210 Series" && got != pdfPrinter {
		t.Fatalf("unexpected order: %v", windowsNames(app.printers))
	}

	// Toggling back to the detected value stores no override at all.
	openSheet(t, tt, pdfPrinter)
	if !tt.HasText("Set by you. Detected: port PORTPROMPT:.") {
		t.Errorf("reopened sheet lost the override caption; texts: %q", tt.Texts())
	}
	toggleSwitch(t, tt, pdfSwitchLabel)
	if err := tt.Click("Done"); err != nil {
		t.Fatal(err)
	}
	if pdf := printerByName(t, app.printers, pdfPrinter); pdf.VirtualOverride != nil || !pdf.IsVirtual {
		t.Fatalf("matching the detected value must clear the override: %+v", pdf)
	}
	if entry := savedEntry(t, app, pdfPrinter); entry.VirtualOverride != nil {
		t.Fatalf("stale override persisted: %+v", entry)
	}
}

func TestSheetVirtualSwitchDiscardedOnCancel(t *testing.T) {
	app := newTestApp(t)
	tt := ui.NewTester(app.view, 860, 620)
	openSheet(t, tt, pdfPrinter)
	toggleSwitch(t, tt, pdfSwitchLabel)
	if err := tt.Click("Cancel"); err != nil {
		t.Fatal(err)
	}
	if pdf := printerByName(t, app.printers, pdfPrinter); !pdf.IsVirtual || pdf.VirtualOverride != nil {
		t.Fatalf("Cancel kept the draft: %+v", pdf)
	}
	openSheet(t, tt, pdfPrinter)
	if !virtualSwitch(t, tt, pdfSwitchLabel).Checked || !tt.HasText("Detected automatically: port PORTPROMPT:.") {
		t.Fatal("reopened sheet shows the discarded draft")
	}
}

func TestSheetUseAutomaticDetectionClearsOverride(t *testing.T) {
	no := false
	app := newTestApp(t)
	app.printers[1].SetVirtualOverride(&no)
	tt := ui.NewTester(app.view, 860, 620)
	openSheet(t, tt, pdfPrinter)
	if virtualSwitch(t, tt, pdfSwitchLabel).Checked || !tt.HasText("Set by you. Detected: port PORTPROMPT:.") {
		t.Fatalf("sheet does not show the override; texts: %q", tt.Texts())
	}
	if err := tt.Click(automaticLabel); err != nil {
		t.Fatal(err)
	}
	if !virtualSwitch(t, tt, pdfSwitchLabel).Checked || !tt.HasText("Detected automatically: port PORTPROMPT:.") {
		t.Fatalf("draft not reset to automatic; texts: %q", tt.Texts())
	}
	if app.printers[1].VirtualOverride == nil {
		t.Fatal("the override was cleared before Done")
	}
	if err := tt.Click("Done"); err != nil {
		t.Fatal(err)
	}
	if pdf := printerByName(t, app.printers, pdfPrinter); pdf.VirtualOverride != nil || !pdf.IsVirtual {
		t.Fatalf("override not cleared: %+v", pdf)
	}
	if entry := savedEntry(t, app, pdfPrinter); entry.VirtualOverride != nil {
		t.Fatalf("override still persisted: %+v", entry)
	}
}

func TestSheetMarkingPhysicalAsVirtualHidesIt(t *testing.T) {
	app := newTestApp(t)
	app.cfg.ShowVirtualPrinters = false
	app.printers[0].Enabled = false
	tt := ui.NewTester(app.view, 860, 620)
	if !tt.HasText("EPSON L3210 Series") {
		t.Fatal("physical printer must be listed")
	}
	openSheet(t, tt, "EPSON L3210 Series")
	if virtualSwitch(t, tt, epsonSwitch).Checked {
		t.Fatal("physical printer opened as virtual")
	}
	toggleSwitch(t, tt, epsonSwitch)
	if err := tt.Click("Done"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("Name and location for EPSON L3210 Series") {
		t.Fatal("printer marked virtual stays in the list while virtual printers are hidden")
	}
	if app.visiblePrinter(printerByName(t, app.printers, "EPSON L3210 Series")) {
		t.Fatal("printer should be hidden")
	}
	if !tt.HasText("EPSON L3210 Series is now hidden. Turn on Show virtual printers to see it.") {
		t.Errorf("toast missing; texts: %q", tt.Texts())
	}
}

func TestSheetMarkingVirtualAsPhysicalShowsIt(t *testing.T) {
	app := newTestApp(t)
	app.cfg.ShowVirtualPrinters = false
	tt := ui.NewTester(app.view, 860, 620)
	if _, ok := tt.AccessByRole(ui.RoleButton, "Name and location for "+pdfPrinter); ok {
		t.Fatal("virtual printer must be hidden while the filter is off")
	}
	app.openPrinterSheet(&app.printers[1])
	tt.Frame()
	toggleSwitch(t, tt, pdfSwitchLabel)
	if err := tt.Click("Done"); err != nil {
		t.Fatal(err)
	}
	if _, ok := tt.AccessByRole(ui.RoleButton, "Name and location for "+pdfPrinter); !ok {
		t.Fatalf("printer marked physical must appear; texts: %q", tt.Texts())
	}
}
