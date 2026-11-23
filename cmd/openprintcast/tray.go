package main

import "github.com/egoist/mygo"

// setupTray creates the tray icon; a missing system tray (for example no
// AppIndicator on Linux) only logs, mirroring the Rust shell's tolerance.
func (a *bridgeApp) setupTray() {
	tray, err := mygo.NewTray(mygo.TrayOptions{
		Icon:    trayIcon(),
		ToolTip: appTitle,
		Menu:    a.trayMenu(),
	})
	if err != nil {
		componentLogger(componentTray).Warn("tray unavailable", "err", err)
		return
	}
	a.tray = tray
	// A left click opens the window, the Windows convention for a status
	// icon; the right click keeps the shortcut menu.
	tray.OnClick(a.show)
	a.syncTray()
}
