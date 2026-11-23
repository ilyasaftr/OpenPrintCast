package main

// tabNames are the config values of the tabs, indexed by tab constant. The
// order matches the tab bar and the tab* constants in view.go.
var tabNames = [...]string{tabPrinters: "printers", tabJobs: "jobs", tabSettings: "settings"}

// tabFromName maps a stored tab name to its index; anything unknown (an
// empty or hand-edited value) is the Printers tab.
func tabFromName(name string) int {
	for tab, n := range tabNames {
		if n == name {
			return tab
		}
	}
	return tabPrinters
}

// tabName is the config value of a tab; an out-of-range tab is Printers.
func tabName(tab int) string {
	if tab < 0 || tab >= len(tabNames) {
		return tabNames[tabPrinters]
	}
	return tabNames[tab]
}

// rememberTab saves the selected tab when it differs from the stored one,
// so the window reopens where it was left. The search text is deliberately
// not remembered: a stale filter would hide printers without saying why.
func (a *bridgeApp) rememberTab() {
	if a.tab == tabFromName(a.cfg.LastTab) {
		return
	}
	a.cfg.LastTab = tabName(a.tab)
	a.saveConfig("save tab")
}
