package main

import (
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

func TestVerboseSwitchPersistsAndTellsTheLogger(t *testing.T) {
	app := newTestApp(t)
	app.tab = 2
	app.settingsSection = sectionAbout
	logs := app.logs.(*fakeLogControl)
	tt := ui.NewTester(app.view, 860, 620)
	for _, want := range []string{"Diagnostics", "Verbose logging", "Record extra detail to help diagnose problems."} {
		if !tt.HasText(want) {
			t.Errorf("diagnostics group missing %q; texts: %q", want, tt.Texts())
		}
	}
	for _, on := range []bool{true, false} {
		if sw := toggleSwitch(t, tt, verboseLoggingLabel); sw.Checked != on {
			t.Fatalf("switch checked = %t, want %t", sw.Checked, on)
		}
		saved, err := core.LoadConfig(app.store.Location())
		if err != nil || saved.VerboseLogging != on {
			t.Fatalf("verbose=%t not persisted: %+v, %v", on, saved, err)
		}
	}
	if want := []bool{true, false}; !reflect.DeepEqual(logs.calls(), want) {
		t.Fatalf("SetVerbose calls = %v, want %v", logs.calls(), want)
	}
}

func TestVerboseSwitchStartsFromPersistedConfig(t *testing.T) {
	app := newTestApp(t)
	app.tab = 2
	app.settingsSection = sectionAbout
	app.cfg.VerboseLogging = true
	tt := ui.NewTester(app.view, 860, 620)
	if sw := scrollToControl(t, tt, ui.RoleSwitch, verboseLoggingLabel, 620); !sw.Checked {
		t.Fatal("switch should start on when verbose logging is persisted")
	}
}

func TestAboutShowsCurrentLogPathAndFollowsChanges(t *testing.T) {
	app := newTestApp(t)
	app.tab = 2
	app.settingsSection = sectionAbout
	logs := app.logs.(*fakeLogControl)
	logs.setPath("/logs/2024-05-01.log")
	tt := ui.NewTester(app.view, 860, 620)
	for _, want := range []string{"Log file", "/logs/2024-05-01.log"} {
		if !tt.HasText(want) {
			t.Fatalf("About missing %q; texts: %q", want, tt.Texts())
		}
	}
	snapshot(t, tt, "settings-bottom")

	logs.setPath("/logs/2024-05-02.log")
	tt.Frame()
	if !tt.HasText("/logs/2024-05-02.log") || tt.HasText("/logs/2024-05-01.log") {
		t.Fatalf("About did not follow the path change; texts: %q", tt.Texts())
	}
}

// TestSettingsSectionsHoldTheirGroupsInOrder checks that each sidebar
// section shows exactly its own groups, top to bottom.
func TestSettingsSectionsHoldTheirGroupsInOrder(t *testing.T) {
	groups := map[string][]string{
		sectionGeneral:  {"Startup", "Notifications", "History"},
		sectionPrinting: {"Printing"},
		sectionNetwork:  {"This Computer", "Connection"},
		sectionUpdates:  {"Updates"},
		sectionAbout:    {"About", "Diagnostics"},
	}
	all := []string{"Startup", "Notifications", "History", "This Computer", "Connection", "Diagnostics", "About"}
	for id, want := range groups {
		app := newTestApp(t)
		app.tab, app.settingsSection = tabSettings, id
		tt := ui.NewTester(app.view, 860, 1600)
		var got []string
		texts := tt.Texts()
		start := slices.Index(texts, "Settings sections") + 1 + len(app.settingsSections())
		for _, text := range texts[start:] {
			if slices.Contains(all, text) || text == "Printing" && id == sectionPrinting || text == "Updates" && id == sectionUpdates {
				got = append(got, text)
			}
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: groups = %v, want %v", id, got, want)
		}
	}
}

// TestSettingsSectionsRender snapshots the sections that need the most room
// in both themes and at both window sizes, when snapshots are on.
func TestSettingsSectionsRender(t *testing.T) {
	app := newTestApp(t)
	app.tab, app.openAtLogin = tabSettings, true
	tt := ui.NewTester(app.view, 860, 620)
	for _, section := range []struct{ id, text string }{{sectionGeneral, "Startup"}, {sectionAbout, "Log file"}, {sectionNetwork, "Network check"}} {
		app.settingsSection = section.id
		for _, view := range []struct {
			dark          bool
			width, height int
		}{{false, 860, 620}, {true, 860, 620}, {false, 640, 460}, {true, 640, 460}} {
			tt.SetDark(view.dark)
			tt.SetSize(view.width, view.height)
			if !tt.HasText(section.text) {
				t.Fatalf("%s %dx%d: %q not shown; texts: %q", section.id, view.width, view.height, section.text, tt.Texts())
			}
			snapshot(t, tt, fmt.Sprintf("settings-%s-%d-%t", section.id, view.width, view.dark))
		}
	}
}
