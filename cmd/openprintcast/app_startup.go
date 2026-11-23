package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Start-at-login wording.
const (
	openAtLoginLabel  = "Open at login"
	openAtLoginDetail = "Start OpenPrintCast in the tray when you sign in to Windows."
	openAtLoginFailed = "Couldn't change Open at login. Try again, or change it in Windows Settings > Apps > Startup."
)

// startupItem is the system's "start with the user's session" entry.
type startupItem interface {
	Enabled() bool
	SetEnabled(enabled bool) error
}

// mygoStartupItem is the production startupItem, backed by mygo's login
// item (a Run entry of the user on Windows).
type mygoStartupItem struct{}

func (mygoStartupItem) Enabled() bool            { return mygo.App.OpenAtLogin() }
func (mygoStartupItem) SetEnabled(on bool) error { return mygo.App.SetOpenAtLogin(on) }

// nopStartupItem is the item of fixtures that set none: always disabled,
// and changing it succeeds without effect.
type nopStartupItem struct{}

func (nopStartupItem) Enabled() bool         { return false }
func (nopStartupItem) SetEnabled(bool) error { return nil }

// shouldShowWindowOnStart decides whether the window opens at launch. It
// stays hidden only for a launch at login with a tray icon to reach it
// from, so a machine without a tray never ends up with an invisible app.
func shouldShowWindowOnStart(openedAtLogin, trayAvailable bool) bool {
	return !(openedAtLogin && trayAvailable)
}

// startup is the login item, never nil.
func (a *bridgeApp) startup() startupItem {
	if a.startupEntry == nil {
		return nopStartupItem{}
	}
	return a.startupEntry
}

// onReady runs on the UI thread once the app is ready: it reads the login
// item's state, creates the tray and opens the window unless this was a
// background launch at login.
func (a *bridgeApp) onReady() {
	a.openAtLogin = a.startup().Enabled()
	a.notifyHealth.ProbeSupport(mygo.NotificationsSupported)
	a.setupTray()
	if shouldShowWindowOnStart(mygo.App.WasOpenedAtLogin(), a.tray != nil) {
		a.show()
	}
}

// onSecondInstance brings the running window forward when the user starts
// the app again; mygo has told the new process to exit.
func (a *bridgeApp) onSecondInstance([]string, string) { mygo.RunOnMain(a.show) }

// openAtLoginChanged applies the switch to the login item; when the system
// refuses, the switch goes back and the user is told.
func (a *bridgeApp) openAtLoginChanged(c *ui.Context) {
	enabled := a.openAtLogin
	if err := a.startup().SetEnabled(enabled); err != nil {
		a.openAtLogin = !enabled
		componentLogger(componentApp).Warn("set open at login failed", "enabled", enabled, "err", err)
		c.Toast(openAtLoginFailed)
	}
}
