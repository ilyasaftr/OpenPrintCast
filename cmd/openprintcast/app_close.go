package main

import "github.com/egoist/mygo"

// closeAction is what the window's close button does.
type closeAction int

const (
	// closeToTray hides the window; the tray icon brings it back.
	closeToTray closeAction = iota
	// closeMinimize keeps a taskbar entry, for a machine with no tray icon.
	closeMinimize
	// closeQuit lets the window close and ends the app.
	closeQuit
)

// chooseCloseAction decides what closing the window does. With a tray icon
// the window hides and the icon reopens it. Without one, hiding would leave
// no way back, so the window minimizes and keeps its taskbar entry; closing
// it again while minimized (the taskbar's own Close) then quits, since
// without a tray there is no Quit menu to reach.
func chooseCloseAction(trayAvailable, minimized bool) closeAction {
	switch {
	case trayAvailable:
		return closeToTray
	case minimized:
		return closeQuit
	default:
		return closeMinimize
	}
}

// closableWindow is the part of the window the close handler drives;
// *mygo.Window implements it.
type closableWindow interface {
	Hide()
	Minimize()
	IsMinimized() bool
}

// onWindowClose handles the close button: it keeps the app running unless
// the app is quitting, and reports whether the close was handled here (the
// caller then cancels it); trayAvailable says whether a tray icon exists.
// It runs on the UI thread.
func (a *bridgeApp) onWindowClose(win closableWindow, trayAvailable bool) (handled bool) {
	if a.quitting {
		return false
	}
	switch chooseCloseAction(trayAvailable, win.IsMinimized()) {
	case closeToTray:
		win.Hide()
		a.network().state.SetVisible(false)
		a.closeHint.OnWindowHidden(true)
	case closeMinimize:
		win.Minimize()
	case closeQuit:
		mygo.App.Quit()
		return false
	}
	return true
}
