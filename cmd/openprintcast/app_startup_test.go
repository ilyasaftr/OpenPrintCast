package main

import (
	"errors"
	"reflect"
	"testing"

	"github.com/egoist/mygo/ui"
)

// scrollToControl scrolls the settings page until the named control is
// fully inside the window, and returns its accessible info.
func scrollToControl(t *testing.T, tt *ui.Tester, role ui.Role, name string, windowH float32) ui.AccessInfo {
	t.Helper()
	const step = 120
	for i := 0; i < 40; i++ {
		info, ok := tt.AccessByRole(role, name)
		if !ok {
			t.Fatalf("%v %q missing; texts: %q", role, name, tt.Texts())
		}
		if info.Bounds.Y >= 0 && info.Bounds.Y+info.Bounds.H <= windowH {
			return info
		}
		tt.Scroll(430, windowH/2, 0, step)
	}
	t.Fatalf("%q never scrolled into view", name)
	return ui.AccessInfo{}
}

func toggleSwitch(t *testing.T, tt *ui.Tester, name string) ui.AccessInfo {
	t.Helper()
	sw := scrollToControl(t, tt, ui.RoleSwitch, name, 620)
	tt.ClickAt(sw.Bounds.X+sw.Bounds.W/2, sw.Bounds.Y+sw.Bounds.H/2)
	after, _ := tt.AccessByRole(ui.RoleSwitch, name)
	return after
}

func TestShouldShowWindowOnStart(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		openedAtLogin, trayExists bool
		want                      bool
	}{
		{"normal launch with tray", false, true, true},
		{"normal launch without tray", false, false, true},
		{"login launch with tray stays in the tray", true, true, false},
		{"login launch without tray must stay visible", true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldShowWindowOnStart(tc.openedAtLogin, tc.trayExists); got != tc.want {
				t.Fatalf("shouldShowWindowOnStart(%t, %t) = %t, want %t", tc.openedAtLogin, tc.trayExists, got, tc.want)
			}
		})
	}
}

func TestStartupGroupWordingAndAccessibleLabel(t *testing.T) {
	app := newTestApp(t)
	app.tab = 2
	tt := ui.NewTester(app.view, 860, 620)
	scrollToControl(t, tt, ui.RoleSwitch, openAtLoginLabel, 620)
	for _, want := range []string{"Startup", "Open at login", "Start OpenPrintCast in the tray when you sign in to Windows."} {
		if !tt.HasText(want) {
			t.Errorf("startup group missing %q; texts: %q", want, tt.Texts())
		}
	}
	if _, ok := tt.Access(openAtLoginLabel); !ok {
		t.Fatal("nothing is labelled Open at login")
	}
	if sw, ok := tt.AccessByRole(ui.RoleSwitch, openAtLoginLabel); !ok || sw.Checked {
		t.Fatalf("Open at login: %+v, found %t", sw, ok)
	}
}

func TestOpenAtLoginSwitchCallsItemAndKeepsStateOnSuccess(t *testing.T) {
	app := newTestApp(t)
	app.tab = 2
	item := &fakeStartupItem{}
	app.startupEntry = item
	tt := ui.NewTester(app.view, 860, 620)

	if sw := toggleSwitch(t, tt, openAtLoginLabel); !sw.Checked {
		t.Fatal("switch did not turn on")
	}
	if !app.openAtLogin || !item.enabled {
		t.Fatalf("state not kept: ui %t, item %t", app.openAtLogin, item.enabled)
	}
	if sw := toggleSwitch(t, tt, openAtLoginLabel); sw.Checked {
		t.Fatal("switch did not turn off")
	}
	if want := []bool{true, false}; !reflect.DeepEqual(item.calls, want) {
		t.Fatalf("SetEnabled calls = %v, want %v", item.calls, want)
	}
	if tt.HasText(openAtLoginFailed) {
		t.Fatal("a toast was shown on success")
	}
}

func TestOpenAtLoginSwitchRevertsAndToastsOnError(t *testing.T) {
	app := newTestApp(t)
	app.tab = 2
	item := &fakeStartupItem{err: errors.New("registry denied")}
	app.startupEntry = item
	tt := ui.NewTester(app.view, 860, 620)

	if sw := toggleSwitch(t, tt, openAtLoginLabel); sw.Checked {
		t.Fatal("switch stayed on after the item refused")
	}
	if app.openAtLogin {
		t.Fatal("UI state not reverted")
	}
	if !tt.HasText("Couldn't change Open at login. Try again, or change it in Windows Settings > Apps > Startup.") {
		t.Fatalf("toast missing; texts: %q", tt.Texts())
	}
	if len(item.calls) != 1 || !item.calls[0] {
		t.Fatalf("SetEnabled calls = %v, want [true]", item.calls)
	}
}

func TestStartupItemDefaultsAreInert(t *testing.T) {
	app := &bridgeApp{}
	if app.startup().Enabled() || app.startup().SetEnabled(true) != nil || app.startup().Enabled() {
		t.Fatal("default startup item should stay disabled and error-free")
	}
}
