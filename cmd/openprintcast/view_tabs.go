package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"
)

// Tab bar metrics, as the toolkit's own tabs: 8 above and below the
// label, 12 beside it, a 2 high underline on the chosen tab inset by 6.
const (
	tabGap            = 4
	tabPaddingV       = 8
	tabPaddingH       = 12
	tabUnderline      = 2
	tabUnderlineInset = 6
	tabSelectedWeight = 600
	tabBadgeGap       = 6
	badgeSize         = 18
	badgePaddingH     = 5
	badgeFontSize     = 11
	badgeMaxCount     = 99
)

// tabItem is one tab: its label, the number its badge shows (none for 0)
// and what that number counts, as assistive technology says it.
type tabItem struct {
	label     string
	badge     int
	badgeName string
}

// tabAccessibleName is what assistive technology reads for a tab; the badge
// count is part of it, so the number is never visible only.
func tabAccessibleName(item tabItem) string {
	if item.badge <= 0 {
		return item.label
	}
	return item.label + ", " + item.badgeName
}

// badgeText is the count as the badge shows it, capped so it stays small.
func badgeText(count int) string {
	if count > badgeMaxCount {
		return fmt.Sprintf("%d+", badgeMaxCount)
	}
	return fmt.Sprint(count)
}

// appTabs is the tab bar: the toolkit's ui.Tabs look with room for a count
// badge beside a label, which ui.Tabs (labels only) cannot hold. It keeps
// the toolkit's behavior (click, arrows, Home and End, one Tab stop) by
// building on ui.TabsBase.
func appTabs(c *ui.Context, selected *int, items []tabItem) {
	tabs := ui.TabsBase(c, selected, len(items))
	tabs.List.Gap(tabGap).Children(func() {
		for i, item := range items {
			appTab(c, tabs.Tab(i), item, i == *selected)
		}
	})
}

// appTab styles one tab: muted until chosen or hovered, the chosen one in
// full text color, semibold and underlined in the accent.
func appTab(c *ui.Context, tab ui.Element, item tabItem, chosen bool) {
	t := c.Theme()
	tab.Padding(tabPaddingV, tabPaddingH).FocusRing(false).Label(tabAccessibleName(item)).Gap(tabBadgeGap)
	switch {
	case chosen:
		tab.TextColor(t.Text).FontWeight(tabSelectedWeight)
	case tab.Hovered():
		tab.TextColor(t.Text)
	default:
		tab.TextColor(t.TextMuted)
	}
	tab.DrawOver(func(p *ui.Painter, r ui.Rect) {
		if chosen {
			p.Fill(ui.Rect{X: r.X + tabUnderlineInset, Y: r.Y + r.H - tabUnderline, W: r.W - 2*tabUnderlineInset, H: tabUnderline},
				tabUnderlineColor(c, t), tabUnderline/2)
		}
		if tab.FocusVisible() {
			p.FocusRing(r, [4]float32{t.Radius, t.Radius, t.Radius, t.Radius})
		}
	})
	tab.Children(func() {
		ui.Text(c, item.label).SingleLine()
		if item.badge > 0 {
			countBadge(c, item.badge)
		}
	})
}

// tabUnderlineColor is the chosen tab's underline: the accent, faded to
// muted text in a window that is not the active one, as the toolkit's do.
func tabUnderlineColor(c *ui.Context, t *ui.Theme) ui.Color {
	if c.WindowActive() {
		return t.Accent
	}
	return t.TextMuted
}

// countBadge is the red capsule with a number. The number, not the color,
// carries the meaning; the fill only has to stand out.
func countBadge(c *ui.Context, count int) {
	fill, text := badgeColors(c.Theme())
	ui.Box(c).Height(badgeSize).MinWidth(badgeSize).Padding(0, badgePaddingH).Radius(fullyRounded).
		Background(fill).Center().Shrink(0).Children(func() {
		ui.Text(c, badgeText(count)).FontSize(badgeFontSize).Bold().TextColor(text).SingleLine()
	})
}

// tabItems lists the tabs, the Jobs one carrying its unseen failures and
// the Settings one a mark while an update is on offer.
func (a *bridgeApp) tabItems() []tabItem {
	items := []tabItem{{label: "Printers"}, {label: "Jobs"}, {label: "Settings"}}
	if a.failures != nil {
		unseen := a.failures.Unseen()
		items[tabJobs].badge, items[tabJobs].badgeName = unseen, plural(unseen, "new failure")
	}
	if a.updates().Status().Notable() {
		items[tabSettings].badge, items[tabSettings].badgeName = 1, updateBadgeName
	}
	return items
}

// viewTabs shows the tab bar. A failure counts as seen once the Jobs page
// is the one shown, so nothing is unseen there.
func (a *bridgeApp) viewTabs(c *ui.Context) {
	a.markFailuresSeenOnJobsTab()
	appTabs(c, &a.tab, a.tabItems())
	a.markFailuresSeenOnJobsTab()
}

// markFailuresSeenOnJobsTab clears the unseen failures while the Jobs tab
// is chosen.
func (a *bridgeApp) markFailuresSeenOnJobsTab() {
	if a.failures != nil && a.tab == tabJobs {
		a.failures.MarkSeen()
	}
}
