package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// Updates group wording and metrics. The Check Now button keeps one width
// whether it says Check Now or Checking…, so the row never shifts.
const (
	updatesGroupTitle    = "Updates"
	softwareUpdateTitle  = "Software update"
	currentVersionFormat = "Current version %s"

	checkNowLabel     = "Check Now"
	checkNowBusyLabel = "Checking…"
	checkNowBusyName  = "Checking for updates"
	checkNowTip       = "Look for a new version now"
	checkNowWidth     = 136

	downloadLabel = "Download…"
	downloadTip   = "Open the download page in your browser"
	skipLabel     = "Skip This Version"
	skipTip       = "Stop offering this version. A newer version is still offered."

	autoUpdateLabel   = "Check for updates automatically"
	autoUpdateDetail  = "Look for a new version about once a day while the app runs."
	includeBetaLabel  = "Include beta versions"
	includeBetaDetail = "Try new versions before they are released."
	updatesFooter     = "Checks for new releases on the feed this build was set up with. Only your IP address and the app version are sent. Updates are downloaded and installed by you."

	// updateBadgeName is what the Settings tab's mark counts.
	updateBadgeName = "update available"
)

// updatesGroup holds the Software update row and the two switches. The
// switches save on change; the row follows the update service.
func (a *bridgeApp) updatesGroup(c *ui.Context) {
	a.updateTick(c)
	status := a.updates().Status()
	groupHeader(c, updatesGroupTitle, nil)
	listGroupInset(c, settingInset,
		func() { a.softwareUpdateRow(c, status) },
		func() {
			settingRow(c, autoUpdateLabel, autoUpdateDetail, func() {
				if ui.Switch(c, &a.cfg.AutoUpdateCheck).Label(autoUpdateLabel).Changed() {
					a.autoUpdateCheckChanged()
				}
			})
		},
		func() {
			settingRow(c, includeBetaLabel, includeBetaDetail, func() {
				if ui.Switch(c, &a.cfg.UpdateBeta).Label(includeBetaLabel).Changed() {
					a.updateBetaChanged()
				}
			})
		})
	groupFooter(c, updatesFooter, c.Theme().TextMuted)
}

// updateTick moves the update state to this frame's time, tells screen
// readers about a change, and schedules the frame that reveals a held-back
// result or redraws the relative time.
func (a *bridgeApp) updateTick(c *ui.Context) {
	tick := a.updates().Tick(c.Now())
	if tick.Announcement != "" {
		c.Announce(tick.Announcement)
	}
	if tick.Wake > 0 {
		c.After(tick.Wake)
	}
}

// softwareUpdateRow is the row that says whether an update exists, with the
// running version beneath and the way to act at the trailing edge.
func (a *bridgeApp) softwareUpdateRow(c *ui.Context, status updateStatus) {
	settingRowDetails(c, softwareUpdateTitle,
		func() { a.updateDetails(c, status) },
		func() { a.updateControls(c, status) })
}

// updateDetails is the two lines under the row title: the status sentence
// (with an alert symbol for a failure, so it is not told by color alone)
// and the version that is running.
func (a *bridgeApp) updateDetails(c *ui.Context, status updateStatus) {
	t := c.Theme()
	color := t.TextMuted
	if status.Phase == updateFailed {
		color = t.Danger
	}
	text := updateStatusLine(status, a.currentTime(), newCLDRDates(c.Locale(), c.Region()))
	ui.Row(c).Gap(statusCellGap).Children(func() {
		if status.Phase == updateFailed {
			ui.Icon(c, alertIcon).FontSize(captionFontSize).TextColor(color).Shrink(0)
		}
		captionText(c, text, color).Shrink(1).MinWidth(0)
	})
	captionText(c, fmt.Sprintf(currentVersionFormat, version), t.TextMuted).SingleLine()
}

// updateStatusLine is the status sentence, followed by when the last check
// finished where that is still true of the status.
func updateStatusLine(status updateStatus, now time.Time, f relativeFormatter) string {
	var parts []string
	if text := status.Text(); text != "" {
		parts = append(parts, text)
	}
	switch status.Phase {
	case updateIdle, updateUpToDate, updateAvailable:
		if last := lastCheckedText(now, status.CheckedAt, f); last != "" {
			parts = append(parts, last+".")
		}
	}
	return strings.Join(parts, " ")
}

// updateControls are the buttons of the row: Skip This Version and
// Download… while an update with a link is on offer, else Check Now.
func (a *bridgeApp) updateControls(c *ui.Context, status updateStatus) {
	if status.HasUpdate() && !status.Skipped {
		a.skipButton(c)
	}
	if status.CanDownload() {
		a.downloadButton(c, status)
		return
	}
	a.checkNowButton(c, status)
}

// downloadButton opens the release page of the update on offer.
func (a *bridgeApp) downloadButton(c *ui.Context, status updateStatus) {
	if ui.PrimaryButton(c, downloadLabel).Tooltip(downloadTip).Clicked() {
		a.openDownload(c, status.Release.URL)
	}
}

// skipButton is the borderless button that stops offering this version.
func (a *bridgeApp) skipButton(c *ui.Context) {
	clicked := false
	ui.Toolbar(c, func() {
		clicked = ui.Button(c, skipLabel).Tooltip(skipTip).Clicked()
	}).Shrink(0)
	if clicked {
		a.updates().Skip()
	}
}

// checkNowButton is the Check Now button. It reacts at once to a click:
// while a check runs it shows a spinner and "Checking…" in the same box,
// tells assistive technology it is busy and unavailable (keeping the
// keyboard focus where it was), and ignores further clicks. Where the build
// cannot check it stays reachable, unavailable, with the reason as its
// tooltip and so as its accessible description.
func (a *bridgeApp) checkNowButton(c *ui.Context, status updateStatus) {
	busy, reason := status.Busy(), status.Unavailable()
	label, name, tip := checkNowLabel, checkNowLabel, checkNowTip
	switch {
	case busy:
		label, name, tip = checkNowBusyLabel, checkNowBusyName, updateCheckingText
	case reason != "":
		tip = reason
	}
	button := ui.Button(c, "").Label(name).Tooltip(tip).Width(checkNowWidth).Busy(busy).AccessDisabled(busy || reason != "").Children(func() {
		if busy {
			ui.Spinner(c).Size(scanIconSize, scanIconSize).Label(name)
		}
		ui.Text(c, label).SingleLine().TextColor(scanLabelColor(c.Theme(), busy || reason != ""))
	})
	if button.Clicked() && status.Checkable() {
		a.updates().CheckNow(c.Now())
	}
}
