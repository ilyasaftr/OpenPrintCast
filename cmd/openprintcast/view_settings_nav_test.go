package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSectionIDOrDefaultFallsBackToGeneral(t *testing.T) {
	for _, id := range []string{"", "diagnostics", "7"} {
		if got := sectionIDOrDefault(id); got != sectionGeneral {
			t.Errorf("sectionIDOrDefault(%q) = %q, want %q", id, got, sectionGeneral)
		}
	}
	for _, s := range newTestApp(t).settingsSections() {
		if got := sectionIDOrDefault(s.id); got != s.id {
			t.Errorf("section %q is not a known id: %q", s.id, got)
		}
	}
}

func TestChosenSectionIsRememberedAcrossRestarts(t *testing.T) {
	app := newTestApp(t)
	app.tab = tabSettings
	tt := ui.NewTester(app.view, 860, 620)
	if err := tt.Click("Network"); err != nil {
		t.Fatal(err)
	}
	if app.settingsSection != sectionNetwork || app.cfg.LastSettingsSection != sectionNetwork {
		t.Fatalf("section = %q, stored = %q", app.settingsSection, app.cfg.LastSettingsSection)
	}
	if got := loadConfig(app.store.Location()).LastSettingsSection; got != sectionNetwork {
		t.Fatalf("saved section = %q", got)
	}
}

func TestSidebarMarksUpdatesWhileAnUpdateIsOnOffer(t *testing.T) {
	app, rig := updateViewApp(t)
	tt := ui.NewTester(app.view, 860, 620)
	clickButton(t, tt, "Check Now")
	finishCheck(tt, rig)
	if err := tt.Click("General"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("1") {
		t.Fatalf("no mark on the Updates item; texts: %q", tt.Texts())
	}
	if _, ok := tt.Access(tabAccessibleName(tabItem{label: "Updates", badge: 1, badgeName: updateBadgeName})); !ok {
		t.Fatal("the Updates item does not say an update is available")
	}
}

func TestSidebarArrowKeysChooseSections(t *testing.T) {
	app := newTestApp(t)
	app.tab = tabSettings
	tt := ui.NewTester(app.view, 860, 620)
	if err := tt.Click("General"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyDown)
	if app.settingsSection != sectionPrinting {
		t.Fatalf("Down chose %q, want %q", app.settingsSection, sectionPrinting)
	}
	tt.Key(0, ui.KeyUp)
	if app.settingsSection != sectionGeneral {
		t.Fatalf("Up chose %q, want %q", app.settingsSection, sectionGeneral)
	}
}
