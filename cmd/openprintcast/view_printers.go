package main

import (
	"fmt"
	"path/filepath"

	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
	bridgenet "github.com/ilyasaftr/OpenPrintCast/internal/net"
)

// widePrintersWidth is the window width from which the toolbar shows the
// Scan Again caption next to its icon.
const widePrintersWidth = 740

const scanIconSize = 16

// minControlHeight is the HIG default control size on the desktop (28 pt):
// the checkbox's clickable box, 17 pt tall by itself, is raised to it.
const minControlHeight = 28

// Search field wording.
const (
	searchPlaceholder = "Search printers"
	searchTip         = "Search by name or location (Ctrl+F)"
)

// folderBehaviorCaption says what Print to Folder saves; it needs no
// Windows printer, so it is shown whether or not a folder is chosen.
const folderBehaviorCaption = "Saved as PDF files in the folder you choose. Only page ranges are applied; color, copies and orientation are not."

// Printer status cell metrics: the cell has a fixed width per layout, so
// the info button and the switch keep one trailing edge in every row.
const (
	printerStatusIconSize     = 16
	printerStatusWideWidth    = 148
	printerStatusCompactWidth = printerStatusIconSize
)

// printerTile is the leading icon of a printer: a gray document for
// software printers, the accent-colored printer otherwise.
func printerTile(c *ui.Context, p *core.PrinterInfo) {
	if p.IsVirtual {
		iconTile(c, documentIcon, tileGray)
	} else {
		iconTile(c, printerIcon, c.Theme().Accent)
	}
}

// viewPrinters follows the System Settings pattern: a toolbar for finding
// and scanning, then inset grouped lists whose rows carry the switch at
// the trailing edge. The switch needs no On/Off caption: the row names what
// it controls and its position shows the state.
func (a *bridgeApp) viewPrinters(c *ui.Context) {
	t := c.Theme()
	a.scanTick(c)
	ui.Row(c).Gap(sectionGap).Children(func() { a.printersToolbar(c) })
	a.viewNetworkBanner(c)
	ui.Scroll(c).Grow(1).Gap(controlGap).Children(func() {
		a.printerGroup(c)
		groupFooter(c, connectionLine(a.host, a.lanIP(), bridgenet.DefaultPort), t.TextMuted)
		groupSpacer(c)
		groupHeader(c, "Destinations", nil)
		a.folderGroup(c)
	})
}

// printersToolbar holds the search field, the virtual-printer filter and
// the rescan button. Ctrl+F moves the keyboard to the search field.
func (a *bridgeApp) printersToolbar(c *ui.Context) {
	width, _ := c.Size()
	wide := width >= widePrintersWidth
	if c.Shortcut(ui.Cmd, ui.KeyF) {
		a.printerSearch.SelectAll()
	}
	ui.SearchField(c.Key("printer-search"), &a.printerQuery).Placeholder(searchPlaceholder).Label("Find a printer").
		Tooltip(searchTip).KeyShortcuts(ui.Cmd, ui.KeyF).Bind(&a.printerSearch).Grow(1)
	if ui.Checkbox(c.Key("show-virtual-printers"), &a.cfg.ShowVirtualPrinters, "Show virtual printers").MinHeight(minControlHeight).
		Tooltip("Include software printers such as Microsoft Print to PDF").Changed() {
		a.syncTray()
	}
	a.scanButton(c, wide)
}

// printerGroup is the "Printers" list: one row per printer matching the
// search, or a message explaining why there are none.
func (a *bridgeApp) printerGroup(c *ui.Context) {
	t := c.Theme()
	shown, visible := a.filterPrinters()
	groupHeader(c, "Printers", func() {
		ui.Row(c).Gap(sectionGap).Children(func() {
			a.scanStatusCaption(c)
			captionText(c, sharedSummary(a.sharedListedCount(), visible), t.TextMuted).SingleLine().Shrink(0)
		})
	})
	var rows []func()
	for _, p := range shown {
		rows = append(rows, func() { a.printerRow(c, p) })
	}
	if len(rows) == 0 {
		msg := emptyPrintersMessage(visible)
		rows = append(rows, func() { ui.Text(c, msg).TextColor(t.TextMuted).Padding(rowPaddingH) })
	}
	listGroup(c, rows...)
}

// filterPrinters returns the printers matching the search query and how
// many printers are visible at all, ignoring the query.
func (a *bridgeApp) filterPrinters() (shown []*core.PrinterInfo, visible int) {
	for i := range a.printers {
		p := &a.printers[i]
		if !a.visiblePrinter(*p) {
			continue
		}
		visible++
		if matchesPrinter(*p, a.printerQuery) {
			shown = append(shown, p)
		}
	}
	return shown, visible
}

// sharedSummary is the "1 of 2 shared" caption of the printers group.
func sharedSummary(shared, total int) string {
	return fmt.Sprintf("%d of %d shared", shared, total)
}

// emptyPrintersMessage explains an empty printer list: either nothing
// matched the search, or there is nothing to search.
func emptyPrintersMessage(visible int) string {
	if visible == 0 {
		return "No printers found. Add a printer in Windows Settings, or show virtual printers."
	}
	return "No printers match your search. Try another name or location."
}

// printerRow is one printer: tile, name and where it appears, an info
// button for name and location, and the share switch at the trailing edge.
func (a *bridgeApp) printerRow(c *ui.Context, p *core.PrinterInfo) {
	detail, detailColor := a.printerDetail(c.Theme(), p)
	ui.Column(c.Key(p.WindowsName)).Children(func() {
		listRowWithNote(c, func() { printerTile(c, p) }, func() { printerTitle(c, p) }, detail, detailColor, a.compatibilityNote(c, p), func() {
			a.printerStatusCell(c, p)
			if infoButton(c, "Name and location for "+p.WindowsName, "Name and location") {
				a.openPrinterSheet(p)
			}
			if ui.Switch(c, &p.Enabled).Label("Share " + p.WindowsName).Changed() {
				a.printersChanged()
			}
		})
	})
}

// printerStatusCell shows the printer's cached condition as symbol and
// label, or the symbol alone in a compact window. It never queries the
// backend: the status monitor refreshes the cache off the UI thread.
func (a *bridgeApp) printerStatusCell(c *ui.Context, p *core.PrinterInfo) {
	cond, _ := a.printerStatus.Condition(p.WindowsName)
	view := describeCondition(cond)
	width, _ := c.Size()
	wide := width >= widePrintersWidth
	cellWidth := float32(printerStatusCompactWidth)
	if wide {
		cellWidth = printerStatusWideWidth
	}
	ui.Row(c).Gap(statusCellGap).Width(cellWidth).Shrink(0).Children(func() {
		ui.Icon(c, conditionSymbol(view.icon)).FontSize(printerStatusIconSize).TextColor(levelColor(c.Theme(), view.level))
		if wide {
			ui.Text(c, view.label).SingleLine().MinWidth(0)
		}
	}).Label("Status of " + p.WindowsName + ": " + view.label).Tooltip(view.tip)
}

// printerTitle is the printer's Windows name, badged when it is virtual.
func printerTitle(c *ui.Context, p *core.PrinterInfo) {
	ui.Row(c).Gap(controlGap).Children(func() {
		ui.Text(c, p.WindowsName).Bold().SingleLine().MinWidth(0)
		if p.IsVirtual {
			ui.Badge(c, "Virtual").Tooltip("A software printer (for example PDF), not a physical one")
		}
	})
}

// printerDetail is the secondary line of a printer row and its color: how
// the printer appears on devices, or a warning for a hidden shared one.
func (a *bridgeApp) printerDetail(t *ui.Theme, p *core.PrinterInfo) (string, ui.Color) {
	if p.IsVirtual && p.Enabled && !a.cfg.ShowVirtualPrinters {
		return "Previously shared virtual printer; turn off sharing to hide it.", t.Warning
	}
	detail := "Appears as “" + p.DisplayName() + "”"
	if p.Location != "" {
		detail += "  ·  " + p.Location
	}
	return detail, t.TextMuted
}

// folderGroup is the "Print to Folder" destination: a switch row and a
// folder chooser row, plus a footer that says what is saved and, while no
// folder is chosen, what to do first.
func (a *bridgeApp) folderGroup(c *ui.Context) {
	listGroup(c, func() { a.folderSwitchRow(c) }, func() { a.folderChooserRow(c) })
	t := c.Theme()
	if !filepath.IsAbs(a.cfg.FolderPath) {
		groupFooter(c, "Choose a folder to enable sharing.", t.TextMuted)
	}
	groupFooter(c, folderBehaviorCaption, t.TextMuted)
}

// folderSwitchRow is the Print to Folder row with its share switch.
func (a *bridgeApp) folderSwitchRow(c *ui.Context) {
	t := c.Theme()
	_, available := a.folderPrinter()
	detail, detailColor := "Save print jobs as PDF files on this PC.", t.TextMuted
	if a.cfg.FolderEnabled && !available {
		detail, detailColor = "Needs setup", t.Warning
	}
	listRow(c, func() { iconTile(c, folderIcon, tileOrange) },
		func() { ui.Text(c, "Print to Folder").Bold().SingleLine() }, detail, detailColor, func() {
			disabled := !a.cfg.FolderEnabled && !filepath.IsAbs(a.cfg.FolderPath)
			if ui.Switch(c.Key("share-print-to-folder"), &a.cfg.FolderEnabled).Label("Share Print to Folder").Disabled(disabled).Changed() {
				a.folderSettingsChanged()
			}
		})
}

// folderChooserRow shows the chosen folder, aligned under the row titles,
// with the button that picks another one.
func (a *bridgeApp) folderChooserRow(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Gap(rowGap).Padding(rowPaddingV, rowPaddingH, rowPaddingV, rowPaddingH+tileSize+rowGap).MinHeight(settingRowMinHeight).Children(func() {
		ui.Text(c, "Folder").SingleLine()
		path := a.cfg.FolderPath
		if path == "" {
			path = "None selected"
		}
		ui.Text(c, path).TextColor(t.TextMuted).Grow(1).MinWidth(0).SingleLine().TextAlign(ui.End)
		if ui.Button(c, "Choose Folder…").Clicked() {
			a.chooseFolder()
		}
	})
}
