package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
	"github.com/ilyasaftr/OpenPrintCast/internal/updatecheck"
)

// withVersion runs the test as a build of that version.
func withVersion(t *testing.T, v string) {
	t.Helper()
	previous := version
	version = v
	t.Cleanup(func() { version = previous })
}

// updateViewApp is the Settings page over an update controller whose
// settings are the app's config and whose clock is the rig's.
func updateViewApp(t *testing.T) (*bridgeApp, *updateRig) {
	t.Helper()
	withVersion(t, "v0.2.9-beta.14")
	app := newTestApp(t)
	app.tab, app.settingsSection = tabSettings, sectionUpdates
	app.cfg.UpdateBeta = true // the fixtures' versions are betas
	rig := newUpdateRig(t)
	deps := rig.updateDeps
	deps.settings, deps.redraw = appUpdateSettings{app: app}, app.invalidateWindow
	rig.updateController = newUpdateController(deps)
	app.update = rig.updateController
	app.clock = rig.clock.now
	return app, rig
}

// revealUpdates checks that the Updates pane shows its whole group in a
// window of that height: the group has its own pane, so nothing scrolls.
func revealUpdates(t *testing.T, tt *ui.Tester, windowH float32) {
	t.Helper()
	header, ok := tt.Find(updatesGroupTitle)
	footer, ok2 := tt.Find(updatesFooter)
	if !ok || !ok2 {
		t.Fatalf("the Updates group is missing; texts: %q", tt.Texts())
	}
	if header.Y < 0 || footer.Y > windowH {
		t.Fatalf("the Updates group is out of view: header %v footer %v", header, footer)
	}
}

// snapshotUpdates draws the Updates group in both themes at both window
// sizes, as name-<size>-<theme>.png.
func snapshotUpdates(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	for _, size := range [][2]int{{860, 620}, {640, 460}} {
		for _, dark := range []bool{false, true} {
			tt.SetSize(size[0], size[1])
			tt.SetDark(dark)
			revealUpdates(t, tt, float32(size[1]))
			theme := map[bool]string{false: "light", true: "dark"}[dark]
			snapshot(t, tt, fmt.Sprintf("updates-%s-%d-%s", name, size[0], theme))
		}
	}
	tt.SetSize(860, 620)
	tt.SetDark(false)
}

// finishCheck answers the held fetch and lets the minimum busy time pass,
// as the frames of the window would.
func finishCheck(tt *ui.Tester, rig *updateRig) {
	rig.held.finish()
	tt.Frame()
	tt.Advance(updateMinBusy)
}

func button(t *testing.T, tt *ui.Tester, name string) ui.AccessInfo {
	t.Helper()
	info, ok := tt.AccessByRole(ui.RoleButton, name)
	if !ok {
		t.Fatalf("no button %q; texts: %q", name, tt.Texts())
	}
	return info
}

func hasButton(tt *ui.Tester, name string) bool {
	_, ok := tt.AccessByRole(ui.RoleButton, name)
	return ok
}

func clickButton(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	info := button(t, tt, name)
	tt.ClickAt(info.Bounds.X+info.Bounds.W/2, info.Bounds.Y+info.Bounds.H/2)
}

func TestUpdatesGroupRowsAndAccessibleLabels(t *testing.T) {
	app, _ := updateViewApp(t)
	tt := ui.NewTester(app.view, 860, 620)
	revealUpdates(t, tt, 620)
	for _, want := range []string{
		"Software update", "Not checked yet.", "Current version v0.2.9-beta.14",
		autoUpdateLabel, autoUpdateDetail, includeBetaLabel, "Try new versions before they are released.",
		"Checks for new releases on the feed this build was set up with. Only your IP address and the app version are sent. Updates are downloaded and installed by you.",
	} {
		if !tt.HasText(want) {
			t.Errorf("missing %q; texts: %q", want, tt.Texts())
		}
	}
	for _, name := range []string{"Check for updates automatically", "Include beta versions"} {
		if sw, ok := tt.AccessByRole(ui.RoleSwitch, name); !ok || !sw.Checked {
			t.Errorf("switch %q: %+v, found %t; both default to on", name, sw, ok)
		}
	}
	if b := button(t, tt, "Check Now"); b.Disabled || b.Busy || b.Description == "" {
		t.Fatalf("idle Check Now = %+v", b)
	}
	snapshotUpdates(t, tt, "idle")
}

func TestUpdatesGroupWhenNotSetUpExplainsAndKeepsSwitchesUsable(t *testing.T) {
	withVersion(t, "v0.2.9-beta.14")
	app := newTestApp(t) // no update service: not set up
	app.tab, app.settingsSection = tabSettings, sectionUpdates
	tt := ui.NewTester(app.view, 860, 620)
	revealUpdates(t, tt, 620)
	if !tt.HasText(updateNotSetUpText) {
		t.Fatalf("no explanation; texts: %q", tt.Texts())
	}
	b := button(t, tt, "Check Now")
	if !b.Disabled || b.Description != updateNotSetUpText || !b.Focusable {
		t.Fatalf("Check Now = %+v, want unavailable, reachable and explained", b)
	}
	tt.ClickAt(b.Bounds.X+b.Bounds.W/2, b.Bounds.Y+b.Bounds.H/2)
	if tt.HasText(updateCheckingText) || button(t, tt, "Check Now").Busy {
		t.Fatal("clicking the unavailable button started something")
	}
	snapshotUpdates(t, tt, "not-set-up")
	toggleSwitch(t, tt, autoUpdateLabel)
	saved, err := core.LoadConfig(app.store.Location())
	if err != nil || saved.AutoUpdateCheck {
		t.Fatalf("the switch must still save while not set up: %+v, %v", saved, err)
	}
}

func TestUpdatesGroupOfADevBuild(t *testing.T) {
	app, rig := updateViewApp(t)
	withVersion(t, "dev")
	rig.state = newUpdateState(updateDevBuild, time.Time{})
	tt := ui.NewTester(app.view, 860, 620)
	revealUpdates(t, tt, 620)
	if !tt.HasText(updateDevBuildText) || !tt.HasText("Current version dev") {
		t.Fatalf("texts: %q", tt.Texts())
	}
	if b := button(t, tt, "Check Now"); !b.Disabled || b.Description != updateDevBuildText {
		t.Fatalf("Check Now = %+v", b)
	}
	snapshotUpdates(t, tt, "dev-build")
}

func TestCheckNowAcknowledgesAtOnceAndIgnoresRepeatClicks(t *testing.T) {
	app, rig := updateViewApp(t)
	tt := ui.NewTester(app.view, 860, 620)
	tt.UseScreenReader()
	revealUpdates(t, tt, 620)
	idle := button(t, tt, "Check Now")
	clickButton(t, tt, "Check Now")
	busy := button(t, tt, "Checking for updates")
	if !busy.Busy || !busy.Disabled || !busy.Focused {
		t.Fatalf("the button after the click = %+v", busy)
	}
	if busy.Bounds != idle.Bounds {
		t.Fatalf("the row shifted: %v -> %v", idle.Bounds, busy.Bounds)
	}
	if !tt.HasText("Checking…") || !tt.HasText(updateCheckingText) || !rig.held.pending() || rig.feed.calls != 0 {
		t.Fatalf("texts: %q", tt.Texts())
	}
	if got := tt.Announcements(); !slices.Contains(got, updateCheckingText) {
		t.Fatalf("announcements = %q", got)
	}
	for i := 0; i < 3; i++ {
		clickButton(t, tt, "Checking for updates")
	}
	rig.held.finish()
	tt.Frame()
	if !button(t, tt, "Checking for updates").Busy {
		t.Fatal("the result showed before the minimum busy time")
	}
	if rig.feed.calls != 1 {
		t.Fatalf("%d fetches for repeated clicks", rig.feed.calls)
	}
	snapshotUpdates(t, tt, "checking")
}

func TestCheckNowResultAppearsAfterTheMinimumBusyTime(t *testing.T) {
	app, rig := updateViewApp(t)
	rig.feed.releases = nil
	tt := ui.NewTester(app.view, 860, 620)
	tt.UseScreenReader()
	revealUpdates(t, tt, 620)
	clickButton(t, tt, "Check Now")
	rig.held.finish()
	tt.Frame()
	tt.Advance(updateMinBusy - time.Millisecond)
	if !button(t, tt, "Checking for updates").Busy {
		t.Fatal("busy ended early")
	}
	tt.Advance(time.Millisecond)
	if b := button(t, tt, "Check Now"); b.Busy || b.Disabled {
		t.Fatalf("Check Now after the result = %+v", b)
	}
	if !tt.HasText("You're up to date. Last checked now.") {
		t.Fatalf("texts: %q", tt.Texts())
	}
	if got := tt.Announcements(); !slices.Contains(got, updateUpToDateText) {
		t.Fatalf("announcements = %q", got)
	}
	app.clock = func() time.Time { return rig.clock.now().Add(5 * time.Minute) }
	tt.Advance(updateRelativeRefresh)
	if !tt.HasText("You're up to date. Last checked 5 minutes ago.") {
		t.Fatalf("the relative time did not follow the clock; texts: %q", tt.Texts())
	}
	snapshotUpdates(t, tt, "up-to-date")
}

func TestUpdateOnOfferShowsDownloadAndSkip(t *testing.T) {
	app, rig := updateViewApp(t)
	opener := &urlRecorder{}
	app.openURL = opener.open
	tt := ui.NewTester(app.view, 860, 620)
	revealUpdates(t, tt, 620)
	clickButton(t, tt, "Check Now")
	finishCheck(tt, rig)
	if !tt.HasText("Version v0.2.9-beta.15 is available. Last checked now.") {
		t.Fatalf("texts: %q", tt.Texts())
	}
	if hasButton(tt, "Check Now") || !hasButton(tt, "Download…") || !hasButton(tt, "Skip This Version") {
		t.Fatalf("buttons while an update is on offer; texts: %q", tt.Texts())
	}
	snapshotUpdates(t, tt, "available")
	clickButton(t, tt, "Download…")
	if !slices.Equal(opener.urls, []string{betaOffer.URL}) {
		t.Fatalf("opened %q", opener.urls)
	}
	if tt.HasText(downloadFailedText) {
		t.Fatal("a toast on success")
	}
}

func TestSkipThisVersionStoresItAndKeepsDownloadOffered(t *testing.T) {
	app, rig := updateViewApp(t)
	tt := ui.NewTester(app.view, 860, 620)
	revealUpdates(t, tt, 620)
	clickButton(t, tt, "Check Now")
	finishCheck(tt, rig)
	if !app.updates().Status().Notable() {
		t.Fatal("setup: the update should be notable")
	}
	clickButton(t, tt, "Skip This Version")
	if app.cfg.SkippedUpdate != betaOffer.Tag {
		t.Fatalf("skipped = %q", app.cfg.SkippedUpdate)
	}
	saved, err := core.LoadConfig(app.store.Location())
	if err != nil || saved.SkippedUpdate != betaOffer.Tag {
		t.Fatalf("not saved: %+v, %v", saved, err)
	}
	if !tt.HasText("Version v0.2.9-beta.15 is available (skipped).") || hasButton(tt, "Skip This Version") || !hasButton(tt, "Download…") {
		t.Fatalf("after skipping; texts: %q", tt.Texts())
	}
	snapshotUpdates(t, tt, "skipped")
}

func TestDownloadFailureLogsAndToasts(t *testing.T) {
	logs := captureLogs(t)
	app, rig := updateViewApp(t)
	opener := &urlRecorder{err: errors.New("no browser")}
	app.openURL = opener.open
	tt := ui.NewTester(app.view, 860, 620)
	revealUpdates(t, tt, 620)
	clickButton(t, tt, "Check Now")
	finishCheck(tt, rig)
	clickButton(t, tt, "Download…")
	if !tt.HasText(downloadFailedText) {
		t.Fatalf("no toast; texts: %q", tt.Texts())
	}
	if !strings.Contains(logs.String(), "open download page failed") {
		t.Fatalf("not logged: %s", logs)
	}
}

func TestUpdateWithoutASafeLinkHidesDownload(t *testing.T) {
	app, rig := updateViewApp(t)
	rig.feed.releases = []updatecheck.Release{{Tag: "v0.2.9-beta.15", Prerelease: true}} // the feed's link was dropped
	tt := ui.NewTester(app.view, 860, 620)
	revealUpdates(t, tt, 620)
	clickButton(t, tt, "Check Now")
	finishCheck(tt, rig)
	if hasButton(tt, "Download…") || !hasButton(tt, "Check Now") || !hasButton(tt, "Skip This Version") {
		t.Fatalf("buttons; texts: %q", tt.Texts())
	}
	if !tt.HasText("Version v0.2.9-beta.15 is available.") {
		t.Fatalf("the update must still be reported; texts: %q", tt.Texts())
	}
}

func TestFailedCheckShowsItsSentenceWithAnAlertAndAllowsRetry(t *testing.T) {
	app, rig := updateViewApp(t)
	rig.feed.err = errOffline
	tt := ui.NewTester(app.view, 860, 620)
	revealUpdates(t, tt, 620)
	clickButton(t, tt, "Check Now")
	finishCheck(tt, rig)
	if !tt.HasText(updateOfflineText) || len(rig.notices.got()) != 0 {
		t.Fatalf("texts: %q", tt.Texts())
	}
	if b := button(t, tt, "Check Now"); b.Disabled || b.Busy {
		t.Fatalf("Check Now after a failure = %+v", b)
	}
	snapshotUpdates(t, tt, "failed")
	rig.feed.err = &updatecheck.RateLimitError{}
	clickButton(t, tt, "Check Now")
	finishCheck(tt, rig)
	if !tt.HasText(updateLimitedText) || tt.HasText(updateOfflineText) {
		t.Fatalf("texts: %q", tt.Texts())
	}
}

func TestUpdateSwitchesPersistAndApplyLive(t *testing.T) {
	app, rig := updateViewApp(t)
	rig.Start(t.Context())
	armed := rig.timers.live()
	tt := ui.NewTester(app.view, 860, 620)
	toggleSwitch(t, tt, autoUpdateLabel)
	toggleSwitch(t, tt, includeBetaLabel)
	saved, err := core.LoadConfig(app.store.Location())
	if err != nil || saved.AutoUpdateCheck || saved.UpdateBeta {
		t.Fatalf("explicit offs were not saved: %+v, %v", saved, err)
	}
	if !armed.canceled || rig.timers.live() != nil {
		t.Fatal("the automatic-checks switch did not stop the schedule")
	}
	toggleSwitch(t, tt, autoUpdateLabel)
	if rig.timers.live() == nil {
		t.Fatal("switching automatic checks back on did not restart the schedule")
	}
}

func TestSettingsTabMarksAnUpdateOnOffer(t *testing.T) {
	app, rig := updateViewApp(t)
	app.tab = tabPrinters
	tt := ui.NewTester(app.view, 860, 620)
	if _, ok := tt.AccessByRole(ui.RoleTab, "Settings"); !ok {
		t.Fatal("a plain Settings tab is missing")
	}
	rig.Start(t.Context())
	rig.runBackgroundCheck(t) // a background check settles without the page being open
	tt.Frame()
	if _, ok := tt.AccessByRole(ui.RoleTab, "Settings, update available"); !ok {
		t.Fatalf("no marked Settings tab; texts: %q", tt.Texts())
	}
	snapshot(t, tt, "updates-tab-badge")
	rig.Skip()
	tt.Frame()
	if _, ok := tt.AccessByRole(ui.RoleTab, "Settings"); !ok {
		t.Fatal("a skipped update must not mark the tab")
	}
}

// urlRecorder is a urlOpener that records the addresses it was given.
type urlRecorder struct {
	urls []string
	err  error
}

func (r *urlRecorder) open(url string) error {
	r.urls = append(r.urls, url)
	return r.err
}

func TestTrayCheckForUpdatesShowsSettingsAndStartsACheck(t *testing.T) {
	app, rig := updateViewApp(t)
	app.tab = tabPrinters
	raised := 0
	app.raiseWindow = func() { raised++ }
	item := itemByLabel(t, trayItems(trayModel{}, app), "Check for Updates…")
	click(item)
	if app.tab != tabSettings || raised != 1 || !app.revealUpdates || !rig.held.pending() {
		t.Fatalf("tab=%d raised=%d reveal=%v pending=%v", app.tab, raised, app.revealUpdates, rig.held.pending())
	}
	tt := ui.NewTester(app.view, 860, 620)
	if app.revealUpdates {
		t.Fatal("the reveal request was not consumed by the page")
	}
	if app.settingsSection != sectionUpdates || !tt.HasText(updatesFooter) {
		t.Fatalf("the Updates section was not shown: %q", app.settingsSection)
	}
}

func TestUpdateNoticeClickShowsSettings(t *testing.T) {
	app, _ := updateViewApp(t)
	app.tab = tabPrinters
	app.raiseWindow = func() {}
	app.showSettings()
	if app.tab != tabSettings {
		t.Fatalf("tab = %d", app.tab)
	}
	app.showJobs()
	if app.tab != tabJobs {
		t.Fatalf("tab = %d", app.tab)
	}
}
