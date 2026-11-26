package main

import (
	"github.com/egoist/mygo/ui"
)

// Settings navigation metrics: the sidebar is a narrow list at the leading
// edge, as in System Settings, with the chosen section's groups beside it.
const (
	settingsSidebarWidth = 176
	settingsSidebarIcon  = 16
)

// Section IDs; they are also the config values of the remembered section.
const (
	sectionGeneral  = "general"
	sectionPrinting = "printing"
	sectionNetwork  = "network"
	sectionUpdates  = "updates"
	sectionAbout    = "about"
)

// settingsSection is one entry of the Settings sidebar and the groups of
// its pane, in the order they are drawn.
type settingsSection struct {
	id     string
	label  string
	icon   *ui.SVG
	groups []func(*ui.Context)
}

// settingsSections lists the sections in sidebar order.
func (a *bridgeApp) settingsSections() []settingsSection {
	return []settingsSection{
		{sectionGeneral, "General", slidersIcon, []func(*ui.Context){a.startupGroup, a.notificationsGroup, a.historyGroup}},
		{sectionPrinting, "Printing", printerIcon, []func(*ui.Context){a.printingSettingsGroup}},
		{sectionNetwork, "Network", globeIcon, []func(*ui.Context){a.computerGroup, a.networkGroup}},
		{sectionUpdates, "Updates", downloadIcon, []func(*ui.Context){a.updatesGroup}},
		{sectionAbout, "About", infoIcon, []func(*ui.Context){a.aboutGroup, a.diagnosticsGroup}},
	}
}

// sectionIDOrDefault is id when it names a section, else the first one, so
// an empty or hand-edited stored value still opens a page.
func sectionIDOrDefault(id string) string {
	switch id {
	case sectionGeneral, sectionPrinting, sectionNetwork, sectionUpdates, sectionAbout:
		return id
	}
	return sectionGeneral
}

// viewSettings is the sidebar of sections beside the pane of the chosen
// one. Every change applies and saves at once, with no Apply button.
func (a *bridgeApp) viewSettings(c *ui.Context) {
	if a.revealUpdates {
		a.revealUpdates, a.settingsSection = false, sectionUpdates
	}
	sections := a.settingsSections()
	a.settingsSection = sectionIDOrDefault(a.settingsSection)
	ui.Row(c).Grow(1).MinHeight(0).Gap(sectionGap).AlignItems(ui.Stretch).Children(func() {
		a.settingsSidebar(c, sections)
		a.settingsPane(c, sections)
	})
	a.rememberSettingsSection()
}

// settingsSidebar is the list of sections; the Updates one carries the same
// mark as the Settings tab while an update is on offer.
func (a *bridgeApp) settingsSidebar(c *ui.Context, sections []settingsSection) {
	offer := a.updates().Status().Notable()
	ui.Sidebar(c, &a.settingsSection, func() {
		for _, s := range sections {
			a.settingsSidebarItem(c, s, offer && s.id == sectionUpdates)
		}
	}).Width(settingsSidebarWidth).Shrink(0).Label("Settings sections")
}

func (a *bridgeApp) settingsSidebarItem(c *ui.Context, s settingsSection, marked bool) {
	item := ui.SidebarItem(c, s.id, s.icon, s.label)
	if !marked {
		return
	}
	item.Label(tabAccessibleName(tabItem{label: s.label, badge: 1, badgeName: updateBadgeName})).Children(func() {
		countBadge(c, 1)
	})
}

// settingsPane shows the groups of the chosen section in a column at a
// readable width beside the sidebar, scrolling only when they do not fit.
// The automatic margins center the column in the pane: on a wide or
// maximized window the settings sit in the middle at their readable width
// rather than clinging to the sidebar with blank space beside them.
func (a *bridgeApp) settingsPane(c *ui.Context, sections []settingsSection) {
	ui.Scroll(c).Grow(1).MinWidth(0).Children(func() {
		ui.Column(c.Key("settings-section-"+a.settingsSection)).MaxWidth(settingsWidth).WidthPercent(100).AlignSelf(ui.Stretch).Margin(0, ui.Auto).Gap(controlGap).Children(func() {
			for _, s := range sections {
				if s.id != a.settingsSection {
					continue
				}
				for i, group := range s.groups {
					if i > 0 {
						groupSpacer(c)
					}
					group(c)
				}
			}
		})
	})
}

// rememberSettingsSection saves the chosen section when it differs from the
// stored one, so the page reopens where it was left.
func (a *bridgeApp) rememberSettingsSection() {
	if a.settingsSection == a.cfg.LastSettingsSection {
		return
	}
	a.cfg.LastSettingsSection = a.settingsSection
	a.saveConfig("save settings section")
}
