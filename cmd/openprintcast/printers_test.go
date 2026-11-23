package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
	bridgenet "github.com/ilyasaftr/OpenPrintCast/internal/net"
	"github.com/ilyasaftr/OpenPrintCast/internal/winspool"
)

// stubBackend feeds a fixed descriptor list to seeding tests.
type stubBackend struct{ names []string }

func (s stubBackend) ListPrinters() []winspool.PrinterDescriptor {
	out := make([]winspool.PrinterDescriptor, len(s.names))
	for i, n := range s.names {
		out[i] = winspool.NewPrinterDescriptor(n, i == 0)
	}
	return out
}

func (stubBackend) SpoolBuffer(string, []byte, string) error { return nil }

func (stubBackend) QueryStatus(string) winspool.PrinterStatus {
	return winspool.PrinterStatusUnknown
}

type monochromeBackend struct{ stubBackend }

func (monochromeBackend) ColorSupported(string) (bool, bool) { return false, true }

func TestSeededColorCapability(t *testing.T) {
	printers := seedPrinters(monochromeBackend{stubBackend{names: []string{"Laser"}}}, "PC")
	if len(printers) != 1 || printers[0].SupportsColor() {
		t.Fatalf("monochrome capability not preserved: %+v", printers)
	}
}

type duplexBackend struct {
	stubBackend
	duplex bool
}

func (b duplexBackend) DuplexSupported(string) bool { return b.duplex }

func TestSeededDuplexCapability(t *testing.T) {
	stub := stubBackend{names: []string{"Laser"}}
	for _, tc := range []struct {
		name    string
		backend winspool.Backend
		known   bool
		duplex  bool
	}{
		{"driver offers duplex", duplexBackend{stub, true}, true, true},
		{"driver does not offer duplex", duplexBackend{stub, false}, true, false},
		{"backend cannot probe duplex", stub, false, false},
	} {
		printer := seedPrinters(tc.backend, "PC")[0]
		if (printer.DuplexCapable != nil) != tc.known || printer.SupportsDuplex() != tc.duplex {
			t.Errorf("%s: DuplexCapable = %v, SupportsDuplex = %t", tc.name, printer.DuplexCapable, printer.SupportsDuplex())
		}
	}
}

func TestSeededIDsAreStableSlugs(t *testing.T) {
	backend := stubBackend{names: []string{"OfficeJet Pro", "Microsoft Print to PDF"}}
	first := seedPrinters(backend, "PC")
	second := seedPrinters(backend, "PC")
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Fatalf("unstable id: %q vs %q", first[i].ID, second[i].ID)
		}
	}
	if first[0].ID != "printer-officejet-pro" {
		t.Fatalf("unexpected slug id: %q", first[0].ID)
	}
	if first[0].AirPrintName != "OfficeJet Pro (PC)" {
		t.Fatalf("unexpected airprint name: %q", first[0].AirPrintName)
	}
}

func TestApplySavedPrinters(t *testing.T) {
	seeded := []core.PrinterInfo{
		core.NewPrinterInfo("printer-0", "Office", "Office (PC)", "", true),
		core.NewPrinterInfo("printer-1", "PDF Writer", "PDF Writer (PC)", "", false),
	}
	saved := []core.PrinterEntry{
		{WindowsName: "office", AirPrintName: "Lobby Printer", Location: "Floor 2", Enabled: false},
		{WindowsName: "Gone Printer", AirPrintName: "Ghost", Enabled: true},
	}
	got := applySavedPrinters(seeded, saved)
	if got[0].AirPrintName != "Lobby Printer" || got[0].Location != "Floor 2" || got[0].Enabled {
		t.Fatalf("saved prefs not applied: %+v", got[0])
	}
	if len(got) != 2 {
		t.Fatalf("removed printers must drop out, new ones stay: %d entries", len(got))
	}
	if got[1].AirPrintName != "PDF Writer (PC)" || got[1].Enabled {
		t.Fatalf("unlisted printer must keep seeded defaults: %+v", got[1])
	}
}

// evidenceBackend lists printers classified from real-world evidence.
type evidenceBackend struct {
	stubBackend
	evidence []winspool.PrinterEvidence
}

func (b evidenceBackend) ListPrinters() []winspool.PrinterDescriptor {
	out := make([]winspool.PrinterDescriptor, len(b.evidence))
	for i, e := range b.evidence {
		out[i] = winspool.NewPrinterDescriptorFromEvidence(e, false)
	}
	return out
}

func realWorldBackend() evidenceBackend {
	return evidenceBackend{evidence: []winspool.PrinterEvidence{
		{Name: "Brother HL-L2350DW series", DriverName: "Brother HL-L2350DW series", PortName: "192.168.0.9"},
		{Name: "Microsoft Print to PDF", DriverName: "Microsoft Print To PDF", PortName: "PORTPROMPT:"},
		{Name: "RustDesk Printer", DriverName: "RustDesk v4 Printer Driver", PortName: "RustDesk Printer"},
	}}
}

func printerByName(t *testing.T, printers []core.PrinterInfo, name string) core.PrinterInfo {
	t.Helper()
	for _, p := range printers {
		if p.WindowsName == name {
			return p
		}
	}
	t.Fatalf("printer %q missing in %+v", name, printers)
	return core.PrinterInfo{}
}

func TestSeededPrintersKeepDetectionAndReason(t *testing.T) {
	printers := discoverPrinters(realWorldBackend(), "PC", nil)
	rust := printerByName(t, printers, "RustDesk Printer")
	if !rust.IsVirtual || !rust.DetectedVirtual || rust.VirtualOverride != nil || rust.VirtualReason != "Local Port named like the printer" {
		t.Fatalf("RustDesk Printer: %+v", rust)
	}
	brother := printerByName(t, printers, "Brother HL-L2350DW series")
	if brother.IsVirtual || brother.VirtualReason != "physical TCP/IP port 192.168.0.9" || !brother.Enabled {
		t.Fatalf("Brother: %+v", brother)
	}
}

func TestDiscoverAppliesVirtualOverrides(t *testing.T) {
	yes, no := true, false
	saved := []core.PrinterEntry{
		{WindowsName: "Brother HL-L2350DW series", Enabled: true, VirtualOverride: &yes},
		{WindowsName: "rustdesk printer", Enabled: false, VirtualOverride: &no},
	}
	printers := discoverPrinters(realWorldBackend(), "PC", saved)
	brother := printerByName(t, printers, "Brother HL-L2350DW series")
	if !brother.IsVirtual || brother.DetectedVirtual || brother.VirtualOverride == nil || !*brother.VirtualOverride {
		t.Fatalf("virtual override not applied: %+v", brother)
	}
	rust := printerByName(t, printers, "RustDesk Printer")
	if rust.IsVirtual || !rust.DetectedVirtual || rust.VirtualReason == "" {
		t.Fatalf("physical override not applied or detection lost: %+v", rust)
	}
	// The sort follows the effective verdict: RustDesk is physical now.
	if printers[0].WindowsName != "RustDesk Printer" || !printers[1].IsVirtual || !printers[2].IsVirtual {
		t.Fatalf("order ignores overrides: %v", windowsNames(printers))
	}
}

func TestOrderPrintersDefaultFirstThenPhysical(t *testing.T) {
	mk := func(name string, virtual, isDefault bool) core.PrinterInfo {
		p := core.NewPrinterInfo(name, name, name, "", false)
		p.SetDetectedVirtual(virtual, "")
		p.IsDefault = isDefault
		return p
	}
	printers := []core.PrinterInfo{mk("a-virtual", true, false), mk("b-physical", false, false), mk("c-default-virtual", true, true), mk("d-physical", false, false)}
	orderPrinters(printers)
	want := []string{"c-default-virtual", "b-physical", "d-physical", "a-virtual"}
	for i, name := range want {
		if printers[i].WindowsName != name {
			t.Fatalf("order = %v, want %v", windowsNames(printers), want)
		}
	}
}

func TestVirtualOverrideSurvivesRescan(t *testing.T) {
	app := newTestApp(t)
	app.backend = realWorldBackend()
	yes := true
	app.cfg.Printers = []core.PrinterEntry{{WindowsName: "Brother HL-L2350DW series", Enabled: true, VirtualOverride: &yes}}
	app.rescanPrinters()
	app.rescanPrinters()
	brother := printerByName(t, app.printers, "Brother HL-L2350DW series")
	if !brother.IsVirtual || brother.VirtualOverride == nil || !*brother.VirtualOverride {
		t.Fatalf("override lost by rescan: %+v", brother)
	}
	saved, err := core.LoadConfig(app.store.Location())
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range saved.Printers {
		if entry.WindowsName == "Brother HL-L2350DW series" && (entry.VirtualOverride == nil || !*entry.VirtualOverride) {
			t.Fatalf("override not persisted: %+v", entry)
		}
		if entry.WindowsName == "Microsoft Print to PDF" && entry.VirtualOverride != nil {
			t.Fatalf("automatic printer got an override: %+v", entry)
		}
	}
}

func TestVisibilityAndTrayFollowEffectiveVirtual(t *testing.T) {
	yes, no := true, false
	app := newTestApp(t)
	app.backend = realWorldBackend()
	app.cfg.Printers = []core.PrinterEntry{
		{WindowsName: "Brother HL-L2350DW series", Enabled: false, VirtualOverride: &yes},
		{WindowsName: "Microsoft Print to PDF", Enabled: false, VirtualOverride: &no},
	}
	app.rescanPrinters()
	brother := printerByName(t, app.printers, "Brother HL-L2350DW series")
	pdf := printerByName(t, app.printers, "Microsoft Print to PDF")
	rust := printerByName(t, app.printers, "RustDesk Printer")

	app.cfg.ShowVirtualPrinters = false
	if app.visiblePrinter(brother) || !app.visiblePrinter(pdf) || app.visiblePrinter(rust) {
		t.Fatal("hidden filter must use the effective value")
	}
	if got := trayNames(app.trayModel()); len(got) != 1 || got[0] != "Microsoft Print to PDF" {
		t.Fatalf("tray lists %v, want only the printer overridden to physical", got)
	}
	if app.destinationCount() != 1 {
		t.Fatalf("destinationCount = %d, want 1", app.destinationCount())
	}
	app.cfg.ShowVirtualPrinters = true
	if !app.visiblePrinter(brother) || len(app.trayModel().printers) != 3 {
		t.Fatalf("all printers must show with the filter off: %v", trayNames(app.trayModel()))
	}
}

func trayNames(m trayModel) []string {
	names := make([]string, len(m.printers))
	for i, p := range m.printers {
		names[i] = p.name
	}
	return names
}

// An old config.json from the time the destination was backed by Microsoft
// Print to PDF: the destination must keep its id (and so its UUID and URL),
// name, location and shared state, and the PDF printer's own entry must be
// untouched.
func TestFolderDestinationKeepsItsIdentityFromAnOldConfig(t *testing.T) {
	folder := t.TempDir()
	old := fmt.Sprintf(`{
		"printer_name": "", "max_render_dpi": 300,
		"printers": [{"windows_name": "Microsoft Print to PDF", "airprint_name": "PDF (PC)", "location": "Desk", "enabled": true}],
		"show_virtual_printers": true, "folder_enabled": true, "folder_path": %q
	}`, folder)
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := core.LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	app := newTestApp(t)
	app.cfg = cfg
	app.printers = applySavedPrinters(app.printers, cfg.Printers)
	got, ok := app.folderPrinter()
	if !ok || got.ID != "bridge-print-to-folder" || got.AirPrintName != "Print to Folder (PC)" ||
		got.Location != "" || !got.Enabled || got.FolderPath != folder {
		t.Fatalf("folder destination after loading an old config: %+v, present=%t", got, ok)
	}
	if want := bridgenet.PrinterUUID("bridge-print-to-folder", "PC"); bridgenet.PrinterUUID(got.ID, "PC") != want {
		t.Fatal("the destination's UUID changed")
	}
	if pdf := printerByName(t, app.printers, "Microsoft Print to PDF"); pdf.Location != "Desk" || !pdf.Enabled {
		t.Fatalf("the PDF printer's saved entry changed: %+v", pdf)
	}
	app.persistPrinters()
	if saved := app.cfg; !saved.FolderEnabled || saved.FolderPath != folder {
		t.Fatalf("folder settings lost: %+v", saved)
	}
}
