package main

import (
	"github.com/egoist/mygo/ui"
)

// Diagnostics group wording; noLogFileText shows when no log is written.
const (
	verboseLoggingLabel  = "Verbose logging"
	verboseLoggingDetail = "Record extra detail to help diagnose problems."
	noLogFileText        = "Not available"
)

// startupGroup holds the Start with Windows switch; the login item applies it.
func (a *bridgeApp) startupGroup(c *ui.Context) {
	groupHeader(c, "Startup", nil)
	listGroupInset(c, settingInset, func() {
		settingRow(c, startWithWindowsLabel, startWithWindowsDetail, func() {
			if ui.Switch(c, &a.openAtLogin).Label(startWithWindowsLabel).Changed() {
				a.openAtLoginChanged(c)
			}
		})
	})
}

// networkGroup holds the Network check row: whether other devices can
// reach this PC.
func (a *bridgeApp) networkGroup(c *ui.Context) {
	groupHeader(c, "Connection", nil)
	listGroupInset(c, settingInset, func() { a.networkCheckRow(c) })
}

// diagnosticsGroup holds the verbose-logging switch; it saves on change.
func (a *bridgeApp) diagnosticsGroup(c *ui.Context) {
	groupHeader(c, "Diagnostics", nil)
	listGroupInset(c, settingInset, func() {
		settingRow(c, verboseLoggingLabel, verboseLoggingDetail, func() {
			if ui.Switch(c, &a.cfg.VerboseLogging).Label(verboseLoggingLabel).Changed() {
				a.verboseLoggingChanged()
			}
		})
	})
}
