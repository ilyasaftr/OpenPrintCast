package main

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

func TestVirtualPrintersHiddenUnlessSharedOrRequested(t *testing.T) {
	app := newTestApp(t)
	app.cfg.ShowVirtualPrinters = false
	tt := ui.NewTester(app.view, 860, 620)
	if tt.HasText("Microsoft Print to PDF") {
		t.Fatal("unshared virtual printer should be hidden")
	}
	app.printers[1].Enabled = true
	tt = ui.NewTester(app.view, 860, 620)
	if !tt.HasText("Microsoft Print to PDF") || !tt.HasText("Previously shared virtual printer; turn off sharing to hide it.") {
		t.Fatalf("previous share is hidden: %q", tt.Texts())
	}
	app.printers[1].Enabled = false
	app.cfg.ShowVirtualPrinters = true
	tt = ui.NewTester(app.view, 860, 620)
	if !tt.HasText("Microsoft Print to PDF") {
		t.Fatal("override did not show the virtual printer")
	}
}

func TestFolderSharingPublishesSeparateDestination(t *testing.T) {
	app := newTestApp(t)
	app.cfg.FolderPath = t.TempDir()
	tt := ui.NewTester(app.view, 860, 620)
	if err := tt.Click("Share Print to Folder"); err != nil {
		t.Fatal(err)
	}
	folder, ok := app.shared.PrinterByID(folderPrinterID)
	if !ok || !folder.Enabled || folder.FolderPath != app.cfg.FolderPath || folder.WindowsName != core.FolderPrinterName {
		t.Fatalf("unexpected folder destination: %+v, present=%t", folder, ok)
	}
	if folder.DisplayName() != "Print to Folder (PC)" {
		t.Fatalf("unexpected advertised name %q", folder.DisplayName())
	}
	saved, err := core.LoadConfig(app.store.Location())
	if err != nil || !saved.FolderEnabled || saved.FolderPath != app.cfg.FolderPath {
		t.Fatalf("folder settings not persisted: %+v, %v", saved, err)
	}
}

func TestFolderDestinationStatesItsOwnCapabilities(t *testing.T) {
	app := newTestApp(t)
	app.cfg.FolderPath = t.TempDir()
	app.cfg.FolderEnabled = true
	duplex, color := true, false
	app.printers[0].DuplexCapable, app.printers[0].ColorCapable = &duplex, &color
	app.printers[1].DuplexCapable, app.printers[1].ColorCapable = &duplex, &color
	folder, ok := app.folderPrinter()
	if !ok || folder.SupportsDuplex() || !folder.SupportsColor() {
		t.Fatalf("folder destination must not borrow capabilities from a Windows queue: %+v, present=%t", folder, ok)
	}
}

func TestFolderDoesNotNeedAWindowsPrinter(t *testing.T) {
	app := newTestApp(t)
	app.printers = nil
	app.cfg.FolderPath = t.TempDir()
	tt := ui.NewTester(app.view, 860, 620)
	if tt.HasText("Install Microsoft Print to PDF and scan again to enable this destination.") {
		t.Fatal("the destination still asks for Microsoft Print to PDF")
	}
	if err := tt.Click("Share Print to Folder"); err != nil {
		t.Fatal(err)
	}
	folder, ok := app.shared.PrinterByID(folderPrinterID)
	if !ok || !folder.Enabled || !folder.IsFolderDestination() {
		t.Fatalf("the folder destination needs no printer: %+v, present=%t", folder, ok)
	}
}

func TestFolderDestinationIgnoresQueuesOfThePDFDriver(t *testing.T) {
	app := newTestApp(t)
	app.cfg.FolderPath = t.TempDir()
	app.cfg.FolderEnabled = true
	app.printers[1].IsRemote = true
	if _, ok := app.folderPrinter(); !ok {
		t.Fatal("a remote PDF queue must not matter for local folder output")
	}
}

func TestFolderCaptionStatesWhatIsSaved(t *testing.T) {
	app := newTestApp(t)
	for _, folder := range []string{"", t.TempDir()} {
		app.cfg.FolderPath = folder
		tt := ui.NewTester(app.view, 860, 620)
		if !tt.HasText(folderBehaviorCaption) {
			t.Fatalf("folder %q: caption missing; texts: %q", folder, tt.Texts())
		}
		if hint := tt.HasText("Choose a folder to enable sharing."); hint != (folder == "") {
			t.Fatalf("folder %q: choose-a-folder hint shown = %t", folder, hint)
		}
	}
}

// snapshotFolder writes the Printers page at both window sizes in both
// themes, scrolled to the bottom so the Destinations group is in view.
// scrollToBottom is a wheel distance larger than any page of the tests.
const scrollToBottom = 2000

func snapshotFolder(t *testing.T, app *bridgeApp, name string) {
	t.Helper()
	tt := ui.NewTester(app.view, 860, 620)
	for _, size := range [][2]int{{860, 620}, {640, 460}} {
		for _, dark := range []bool{false, true} {
			tt.SetSize(size[0], size[1])
			tt.SetDark(dark)
			tt.Scroll(float32(size[0])/2, float32(size[1])/2, 0, scrollToBottom)
			theme := map[bool]string{false: "light", true: "dark"}[dark]
			snapshot(t, tt, fmt.Sprintf("%s-%d-%s", name, size[0], theme))
			tt.Scroll(float32(size[0])/2, float32(size[1])/2, 0, -scrollToBottom)
		}
	}
}

func TestFolderRowSnapshots(t *testing.T) {
	app := newTestApp(t)
	snapshotFolder(t, app, "folder-none")
	app.cfg.FolderPath = t.TempDir()
	app.cfg.FolderEnabled = true
	snapshotFolder(t, app, "folder-chosen")
}

func TestSharingSwitchPublishesChange(t *testing.T) {
	app := newTestApp(t)
	tt := ui.NewTester(app.view, 860, 620)
	if err := tt.Click("Share Microsoft Print to PDF"); err != nil {
		t.Fatal(err)
	}
	if !app.printers[1].Enabled {
		t.Fatal("switch did not enable the printer")
	}
	select {
	case <-app.shared.Changed():
	default:
		t.Fatal("server thread was not notified of the change")
	}
	if got := len(app.shared.EnabledPrinters()); got != 2 {
		t.Fatalf("server sees %d enabled printers, want 2", got)
	}
}

func TestSwitchesSitInRowsWithoutStateCaptions(t *testing.T) {
	for _, size := range [][2]int{{860, 620}, {640, 460}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			app := newTestApp(t)
			tt := ui.NewTester(app.view, size[0], size[1])
			for _, text := range tt.Texts() {
				if text == "On" || text == "Off" || text == "Shared" || text == "Not shared" {
					t.Fatalf("switch state is captioned %q; the switch position is the state", text)
				}
			}
			first, ok := tt.Find("Share EPSON L3210 Series")
			second, ok2 := tt.Find("Share Microsoft Print to PDF")
			if !ok || !ok2 || first.X != second.X || first.W != second.W {
				t.Fatalf("switches are not aligned at the row's trailing edge: %v %v", first, second)
			}
			info, _ := tt.Find("Name and location for EPSON L3210 Series")
			if info.X+info.W > first.X {
				t.Fatalf("info button %v overlaps the switch %v", info, first)
			}
			before := tt.Texts()
			if err := tt.Click("Share EPSON L3210 Series"); err != nil {
				t.Fatal(err)
			}
			if sw, ok := tt.AccessByRole(ui.RoleSwitch, "Share EPSON L3210 Series"); !ok || sw.Checked {
				t.Fatal("switch did not expose its off state")
			}
			if after, _ := tt.Find("Share EPSON L3210 Series"); after != first {
				t.Fatalf("switch moved on toggle: %v -> %v", first, after)
			}
			tt.Key(0, ui.KeySpace)
			if sw, ok := tt.AccessByRole(ui.RoleSwitch, "Share EPSON L3210 Series"); !ok || !sw.Checked {
				t.Fatal("keyboard did not turn sharing back on")
			}
			if got := tt.Texts(); fmt.Sprint(got) != fmt.Sprint(before) {
				t.Fatalf("content changed after toggling off and on:\n%q\n%q", before, got)
			}
		})
	}
}

func TestFolderSharingControlsStayInPlace(t *testing.T) {
	app := newTestApp(t)
	app.cfg.FolderPath = t.TempDir()
	tt := ui.NewTester(app.view, 860, 620)
	beforeSwitch, ok := tt.Find("Share Print to Folder")
	if !ok {
		t.Fatal("folder sharing switch missing")
	}
	beforeButton, ok := tt.Find("Choose Folder…")
	if !ok {
		t.Fatal("choose folder button missing")
	}
	if err := tt.Click("Share Print to Folder"); err != nil {
		t.Fatal(err)
	}
	afterSwitch, _ := tt.Find("Share Print to Folder")
	afterButton, _ := tt.Find("Choose Folder…")
	if !app.cfg.FolderEnabled || beforeSwitch != afterSwitch || beforeButton != afterButton {
		t.Fatal("folder sharing did not update without moving its controls")
	}
}

func TestFolderSharingShowsWhenSetupIsNeeded(t *testing.T) {
	app := newTestApp(t)
	app.cfg.FolderEnabled = true
	tt := ui.NewTester(app.view, 860, 620)
	if !tt.HasText("Needs setup") || !tt.HasText("Choose a folder to enable sharing.") {
		t.Fatalf("missing folder setup guidance: %q", tt.Texts())
	}
	snapshot(t, tt, "folder-needs-setup")
	if err := tt.Click("Share Print to Folder"); err != nil {
		t.Fatal(err)
	}
	if app.cfg.FolderEnabled {
		t.Fatal("folder sharing could not be turned off")
	}
}

func TestPrinterSearchAndSharing(t *testing.T) {
	app := newTestApp(t)
	tt := ui.NewTester(app.view, 860, 620)
	if err := tt.Click("Find a printer"); err != nil {
		t.Fatal(err)
	}
	tt.Type("office")
	if !tt.HasText("EPSON L3210 Series") || tt.HasText("Microsoft Print to PDF") {
		t.Fatalf("location search returned wrong printers: %q", tt.Texts())
	}
	if err := tt.Click("Share EPSON L3210 Series"); err != nil {
		t.Fatal(err)
	}
	if app.printers[0].Enabled || app.printers[1].Enabled || len(app.shared.EnabledPrinters()) != 0 {
		t.Fatal("sharing changed the wrong printer or was not published")
	}
	if err := tt.Click("Find a printer"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Type("pdf")
	if !tt.HasText("Microsoft Print to PDF") || tt.HasText("EPSON L3210 Series") {
		t.Fatalf("name search returned wrong printers: %q", tt.Texts())
	}
	tt.Key(0, ui.KeyEscape)
	tt.Type("no such printer")
	if !tt.HasText("No printers match your search. Try another name or location.") || !tt.HasText("Print to Folder") {
		t.Fatalf("search empty state or folder destination missing: %q", tt.Texts())
	}
}

func TestLongPrinterNameKeepsSharingAccessible(t *testing.T) {
	app := newTestApp(t)
	app.printers[0].WindowsName = "A very long printer name for the office on the third floor with an extra descriptive suffix"
	tt := ui.NewTester(app.view, 640, 460)
	if err := tt.Click("Share " + app.printers[0].WindowsName); err != nil {
		t.Fatal(err)
	}
	if app.printers[0].Enabled {
		t.Fatal("sharing switch did not change the printer")
	}
	if err := tt.Click("Name and location for " + app.printers[0].WindowsName); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Name and Location") {
		t.Fatal("sheet did not open")
	}
}
