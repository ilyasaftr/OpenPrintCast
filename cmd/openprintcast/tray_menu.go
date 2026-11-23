package main

import (
	"github.com/egoist/mygo"
)

// Tray menu labels.
const (
	trayOpenLabel     = "Open " + appTitle
	trayPauseLabel    = "Pause Sharing"
	trayResumeLabel   = "Resume Sharing"
	trayPrintersLabel = "Printers"
	trayNoPrinters    = "No printers found"
	trayUpdatesLabel  = "Check for Updates…"
	trayOpenLogLabel  = "Open Log File"
)

// trayPrinter is one Windows printer as the tray menu lists it.
type trayPrinter struct {
	name   string
	shared bool
}

// trayModel is everything the tray menu shows, free of app state.
type trayModel struct {
	paused   bool
	printers []trayPrinter
}

// trayActions is what the tray menu can ask the app to do.
type trayActions interface {
	show()
	togglePaused()
	setPrinterShared(windowsName string, shared bool)
	checkForUpdates()
	openLog()
}

// trayItems builds the menu items for m; clicks go to act. It is pure so
// the menu can be inspected without an app or a tray.
func trayItems(m trayModel, act trayActions) []*mygo.MenuItem {
	return []*mygo.MenuItem{
		{Label: trayOpenLabel, Click: func(*mygo.MenuItem, *mygo.Window) { act.show() }},
		pauseItem(m.paused, act),
		mygo.Separator(),
		{Label: trayPrintersLabel, Submenu: printerItems(m.printers, act)},
		mygo.Separator(),
		{Label: trayUpdatesLabel, Click: func(*mygo.MenuItem, *mygo.Window) { act.checkForUpdates() }},
		{Label: trayOpenLogLabel, Click: func(*mygo.MenuItem, *mygo.Window) { act.openLog() }},
		mygo.Separator(),
		{Role: mygo.RoleQuit},
	}
}

// pauseItem is the Pause/Resume command. It changes its title with the
// state and has no checkmark: HIG menus say a toggling item shows one or
// the other, never both.
func pauseItem(paused bool, act trayActions) *mygo.MenuItem {
	label := trayPauseLabel
	if paused {
		label = trayResumeLabel
	}
	return &mygo.MenuItem{
		Label: label,
		Click: func(*mygo.MenuItem, *mygo.Window) { act.togglePaused() },
	}
}

// printerItems is one sharing checkbox per printer, or a disabled
// placeholder when there are none.
func printerItems(printers []trayPrinter, act trayActions) []*mygo.MenuItem {
	if len(printers) == 0 {
		return []*mygo.MenuItem{{Label: trayNoPrinters, Disabled: true}}
	}
	items := make([]*mygo.MenuItem, 0, len(printers))
	for _, p := range printers {
		items = append(items, printerItem(p, act))
	}
	return items
}

// printerItem is the sharing checkbox of one printer; a click flips the
// state the menu was built with.
func printerItem(p trayPrinter, act trayActions) *mygo.MenuItem {
	return &mygo.MenuItem{
		Label:   p.name,
		Type:    mygo.MenuItemCheckbox,
		Checked: p.shared,
		Click:   func(*mygo.MenuItem, *mygo.Window) { act.setPrinterShared(p.name, !p.shared) },
	}
}

// trayModel snapshots the state the tray menu shows: the printers the
// Printers page lists, with their sharing state.
func (a *bridgeApp) trayModel() trayModel {
	m := trayModel{paused: a.paused}
	for _, p := range a.printers {
		if a.visiblePrinter(p) {
			m.printers = append(m.printers, trayPrinter{name: p.WindowsName, shared: p.Enabled})
		}
	}
	return m
}

// trayMenu builds the menu for the current state. mygo menus are
// snapshots, so syncTray swaps in a fresh one whenever sharing changes.
func (a *bridgeApp) trayMenu() *mygo.Menu {
	return mygo.NewMenu(trayItems(a.trayModel(), a))
}
