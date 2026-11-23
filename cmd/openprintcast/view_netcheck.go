package main

import (
	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/reach"
)

// Banner metrics: the text is indented by the symbol's width and gap, so
// the buttons line up under it.
const (
	netBannerIconSize = 20
	netSheetWidth     = 440
)

// Confirmation sheet for allowing Public networks.
const (
	netSheetTitle  = "Allow OpenPrintCast on this network?"
	netSheetBody   = "Public networks are shared with strangers. Anyone on this network could reach OpenPrintCast."
	netSheetDetail = "Windows applies this to every Public network on this PC. Only devices on the same local network can connect."
	netSheetAllow  = "Allow"
)

// viewNetworkBanner is the banner of a reachability problem, between the
// toolbar and the lists. It cannot be dismissed while the problem lasts and
// is gone once a check finds it fixed. The text is announced once to screen
// readers when it appears.
func (a *bridgeApp) viewNetworkBanner(c *ui.Context) {
	m := a.network()
	b, ok := describeNetwork(m.state.Result())
	if !m.state.Problem() || !ok {
		return
	}
	a.announceNetworkBanner(c, b)
	t := c.Theme()
	ui.Column(c).Gap(controlGap).Padding(bannerPaddingV, bannerPaddingH).Radius(t.Radius).Background(t.Surface).
		Border(hairline, t.Warning).Children(func() {
		a.bannerText(c, b)
		a.bannerRemedies(c, b)
		a.bannerSecondary(c, b)
	}).Label(b.Headline + ". " + b.Body)
}

// announceNetworkBanner tells screen readers about the banner once per
// appearance of a problem (and again when the problem changes).
func (a *bridgeApp) announceNetworkBanner(c *ui.Context, b networkBanner) {
	m := a.network()
	if key := b.Headline + b.Body; m.announced != key {
		m.announced = key
		c.Announce(b.Headline + ". " + b.Body)
	}
}

// bannerText is the warning symbol with the headline and the sentence.
func (a *bridgeApp) bannerText(c *ui.Context, b networkBanner) {
	t := c.Theme()
	ui.Row(c).Gap(bannerGap).Children(func() {
		ui.Icon(c, alertIcon).FontSize(netBannerIconSize).TextColor(t.Warning).Shrink(0)
		ui.Column(c).Gap(captionTextGap).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, b.Headline).Bold()
			ui.Text(c, b.Body)
		})
	})
}

// bannerRemedies are the primary button and the alternative one.
func (a *bridgeApp) bannerRemedies(c *ui.Context, b networkBanner) {
	ui.Row(c).Gap(controlGap).Justify(ui.End).Children(func() {
		if b.Alt != reach.FixNone && a.remedyButton(c, b.Alt, false) {
			a.performRemedy(c, b.Alt)
		}
		if a.remedyButton(c, b.Primary, true) {
			a.performRemedy(c, b.Primary)
		}
	})
}

// remedyLabel names the button of a remedy; no remedy means Check Again.
func remedyLabel(fix reach.Fix) string {
	switch fix {
	case reach.FixOpenNetworkSettings:
		return netOpenSettingsLabel
	case reach.FixAllowFirewall:
		return netAllowLabel
	case reach.FixAllowOnPublic:
		return netAllowPublicLabel
	}
	return netCheckAgainLabel
}

// remedyButton draws the button of a remedy and reports a click. While the
// firewall change or the check is under way it shows that at once and
// ignores clicks, as Scan Again does.
func (a *bridgeApp) remedyButton(c *ui.Context, fix reach.Fix, primary bool) bool {
	busy := a.remedyBusy(fix)
	if primary {
		return ui.PrimaryButton(c, remedyLabel(fix)).Busy(busy).AccessDisabled(busy).Clicked()
	}
	return ui.Button(c, remedyLabel(fix)).Busy(busy).AccessDisabled(busy).Clicked()
}

// remedyBusy reports whether the remedy's work is running.
func (a *bridgeApp) remedyBusy(fix reach.Fix) bool {
	m := a.network()
	switch fix {
	case reach.FixAllowFirewall, reach.FixAllowOnPublic:
		return m.fixing
	case reach.FixNone:
		return m.state.Busy()
	}
	return false
}

// performRemedy carries out what a remedy button asks for.
func (a *bridgeApp) performRemedy(c *ui.Context, fix reach.Fix) {
	m := a.network()
	switch fix {
	case reach.FixOpenNetworkSettings:
		a.openNetworkSettings(c)
	case reach.FixAllowFirewall:
		a.applyFirewallFix(false, false)
	case reach.FixAllowOnPublic:
		m.confirmOpen = true
	default:
		a.requestNetCheck(c.Now(), true)
	}
}

// bannerSecondary is the quiet row under the sentence: the Details
// disclosure, Check Again where it is not the main button, and Copy
// Diagnostics.
func (a *bridgeApp) bannerSecondary(c *ui.Context, b networkBanner) {
	m := a.network()
	ui.Row(c).Gap(controlGap).AlignItems(ui.Start).Margin(0, 0, 0, netBannerIconSize+bannerGap).Children(func() {
		ui.Collapsible(c, netDetailsLabel, &m.detailsOpen, func() { a.bannerDetails(c) }).Grow(1).MinWidth(0)
		if b.Primary != reach.FixNone && a.remedyButton(c, reach.FixNone, false) {
			a.performRemedy(c, reach.FixNone)
		}
		if ui.Button(c, netCopyLabel).Clicked() {
			a.copyDiagnostics(c)
		}
	})
}

// bannerDetails is the diagnostics text under the Details disclosure.
func (a *bridgeApp) bannerDetails(c *ui.Context) {
	ui.Text(c, a.diagnostics()).FontSize(captionFontSize).TextColor(c.Theme().TextMuted).Selectable()
}

// viewNetworkSheet is the confirmation before the firewall is opened for
// Public networks. Allow runs the fix; Cancel, Escape and a click outside
// change nothing.
func (a *bridgeApp) viewNetworkSheet(c *ui.Context) {
	m := a.network()
	if !m.confirmOpen {
		return
	}
	allow := false
	ui.Modal(c, &m.confirmOpen, func() {
		ui.Text(c, netSheetTitle).FontSize(titleFontSize).Bold()
		ui.Text(c, netSheetBody)
		ui.Text(c, netSheetDetail).TextColor(c.Theme().TextMuted)
		ui.Row(c).Gap(controlGap).Justify(ui.End).Margin(sheetButtonsTop, 0, 0, 0).Children(func() {
			if ui.Button(c, "Cancel").Clicked() {
				m.confirmOpen = false
			}
			allow = ui.PrimaryButton(c, netSheetAllow).Clicked()
		})
	}).Width(netSheetWidth).Label(netSheetTitle)
	if allow {
		m.confirmOpen = false
		a.applyFirewallFix(true, false)
	}
}

// networkCheckRow is the Network check row of Settings > Network: the
// result in words, Check Again and Copy Diagnostics.
func (a *bridgeApp) networkCheckRow(c *ui.Context) {
	m := a.network()
	text := networkStatusText(m.state.Busy(), m.state.HasResult(), m.state.Result())
	settingRow(c, netCheckLabel, text, func() {
		if a.remedyButton(c, reach.FixNone, false) {
			a.performRemedy(c, reach.FixNone)
		}
		if ui.Button(c, netCopyLabel).Clicked() {
			a.copyDiagnostics(c)
		}
	})
}
