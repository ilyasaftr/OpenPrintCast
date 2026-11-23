package main

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// fakeStore is an in-memory configStore that records every Save.
type fakeStore struct {
	saved []core.AppConfig
	err   error
}

func (f *fakeStore) Save(cfg core.AppConfig) error {
	f.saved = append(f.saved, cfg)
	return f.err
}

func (f *fakeStore) Location() string { return "memory" }

func TestPersistSettingsSavesRenderSettings(t *testing.T) {
	app := newTestApp(t)
	store := &fakeStore{}
	app.store = store
	app.reversePageOrder, app.autoRotateToFit, app.maxRenderDPI = true, true, 300

	app.persistSettings()

	if len(store.saved) != 1 {
		t.Fatalf("Save called %d times, want 1", len(store.saved))
	}
	got := store.saved[0]
	if !got.ReversePageOrder || !got.AutoRotateToFit || got.MaxRenderDPI != 300 {
		t.Fatalf("saved settings = %+v", got)
	}
}

func TestPersistPrintersSavesEntriesByWindowsName(t *testing.T) {
	app := newTestApp(t)
	store := &fakeStore{}
	app.store = store

	app.persistPrinters()

	if len(store.saved) != 1 {
		t.Fatalf("Save called %d times, want 1", len(store.saved))
	}
	entries := store.saved[0].Printers
	if len(entries) != 2 || entries[0].WindowsName != "EPSON L3210 Series" ||
		entries[0].AirPrintName != "EPSON L3210 Series (PC)" || entries[0].Location != "Office" || !entries[0].Enabled ||
		entries[1].WindowsName != "Microsoft Print to PDF" || entries[1].Enabled {
		t.Fatalf("saved printers = %+v", entries)
	}
}

func TestSaveFailureIsLoggedNotFatal(t *testing.T) {
	app := newTestApp(t)
	app.store = &fakeStore{err: errors.New("disk full")}
	logged := captureLogs(t)

	app.persistSettings()
	app.persistPrinters()

	for _, want := range []string{
		`level=ERROR msg="save failed" component=config action="save config" err="disk full"`,
		`level=ERROR msg="save failed" component=config action="save printers" err="disk full"`,
	} {
		if !strings.Contains(logged.String(), want) {
			t.Errorf("log missing %q; got %q", want, logged.String())
		}
	}
}

func TestTabFromNameFallsBackToPrinters(t *testing.T) {
	for name, want := range map[string]int{
		"printers": tabPrinters, "jobs": tabJobs, "settings": tabSettings,
		"": tabPrinters, "Jobs": tabPrinters, "unknown": tabPrinters,
	} {
		if got := tabFromName(name); got != want {
			t.Errorf("tabFromName(%q) = %d, want %d", name, got, want)
		}
	}
	for tab, want := range map[int]string{tabPrinters: "printers", tabJobs: "jobs", tabSettings: "settings", -1: "printers", 9: "printers"} {
		if got := tabName(tab); got != want {
			t.Errorf("tabName(%d) = %q, want %q", tab, got, want)
		}
	}
}

func TestChangingTabSavesItAndNothingElseAboutThePage(t *testing.T) {
	app := newTestApp(t)
	store := &fakeStore{}
	app.store = store
	tt := ui.NewTester(app.view, 860, 620)
	if len(store.saved) != 0 {
		t.Fatalf("first frame saved %d times; the default tab needs no save", len(store.saved))
	}
	app.printerQuery = "epson"
	if err := tt.Click("Jobs"); err != nil {
		t.Fatal(err)
	}
	if len(store.saved) != 1 || store.saved[0].LastTab != "jobs" {
		t.Fatalf("saved = %+v, want one save with last_tab jobs", store.saved)
	}
	if got, err := json.Marshal(store.saved[0]); err != nil || strings.Contains(string(got), "epson") {
		t.Fatalf("the search text must not be persisted: %s", got)
	}
}

func TestLastTabRoundTripsThroughTheConfigFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), configFileName)
	cfg := core.DefaultConfig()
	cfg.LastTab = "settings"
	if err := cfg.SaveConfig(path); err != nil {
		t.Fatal(err)
	}
	if got := tabFromName(loadConfig(path).LastTab); got != tabSettings {
		t.Fatalf("restored tab = %d, want settings", got)
	}
	writeConfigFile(t, path, `{"last_tab": 7}`)
	if got := tabFromName(loadConfig(path).LastTab); got != tabPrinters {
		t.Fatalf("a malformed last_tab gave tab %d, want printers", got)
	}
}
