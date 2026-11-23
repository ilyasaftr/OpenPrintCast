package main

import (
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
	bridgenet "github.com/ilyasaftr/OpenPrintCast/internal/net"
)

// Settings page metrics.
const (
	dpiStep       = 10
	dpiFieldWidth = 120
)

// Printing group wording for the supply-level switch.
const (
	readSupplyLevelsLabel  = "Read ink and toner levels"
	readSupplyLevelsDetail = "Ask network printers for their supply levels over SNMP. Needed for ink and toner levels on iPhone and Mac."
)

// Notifications group wording.
const (
	notifyOnFailureLabel  = "Notify when a print job fails"
	notifyOnFailureDetail = "Show a notification when a print job fails. Not shown while sharing is paused."
	// What the row says instead when the system refuses notifications.
	notifyBlockedDetail     = "Windows is blocking notifications for " + appTitle + ". Allow them in Windows Settings > System > Notifications."
	notifyUnsupportedDetail = "Notifications aren't available on this system."
)

// History group wording.
const (
	keepJobHistoryLabel  = "Keep job history"
	keepJobHistoryDetail = "Remember finished jobs after you quit. File and user names stay on this PC."
	keepJobHistoryFooter = "Clear Finished on the Jobs page also removes them from this file."
)

// printingSettingsGroup holds the render settings; each control saves on change.
func (a *bridgeApp) printingSettingsGroup(c *ui.Context) {
	groupHeader(c, "Printing", nil)
	listGroupInset(c, settingInset,
		func() {
			settingRow(c, "Reverse page order", "Print pages last to first, for printers that stack paper face up.", func() {
				a.settingSwitch(c, &a.reversePageOrder, "Reverse page order")
			})
		},
		func() {
			settingRow(c, "Auto-rotate to fit", "Rotate pages to match the paper orientation.", func() {
				a.settingSwitch(c, &a.autoRotateToFit, "Auto-rotate to fit")
			})
		},
		func() {
			settingRow(c, readSupplyLevelsLabel, readSupplyLevelsDetail, func() {
				if ui.Switch(c, &a.cfg.ReadSupplyLevels).Label(readSupplyLevelsLabel).Changed() {
					a.readSupplyLevelsChanged()
				}
			})
		},
		func() {
			settingRow(c, "Maximum quality",
				fmt.Sprintf("From %d to %d DPI. Higher is sharper but slower to print.", core.MinRenderDPI, core.MaxRenderDPI),
				func() { a.qualityControl(c) })
		})
	groupFooter(c, "Changes apply immediately.", c.Theme().TextMuted)
}

// settingSwitch is a switch bound to a render setting that persists on change.
func (a *bridgeApp) settingSwitch(c *ui.Context, value *bool, label string) {
	if ui.Switch(c, value).Label(label).Changed() {
		a.persistSettings()
	}
}

// qualityControl is the DPI number field; out-of-range input is clamped.
func (a *bridgeApp) qualityControl(c *ui.Context) {
	dpi := float64(a.maxRenderDPI)
	if ui.NumberInput(c, &dpi, core.MinRenderDPI, core.MaxRenderDPI, dpiStep).Label("Maximum quality in DPI").Width(dpiFieldWidth).Changed() {
		a.maxRenderDPI = core.ClampRenderDPI(int(math.Round(dpi)))
		a.persistSettings()
	}
	ui.Text(c, "DPI").TextColor(c.Theme().TextMuted)
}

// notificationsGroup holds the failure-notification switch; it saves on change.
func (a *bridgeApp) notificationsGroup(c *ui.Context) {
	groupHeader(c, "Notifications", nil)
	listGroupInset(c, settingInset, func() {
		settingRow(c, notifyOnFailureLabel, notifyDetail(a.notifyHealth), func() {
			if ui.Switch(c, &a.cfg.NotifyOnFailure).Label(notifyOnFailureLabel).Changed() {
				a.notifyOnFailureChanged()
			}
		})
	})
}

// notifyDetail is the explanation under the notification switch: the
// normal one, or what to do when the system refuses notifications. A nil
// reader means nothing is known to be wrong.
func notifyDetail(r notificationStatusReader) string {
	if r == nil {
		return notifyOnFailureDetail
	}
	switch r.Status() {
	case notificationsBlocked:
		return notifyBlockedDetail
	case notificationsUnsupported:
		return notifyUnsupportedDetail
	default:
		return notifyOnFailureDetail
	}
}

// historyGroup holds the Keep job history switch; it saves on change.
func (a *bridgeApp) historyGroup(c *ui.Context) {
	groupHeader(c, "History", nil)
	listGroupInset(c, settingInset, func() {
		settingRow(c, keepJobHistoryLabel, keepJobHistoryDetail, func() {
			if ui.Switch(c, &a.cfg.KeepJobHistory).Label(keepJobHistoryLabel).Changed() {
				a.keepJobHistoryChanged()
			}
		})
	})
	groupFooter(c, keepJobHistoryFooter, c.Theme().TextMuted)
}

// computerGroup shows how other devices reach this PC.
func (a *bridgeApp) computerGroup(c *ui.Context) {
	groupHeader(c, "This Computer", nil)
	address := "Not connected to a network"
	if ip := a.lanIP(); ip != "" {
		address = fmt.Sprintf("%s:%d", ip, bridgenet.DefaultPort)
	}
	listGroupInset(c, settingInset,
		func() { valueRow(c, "Name", a.host) },
		func() { valueRow(c, "Address", address) })
	groupFooter(c, "If a device can't find this PC automatically, add a printer by its address.", c.Theme().TextMuted)
}

// aboutGroup shows the version and where settings and the log are stored.
// The log path is asked on every frame: the current file changes over time.
func (a *bridgeApp) aboutGroup(c *ui.Context) {
	groupHeader(c, "About", nil)
	listGroupInset(c, settingInset,
		func() { valueRow(c, "Version", version) },
		func() { valueRow(c, "Settings file", a.store.Location()) },
		func() { valueRow(c, "Log file", orDefault(a.logger().CurrentPath(), noLogFileText)) })
}
