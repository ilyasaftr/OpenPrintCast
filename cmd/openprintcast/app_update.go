package main

import (
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/updatecheck"
)

// downloadFailedText is the toast when the release page cannot be opened.
const downloadFailedText = "Couldn't open the download page."

// urlOpener opens a web address in the user's default browser.
type urlOpener func(url string) error

// shellOpenURL is the production urlOpener: the system shell.
func shellOpenURL(url string) error { return mygo.Shell.OpenExternal(url) }

// updates is the update service, never nil: fixtures that set none get one
// that is not set up.
func (a *bridgeApp) updates() updateService {
	if a.update == nil {
		return noUpdates{}
	}
	return a.update
}

// newUpdateService builds the production service for app: the feed of the
// build's setup, the config as its settings, and a notice that opens the
// Settings page when clicked. All mygo calls it makes are in thin adapters
// (the runner, the timer, the notifier).
func newUpdateService(app *bridgeApp, setup updateSetup) updateService {
	return newUpdateController(updateDeps{
		feed:     updatecheck.NewFeed(updatecheck.Options{URL: setup.feedURL, Version: version}),
		current:  setup.current,
		phase:    setup.phase,
		settings: appUpdateSettings{app: app},
		notifier: systemNotifier{onClick: app.showSettings, health: app.notifyHealth},
		run:      runOffUIThread,
		now:      time.Now,
		after:    mainThreadAfter,
		jitter:   randomJitter,
		redraw:   app.invalidateWindow,
	})
}

// appUpdateSettings is the update feature's view of the config.
type appUpdateSettings struct{ app *bridgeApp }

func (s appUpdateSettings) AutoCheck() bool   { return s.app.cfg.AutoUpdateCheck }
func (s appUpdateSettings) IncludeBeta() bool { return s.app.cfg.UpdateBeta }
func (s appUpdateSettings) Skipped() string   { return s.app.cfg.SkippedUpdate }

// SetSkipped saves the user's choice; a failed save is shown, as for any
// other switch.
func (s appUpdateSettings) SetSkipped(tag string) {
	s.app.cfg.SkippedUpdate = tag
	s.app.saveConfig("save skipped update")
}

// LastChecked is when the last check finished, or the zero time.
func (s appUpdateSettings) LastChecked() time.Time {
	if s.app.cfg.LastUpdateCheck <= 0 {
		return time.Time{}
	}
	return time.Unix(s.app.cfg.LastUpdateCheck, 0)
}

// SetLastChecked saves when a check finished. It is written after every
// check, background ones included, so a failed save is only logged: a
// background check never raises a toast.
func (s appUpdateSettings) SetLastChecked(at time.Time) {
	s.app.cfg.LastUpdateCheck = at.Unix()
	s.app.trySaveConfig("save last update check")
}

// autoUpdateCheckChanged applies the automatic-checks switch: it saves the
// config, then starts or stops the schedule.
func (a *bridgeApp) autoUpdateCheckChanged() {
	a.persistSettings()
	a.updates().AutoCheckChanged()
}

// updateBetaChanged applies the beta switch: it saves the config, then
// re-reads the last feed answer under the new setting.
func (a *bridgeApp) updateBetaChanged() {
	a.persistSettings()
	a.updates().IncludeBetaChanged()
}

// openDownload opens the release page in the browser; a failure is logged
// and told to the user.
func (a *bridgeApp) openDownload(c *ui.Context, url string) {
	open := a.openURL
	if open == nil {
		open = shellOpenURL
	}
	if err := open(url); err != nil {
		componentLogger(componentUpdate).Error("open download page failed", "err", err)
		c.Toast(downloadFailedText)
	}
}

// checkForUpdates is the tray menu's entry point: it shows the Settings
// page, scrolled to the Updates group, and starts a manual check.
func (a *bridgeApp) checkForUpdates() {
	a.revealUpdates = true
	a.showSettings()
	a.updates().CheckNow(a.currentTime())
}
