package main

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

func TestSettingsSwitchesPersist(t *testing.T) {
	app := newTestApp(t)
	app.tab = 2
	app.settingsSection = sectionPrinting
	tt := ui.NewTester(app.view, 860, 620)
	for _, name := range []string{"Reverse page order", "Auto-rotate to fit"} {
		sw, ok := tt.AccessByRole(ui.RoleSwitch, name)
		if !ok {
			t.Fatalf("switch %q missing", name)
		}
		tt.ClickAt(sw.Bounds.X+sw.Bounds.W/2, sw.Bounds.Y+sw.Bounds.H/2)
	}
	saved, err := core.LoadConfig(app.store.Location())
	if err != nil || !saved.ReversePageOrder || !saved.AutoRotateToFit {
		t.Fatalf("switches not persisted: %+v, %v", saved, err)
	}
}

func TestQualityFieldStepsTypesAndRejectsOutOfRange(t *testing.T) {
	app := newTestApp(t)
	app.tab = 2
	app.settingsSection = sectionPrinting
	tt := ui.NewTester(app.view, 860, 620)
	spin, ok := tt.Access("Maximum quality in DPI")
	if !ok {
		t.Fatal("quality field missing")
	}
	tt.ClickAt(spin.Bounds.X+12, spin.Bounds.Y+spin.Bounds.H/2)
	tt.Key(0, ui.KeyUp)
	if app.maxRenderDPI != 310 {
		t.Fatalf("Up arrow: %d DPI, want 310", app.maxRenderDPI)
	}
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type("600")
	if app.maxRenderDPI != 600 {
		t.Fatalf("typed 600: %d DPI", app.maxRenderDPI)
	}
	tt.Key(ui.Ctrl, ui.KeyA)
	tt.Type("5000")
	if app.maxRenderDPI < core.MinRenderDPI || app.maxRenderDPI > core.MaxRenderDPI {
		t.Fatalf("out-of-range value applied: %d DPI", app.maxRenderDPI)
	}
	saved, err := core.LoadConfig(app.store.Location())
	if err != nil || saved.MaxRenderDPI != app.maxRenderDPI {
		t.Fatalf("quality not persisted: %+v, %v", saved, err)
	}
}

func TestNotifySwitchTogglesPersistsAndUpdatesAnnouncer(t *testing.T) {
	app := newTestApp(t)
	app.tab = 2
	notifier := &fakeNotifier{}
	app.announcer = newJobFailureAnnouncer(notifier, app.shared, app.shared, app.cfg.NotifyOnFailure)
	failed := core.NewJobEntry("printer-epson", "Agung", "a.pdf", 1)
	failed.Status = core.JobFailed
	tt := ui.NewTester(app.view, 860, 620)
	if !tt.HasText("Notifications") || !tt.HasText("Show a notification when a print job fails. Not shown while sharing is paused.") {
		t.Fatalf("notifications group missing; texts: %q", tt.Texts())
	}
	sw, ok := tt.AccessByRole(ui.RoleSwitch, "Notify when a print job fails")
	if !ok {
		t.Fatal("notification switch missing")
	}
	if !app.cfg.NotifyOnFailure {
		t.Fatal("notifications should default to on")
	}
	tt.ClickAt(sw.Bounds.X+sw.Bounds.W/2, sw.Bounds.Y+sw.Bounds.H/2)
	if app.cfg.NotifyOnFailure {
		t.Fatal("switch did not turn the setting off")
	}
	saved, err := core.LoadConfig(app.store.Location())
	if err != nil || saved.NotifyOnFailure {
		t.Fatalf("setting not persisted: %+v, %v", saved, err)
	}
	app.announcer.OnJob(failed)
	if len(notifier.got()) != 0 {
		t.Fatal("announcer still enabled after switching off")
	}
	snapshot(t, tt, "settings-notifications")
	tt.ClickAt(sw.Bounds.X+sw.Bounds.W/2, sw.Bounds.Y+sw.Bounds.H/2)
	app.announcer.OnJob(failed)
	if len(notifier.got()) != 1 {
		t.Fatal("announcer not re-enabled by the switch")
	}
}

// supplyReadingBackend records the live Read ink and toner levels switch,
// like the real backend's flag.
type supplyReadingBackend struct {
	stubBackend
	calls []bool
}

func (b *supplyReadingBackend) SetReadSupplyLevels(on bool) { b.calls = append(b.calls, on) }

func TestReadSupplyLevelsSwitchIsOnByDefaultAndAppliesLive(t *testing.T) {
	app := newTestApp(t)
	app.tab = 2
	app.settingsSection = sectionPrinting
	backend := &supplyReadingBackend{}
	app.backend = backend
	tt := ui.NewTester(app.view, 860, 620)
	if !tt.HasText(readSupplyLevelsDetail) {
		t.Fatalf("the switch's explanation is missing; texts: %q", tt.Texts())
	}
	sw, ok := tt.AccessByRole(ui.RoleSwitch, readSupplyLevelsLabel)
	if !ok {
		t.Fatal("Read ink and toner levels switch missing or without an accessible label")
	}
	if !app.cfg.ReadSupplyLevels {
		t.Fatal("reading supply levels should default to on")
	}
	snapshot(t, tt, "settings-supply-levels")
	tt.SetDark(true)
	snapshot(t, tt, "settings-supply-levels-dark")
	tt.SetSize(640, 460)
	snapshot(t, tt, "settings-supply-levels-min-dark")
	tt.SetDark(false)
	snapshot(t, tt, "settings-supply-levels-min")
	tt.SetSize(860, 620)
	tt.ClickAt(sw.Bounds.X+sw.Bounds.W/2, sw.Bounds.Y+sw.Bounds.H/2)
	if app.cfg.ReadSupplyLevels || len(backend.calls) != 1 || backend.calls[0] {
		t.Fatalf("switching off: setting=%v, backend calls=%v", app.cfg.ReadSupplyLevels, backend.calls)
	}
	saved, err := core.LoadConfig(app.store.Location())
	if err != nil || saved.ReadSupplyLevels {
		t.Fatalf("an explicit off was not persisted: %+v, %v", saved, err)
	}
	tt.ClickAt(sw.Bounds.X+sw.Bounds.W/2, sw.Bounds.Y+sw.Bounds.H/2)
	if !app.cfg.ReadSupplyLevels || len(backend.calls) != 2 || !backend.calls[1] {
		t.Fatalf("switching on: setting=%v, backend calls=%v", app.cfg.ReadSupplyLevels, backend.calls)
	}
}

func TestReadSupplyLevelsReachesTheBackendAtStartup(t *testing.T) {
	for _, on := range []bool{true, false} {
		backend := &supplyReadingBackend{}
		cfg := core.DefaultConfig()
		cfg.ReadSupplyLevels = on
		applyReadSupplyLevels(backend, cfg)
		if len(backend.calls) != 1 || backend.calls[0] != on {
			t.Fatalf("setting %v: backend calls=%v", on, backend.calls)
		}
	}
	// A backend that cannot read supplies simply ignores the setting.
	applyReadSupplyLevels(stubBackend{}, core.DefaultConfig())
}
