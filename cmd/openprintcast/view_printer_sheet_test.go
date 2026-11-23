package main

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

func openSheet(t *testing.T, tt *ui.Tester, printer string) {
	t.Helper()
	if err := tt.Click("Name and location for " + printer); err != nil {
		t.Fatal(err)
	}
	if !tt.Focused("Name on devices") {
		t.Fatal("name field did not receive focus")
	}
}

func TestSheetDoneCommitsAndPublishes(t *testing.T) {
	app := newTestApp(t)
	tt := ui.NewTester(app.view, 860, 620)
	if err := tt.Click("Find a printer"); err != nil {
		t.Fatal(err)
	}
	tt.Type("office")
	openSheet(t, tt, "EPSON L3210 Series")
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type("Family printer")
	if err := tt.Click("Location"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type("Living room")
	if app.printers[0].AirPrintName == "Family printer" || app.printers[0].Location == "Living room" {
		t.Fatal("edits were applied before Done")
	}
	if !tt.HasText("Name and Location") {
		t.Fatal("sheet closed while editing")
	}
	if err := tt.Click("Done"); err != nil {
		t.Fatal(err)
	}
	if app.printerDialogOpen || app.printers[0].AirPrintName != "Family printer" || app.printers[0].Location != "Living room" ||
		app.printers[1].Location != "" {
		t.Fatalf("wrong printer settings after Done: %+v", app.printers)
	}
	saved, err := core.LoadConfig(app.store.Location())
	if err != nil || len(saved.Printers) == 0 || saved.Printers[0].AirPrintName != "Family printer" || saved.Printers[0].Location != "Living room" {
		t.Fatalf("printer settings not persisted: %+v, %v", saved.Printers, err)
	}
	published, ok := app.shared.PrinterByID(app.printers[0].ID)
	if !ok || published.DisplayName() != "Family printer" || published.Location != "Living room" {
		t.Fatalf("printer settings not published: %+v", published)
	}
}

func TestSheetCancelAndEscapeDiscardEdits(t *testing.T) {
	app := newTestApp(t)
	original := app.printers[0]
	tt := ui.NewTester(app.view, 860, 620)
	for _, dismiss := range []func(){
		func() {
			if err := tt.Click("Cancel"); err != nil {
				t.Fatal(err)
			}
		},
		func() { tt.Key(0, ui.KeyEscape) },
	} {
		openSheet(t, tt, "EPSON L3210 Series")
		tt.Key(ui.Ctrl, ui.KeyA)
		tt.Type("Discard me")
		dismiss()
		if app.printerDialogOpen || tt.HasText("Name and Location") {
			t.Fatal("sheet did not close")
		}
		if app.printers[0].AirPrintName != original.AirPrintName || app.printers[0].Location != original.Location {
			t.Fatalf("dismissing kept the edits: %+v", app.printers[0])
		}
		if !tt.Focused("Name and location for EPSON L3210 Series") {
			t.Fatal("focus did not return to the button that opened the sheet")
		}
	}
}

func TestSheetEnterCommitsAndInvalidNameBlocksDone(t *testing.T) {
	app := newTestApp(t)
	tt := ui.NewTester(app.view, 860, 620)
	openSheet(t, tt, "Microsoft Print to PDF")
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type(strings.Repeat("x", 64))
	if info, ok := tt.AccessByRole(ui.RoleButton, "Done"); !ok || !info.Disabled {
		t.Fatal("Done must be disabled while the name is too long")
	}
	tt.Key(0, ui.KeyEnter)
	if !app.printerDialogOpen {
		t.Fatal("Enter committed an invalid name")
	}
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type("  Scanner room  ")
	tt.Key(0, ui.KeyEnter)
	if app.printerDialogOpen || app.printers[1].AirPrintName != "Scanner room" {
		t.Fatalf("Enter did not commit the trimmed name: open=%t %q", app.printerDialogOpen, app.printers[1].AirPrintName)
	}
}
