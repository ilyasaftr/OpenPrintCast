package main

import (
	"github.com/egoist/mygo/ui"
)

// appTagline sits under the app name in the header; its tooltip shows it
// whole where the window is too narrow for it.
const appTagline = "Print from mobile devices and Macs to this PC's printers"

// pauseButtonWidth fits the wider of the Pause and Resume labels.
const pauseButtonWidth = 85

// Tab indexes, in the order the tab bar lists them.
const (
	tabPrinters = iota
	tabJobs
	tabSettings
)

// view builds the whole window from live state on the main thread: a
// header that always answers "is it working?", a banner only when
// something needs attention, then the page.
func (a *bridgeApp) view(c *ui.Context) {
	c.SetTheme(accessibleTheme(c.Theme()))
	a.showNotices(c)
	a.netTick(c)
	ui.Column(c).Fill().Padding(pagePadding).Gap(sectionGap).Children(func() {
		a.viewHeader(c)
		a.viewBanner(c)
		a.viewTabs(c)
		switch a.tab {
		case tabJobs:
			a.viewJobs(c)
		case tabSettings:
			a.viewSettings(c)
		default:
			a.viewPrinters(c)
		}
	})
	a.viewPrinterDialog(c)
	a.viewNetworkSheet(c)
	a.rememberTab()
}

// viewHeader shows the app mark, the app name, the live status and the one global
// action. Resume is the primary button while paused, since that is the
// next thing the user wants; Pause stays secondary.
func (a *bridgeApp) viewHeader(c *ui.Context) {
	t := c.Theme()
	width, _ := c.Size()
	ui.Row(c).Gap(sectionGap).Children(func() {
		// The mark only appears where the tagline still fits beside it.
		if width >= headerMarkMinWidth {
			ui.Image(c, appMark).Size(headerMarkSize, headerMarkSize).Shrink(0)
		}
		ui.Column(c).Gap(captionTextGap).Grow(1).Children(func() {
			ui.Text(c, appTitle).FontSize(headlineFontSize).Bold()
			ui.Text(c, appTagline).TextColor(t.TextMuted).SingleLine().Tooltip(appTagline)
		})
		a.statusPill(c)
		a.pauseButton(c)
	})
}

// statusPill is the rounded live-status label in the header.
func (a *bridgeApp) statusPill(c *ui.Context) {
	t := c.Theme()
	text, lvl := a.status()
	ui.Row(c).Gap(pillGap).Padding(pillPaddingV, pillPaddingH).Radius(fullyRounded).Background(t.Surface).Border(hairline, t.Border).Children(func() {
		a.statusMarker(c, lvl)
		ui.Text(c, text).SingleLine()
	}).Label("Status: " + text)
}

// statusMarker is the pill's symbol: a dot, or, while the network check
// has found a problem, a warning symbol, so the state is not told by color
// alone.
func (a *bridgeApp) statusMarker(c *ui.Context, lvl level) {
	if a.netProblem() && lvl == levelWarn {
		ui.Icon(c, alertIcon).FontSize(captionFontSize).TextColor(levelColor(c.Theme(), lvl))
		return
	}
	statusDot(c, lvl)
}

// pauseButton is Resume (primary) while paused, Pause (secondary) otherwise.
// Both have one width, so the status pill beside them does not move.
func (a *bridgeApp) pauseButton(c *ui.Context) {
	if a.paused {
		if ui.PrimaryButton(c, "Resume").Width(pauseButtonWidth).Clicked() {
			a.setPaused(false)
		}
	} else if ui.Button(c, "Pause").Width(pauseButtonWidth).Clicked() {
		a.setPaused(true)
	}
}

// viewBanner explains a status that needs the user's attention, with the
// fix in plain language; it renders nothing when all is well.
func (a *bridgeApp) viewBanner(c *ui.Context) {
	msg, lvl := bannerMessage(a.paused, a.lanIP() != "")
	if msg == "" {
		return
	}
	t := c.Theme()
	ui.Row(c).Gap(bannerGap).Padding(bannerPaddingV, bannerPaddingH).Radius(t.Radius).Background(t.Surface).
		Border(hairline, levelColor(t, lvl)).Children(func() {
		statusDot(c, lvl)
		ui.Text(c, msg).Grow(1)
	})
}
