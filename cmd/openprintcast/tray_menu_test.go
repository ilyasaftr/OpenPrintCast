package main

import (
	"reflect"
	"testing"

	"github.com/egoist/mygo"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// fakeTrayActions records the calls the menu makes.
type fakeTrayActions struct{ calls []string }

func (f *fakeTrayActions) show()         { f.calls = append(f.calls, "show") }
func (f *fakeTrayActions) togglePaused() { f.calls = append(f.calls, "toggle") }
func (f *fakeTrayActions) openLog()      { f.calls = append(f.calls, "log") }
func (f *fakeTrayActions) checkForUpdates() {
	f.calls = append(f.calls, "updates")
}
func (f *fakeTrayActions) setPrinterShared(name string, shared bool) {
	f.calls = append(f.calls, "share:"+name+":"+map[bool]string{true: "on", false: "off"}[shared])
}

func itemByLabel(t *testing.T, items []*mygo.MenuItem, label string) *mygo.MenuItem {
	t.Helper()
	for _, item := range items {
		if item.Label == label {
			return item
		}
	}
	t.Fatalf("no item %q in %+v", label, items)
	return nil
}

func click(item *mygo.MenuItem) { item.Click(item, nil) }

func TestTrayItemsLayout(t *testing.T) {
	items := trayItems(trayModel{}, &fakeTrayActions{})
	var shape []string
	for _, item := range items {
		switch {
		case item.Type == mygo.MenuItemSeparator:
			shape = append(shape, "-")
		case item.Role == mygo.RoleQuit:
			shape = append(shape, "quit")
		default:
			shape = append(shape, item.Label)
		}
	}
	want := []string{"Open OpenPrintCast", "Pause Sharing", "-", "Printers", "-", "Check for Updates…", "Open Log File", "-", "quit"}
	if !reflect.DeepEqual(shape, want) {
		t.Fatalf("menu = %q, want %q", shape, want)
	}
}

func TestTrayPauseItemReflectsState(t *testing.T) {
	pause := itemByLabel(t, trayItems(trayModel{}, &fakeTrayActions{}), "Pause Sharing")
	if pause.Checked || pause.Type == mygo.MenuItemCheckbox {
		t.Fatalf("running: %+v", pause)
	}
	act := &fakeTrayActions{}
	resume := itemByLabel(t, trayItems(trayModel{paused: true}, act), "Resume Sharing")
	if resume.Checked || resume.Type == mygo.MenuItemCheckbox {
		t.Fatalf("paused: Resume Sharing must be a plain item, not a checkbox: %+v", resume)
	}
	click(resume)
	if !reflect.DeepEqual(act.calls, []string{"toggle"}) {
		t.Fatalf("calls = %v", act.calls)
	}
}

func TestTrayPrinterSubmenu(t *testing.T) {
	act := &fakeTrayActions{}
	m := trayModel{printers: []trayPrinter{{"EPSON", true}, {"PDF", false}}}
	sub := itemByLabel(t, trayItems(m, act), "Printers").Submenu
	if len(sub) != 2 || !sub[0].Checked || sub[1].Checked || sub[0].Type != mygo.MenuItemCheckbox {
		t.Fatalf("submenu = %+v", sub)
	}
	click(sub[0])
	click(sub[1])
	if want := []string{"share:EPSON:off", "share:PDF:on"}; !reflect.DeepEqual(act.calls, want) {
		t.Fatalf("calls = %v, want %v", act.calls, want)
	}
}

func TestTrayPrinterSubmenuPlaceholder(t *testing.T) {
	sub := itemByLabel(t, trayItems(trayModel{}, &fakeTrayActions{}), "Printers").Submenu
	if len(sub) != 1 || sub[0].Label != "No printers found" || !sub[0].Disabled {
		t.Fatalf("placeholder = %+v", sub)
	}
}

func TestTrayOpenAndLogItemsInvokeActions(t *testing.T) {
	act := &fakeTrayActions{}
	items := trayItems(trayModel{}, act)
	click(itemByLabel(t, items, "Open OpenPrintCast"))
	click(itemByLabel(t, items, "Open Log File"))
	if want := []string{"show", "log"}; !reflect.DeepEqual(act.calls, want) {
		t.Fatalf("calls = %v, want %v", act.calls, want)
	}
}

func TestTrayModelUsesPrinterPageVisibility(t *testing.T) {
	app := newTestApp(t)
	app.paused = true
	m := app.trayModel()
	if !m.paused || len(m.printers) != 2 {
		t.Fatalf("model with virtual shown = %+v", m)
	}
	app.cfg.ShowVirtualPrinters = false
	if got := app.trayModel().printers; len(got) != 1 || got[0] != (trayPrinter{"EPSON L3210 Series", true}) {
		t.Fatalf("hidden virtual printer listed: %+v", got)
	}
	app.printers[1].Enabled = true
	if got := app.trayModel().printers; len(got) != 2 || !got[1].shared {
		t.Fatalf("shared virtual printer must stay listed: %+v", got)
	}
}

func TestSetPrinterSharedPublishesAndPersists(t *testing.T) {
	app := newTestApp(t)
	app.setPrinterShared("Microsoft Print to PDF", true)
	if !app.printers[1].Enabled {
		t.Fatal("printer not enabled")
	}
	select {
	case <-app.shared.Changed():
	default:
		t.Fatal("server thread was not notified")
	}
	if got := len(app.shared.EnabledPrinters()); got != 2 {
		t.Fatalf("server sees %d enabled printers, want 2", got)
	}
	saved, err := core.LoadConfig(app.store.Location())
	if err != nil || len(saved.Printers) != 2 || !saved.Printers[1].Enabled {
		t.Fatalf("not persisted: %+v, %v", saved, err)
	}
}
