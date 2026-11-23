package main

import (
	"context"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	bridgenet "github.com/ilyasaftr/OpenPrintCast/internal/net"
)

// appTitle names the app in the window, tray tooltip and header.
const appTitle = "OpenPrintCast"

// Main window geometry in DIPs.
const (
	windowWidth     = 860
	windowHeight    = 620
	windowMinWidth  = 640
	windowMinHeight = 460
)

// newWindow creates the main window; the content closure reads live state
// on every frame, so no refresh plumbing is needed beyond Invalidate.
func (a *bridgeApp) newWindow() *mygo.Window {
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:     appTitle,
		Width:     windowWidth,
		Height:    windowHeight,
		MinWidth:  windowMinWidth,
		MinHeight: windowMinHeight,
		Content:   ui.View(a.view),
	})
	win.OnClose(func(e *mygo.CloseEvent) {
		if a.onWindowClose(win, a.tray != nil) {
			e.PreventDefault()
		}
	})
	return win
}

// show brings the window back: reopening it when destroyed, revealing it
// otherwise, which is the tray menu's and dock activation's entry point.
func (a *bridgeApp) show() {
	if a.win == nil || a.win.IsDestroyed() {
		a.win = a.newWindow()
	}
	a.network().state.SetVisible(true)
	a.win.Show()
}

// showJobs brings the window up on the Jobs tab, where a failed job is
// listed; it is the notification click's entry point.
func (a *bridgeApp) showJobs() { a.showTab(tabJobs) }

// showSettings brings the window up on the Settings tab, where an update is
// offered; it is the update notification click's entry point.
func (a *bridgeApp) showSettings() { a.showTab(tabSettings) }

// showTab brings the window up on tab.
func (a *bridgeApp) showTab(tab int) {
	a.tab = tab
	a.bringForward()
	a.invalidateWindow()
}

// bringForward shows the window: the injected function in tests (a window
// needs the running app), else show.
func (a *bridgeApp) bringForward() {
	if a.raiseWindow != nil {
		a.raiseWindow()
		return
	}
	a.show()
}

// invalidateWindow asks the window, when it still exists, to redraw.
func (a *bridgeApp) invalidateWindow() {
	if a.win != nil && !a.win.IsDestroyed() {
		a.win.Invalidate()
	}
}

// invalidateFromBackground redraws from a non-UI goroutine: it hops to the
// main thread first, because the window handle is UI-owned state.
func (a *bridgeApp) invalidateFromBackground() { mygo.RunOnMain(a.invalidateWindow) }

// syncTray keeps the tray tooltip on the current status, so the state is
// readable without opening the window, and swaps in a fresh menu so its
// Pause/Resume label and printer checkboxes match.
func (a *bridgeApp) syncTray() {
	if a.tray == nil {
		return
	}
	text, _ := a.status()
	a.tray.SetToolTip(appTitle + " — " + text)
	a.tray.SetMenu(a.trayMenu())
}

// setPaused flips the paused flag in the UI and the server thread, then
// refreshes the window and the tray menu label together.
func (a *bridgeApp) setPaused(paused bool) {
	a.paused = paused
	a.shared.SetPaused(paused)
	a.syncTray()
	a.invalidateWindow()
}

// togglePaused inverts the sharing state; tray and window handlers share it.
func (a *bridgeApp) togglePaused() { a.setPaused(!a.paused) }

// registerAppHandlers wires the app's lifecycle events; cancel stops the
// background goroutines on quit.
func (a *bridgeApp) registerAppHandlers(cancel context.CancelFunc) {
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		a.quitting = true
		a.updates().Stop()
		cancel()
	})
	// Stay alive in the tray after the last window closes; X minimizes
	// to tray and Quit comes from the tray menu.
	mygo.App.OnWindowAllClosed(func() {})
	mygo.App.OnActivate(func(hasVisibleWindows bool) {
		if !hasVisibleWindows {
			a.show()
		}
	})
	mygo.App.OnSecondInstance(a.onSecondInstance)
	mygo.App.WhenReady(a.onReady)
}

// run starts the server goroutine and the mygo event loop. It returns when
// the app quits.
func (a *bridgeApp) run() error {
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.network().ctx = ctx
	go runServerForever(ctx, a.shared, bridgenet.DefaultPort)

	if a.printerStatus != nil {
		go a.printerStatus.Run(ctx, a.printerNames.Get, statusPollInterval)
	}

	if a.printerCompat != nil {
		go a.printerCompat.Run(ctx, a.printerList.Get, compatibilityInterval)
	}

	a.updates().Start(ctx)

	a.registerAppHandlers(cancel)
	return mygo.App.Run()
}
