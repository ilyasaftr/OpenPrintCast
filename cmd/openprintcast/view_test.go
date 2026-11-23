package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
	bridgenet "github.com/ilyasaftr/OpenPrintCast/internal/net"
)

// newTestApp builds a bridgeApp over stub printers and a few jobs, with
// the LAN address pinned so the view does not touch the network.
func newTestApp(t *testing.T) *bridgeApp {
	t.Helper()
	printers := []core.PrinterInfo{
		core.NewPrinterInfo("printer-epson", "EPSON L3210 Series", "EPSON L3210 Series (PC)", "Office", true),
		core.NewPrinterInfo("printer-pdf", "Microsoft Print to PDF", "Microsoft Print to PDF (PC)", "", false),
	}
	printers[1].SetDetectedVirtual(true, "port PORTPROMPT:")
	printers[1].DriverName = "Microsoft Print To PDF"
	jobs := &core.JobLog{}
	done := core.NewJobEntry("printer-epson", "Agung", "invoice.pdf", 2)
	done.SetStatus(core.JobCompleted)
	failed := core.NewJobEntry("printer-epson", "Agung", "", 0)
	failed.SetStatus(core.JobFailed)
	jobs.Push(done)
	jobs.Push(failed)
	cfg := core.DefaultConfig()
	cfg.ShowVirtualPrinters = true
	shared := bridgenet.NewSharedState(printers, jobs)
	return &bridgeApp{
		printers:     printers,
		shared:       shared,
		announcer:    newJobFailureAnnouncer(&fakeNotifier{}, shared, shared, cfg.NotifyOnFailure),
		logs:         &fakeLogControl{path: filepath.Join(t.TempDir(), "openprintcast.log")},
		startupEntry: &fakeStartupItem{},
		cfg:          cfg,
		store:        fileConfigStore{path: filepath.Join(t.TempDir(), "config.json")},
		maxRenderDPI: cfg.MaxRenderDPI,
		host:         "PC",
		ip:           "192.168.1.5",
		ipAt:         time.Now(),
	}
}

func snapshot(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	dir := os.Getenv("OPENPRINTCAST_SNAPSHOT_DIR")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

func TestViewPagesRender(t *testing.T) {
	app := newTestApp(t)
	tt := ui.NewTester(app.view, 860, 620)

	for _, want := range []string{"OpenPrintCast", "Sharing 1 of 2", "Pause", "EPSON L3210 Series", "Virtual", "1 of 2 shared", "Scan Again"} {
		if !tt.HasText(want) {
			t.Errorf("printers page missing %q; texts: %q", want, tt.Texts())
		}
	}
	snapshot(t, tt, "printers")
	tt.SetDark(true)
	snapshot(t, tt, "printers-list-dark")
	tt.SetDark(false)
	tt.SetSize(640, 460)
	snapshot(t, tt, "printers-list-min")
	tt.SetSize(860, 620)

	if err := tt.Click("Name and location for EPSON L3210 Series"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("Name on devices") {
		t.Errorf("printer dialog did not open; texts: %q", tt.Texts())
	}
	snapshot(t, tt, "printer-dialog")

	tt.SetDark(true)
	snapshot(t, tt, "printers-dark")
	tt.SetDark(false)

	tt.SetSize(640, 460)
	snapshot(t, tt, "printers-min")
	tt.SetSize(860, 620)

	if err := tt.Click("Done"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("Jobs"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Latest jobs", "Completed", "Failed", "invoice.pdf", "Clear Finished", "Received", "Status"} {
		if !tt.HasText(want) {
			t.Errorf("jobs page missing %q; texts: %q", want, tt.Texts())
		}
	}
	snapshot(t, tt, "jobs")
	tt.SetDark(true)
	snapshot(t, tt, "jobs-dark")
	tt.SetDark(false)
	tt.SetSize(640, 460)
	snapshot(t, tt, "jobs-min")
	tt.SetSize(860, 620)

	if err := tt.Click("Settings"); err != nil {
		t.Fatal(err)
	}
	for _, section := range []struct {
		name  string
		wants []string
	}{
		{"General", []string{"Startup", "Notifications", "History"}},
		{"Printing", []string{"Maximum quality"}},
		{"Network", []string{"This Computer", "Address", "192.168.1.5:631", "Network check"}},
		{"About", []string{"Version", "Settings file", "Verbose logging"}},
	} {
		if err := tt.Click(section.name); err != nil {
			t.Fatal(err)
		}
		for _, want := range section.wants {
			if !tt.HasText(want) {
				t.Errorf("settings section %s missing %q; texts: %q", section.name, want, tt.Texts())
			}
		}
	}
	snapshot(t, tt, "settings")
	tt.SetDark(true)
	snapshot(t, tt, "settings-dark")
	tt.SetDark(false)
	tt.SetSize(640, 460)
	snapshot(t, tt, "settings-min")
}

func TestPausedShowsBannerAndResume(t *testing.T) {
	app := newTestApp(t)
	app.shared.SetPaused(true)
	app.paused = true
	tt := ui.NewTester(app.view, 860, 620)
	for _, want := range []string{"Paused", "Sharing is paused. Print jobs from iPhone, iPad and Mac are rejected until you resume.", "Resume"} {
		if !tt.HasText(want) {
			t.Errorf("paused view missing %q; texts: %q", want, tt.Texts())
		}
	}
	snapshot(t, tt, "paused")
	tt.SetDark(true)
	snapshot(t, tt, "paused-dark")
}

func TestEmptyStatesGuideTheUser(t *testing.T) {
	app := newTestApp(t)
	app.printers = nil
	app.shared = bridgenet.NewSharedState(nil, nil)
	tt := ui.NewTester(app.view, 860, 620)
	if !tt.HasText("No printers found. Add a printer in Windows Settings, or show virtual printers.") || !tt.HasText("Scan Again") {
		t.Errorf("empty printers state missing guidance; texts: %q", tt.Texts())
	}
	snapshot(t, tt, "empty-printers")
	if err := tt.Click("Jobs"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("No Print Jobs") || !tt.HasText("Show Printers") {
		t.Errorf("empty jobs state missing; texts: %q", tt.Texts())
	}
	snapshot(t, tt, "empty-jobs")
}
