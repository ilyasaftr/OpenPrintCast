package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// deviceSupplyReader is the backend capability of reading supply levels;
// the production backend implements it behind the small Backend interface,
// so the row reaches it by assertion — a backend without it shows nothing.
type deviceSupplyReader interface {
	SupplyLevels(printer string) ([]core.Supply, string)
}

// Ink strip metrics. The strip follows the print-dialog pattern: one small
// level bar per cartridge, in the cartridge's own color, glanceable
// without words. Bars carry their label and percent as accessibility text.
const (
	supplyStripBarWidth  = 28
	supplyStripBarHeight = 5
	supplyStripGap       = 5
	supplyTrackAlpha     = 56 // track transparency over the muted text color
)

// supplyInkColors maps the standard colorant names to the colors users
// know from their cartridges; a supply without one draws in the accent.
var supplyInkColors = map[string]ui.Color{
	"cyan":          ui.RGB(0, 174, 239),
	"magenta":       ui.RGB(236, 0, 140),
	"yellow":        ui.RGB(255, 200, 0),
	"black":         ui.RGB(60, 60, 60),
	"light-cyan":    ui.RGB(128, 214, 245),
	"light-magenta": ui.RGB(245, 128, 197),
}

// supplyStripNote is the ink strip of a printer row, or nil when the
// printer has no readings: rows of unread printers stay quiet rather than
// claim anything, and receptacles stay out — the strip answers "how much
// ink" at a glance, and the waste container's meaning is the opposite
// direction, which a bar cannot tell without words.
func (a *bridgeApp) supplyStripNote(c *ui.Context, p *core.PrinterInfo) func() {
	reader, ok := a.backend.(deviceSupplyReader)
	if !ok {
		return nil
	}
	supplies, _ := reader.SupplyLevels(p.WindowsName)
	var inks []core.Supply
	for _, supply := range supplies {
		if !supply.Receptacle && (supply.Kind == core.SupplyInk || supply.Kind == core.SupplyToner) {
			inks = append(inks, supply)
		}
	}
	if len(inks) == 0 {
		return nil
	}
	return func() {
		ui.Row(c).Gap(supplyStripGap).Margin(2, 0, 0, 0).Children(func() {
			for _, supply := range inks {
				supplyStripBar(c, supply)
			}
		})
	}
}

// supplyStripBar is one cartridge's bar: the level filling its track in
// the cartridge's color. Levels that are not a number draw an empty track.
func supplyStripBar(c *ui.Context, supply core.Supply) {
	label := supply.Label()
	if label == "" {
		label = string(supply.Kind)
	}
	percent, known := supply.Level.Percent()
	accessLabel := label
	if known {
		accessLabel = fmt.Sprintf("%s %d%%", label, percent)
	} else {
		accessLabel = label + " unknown"
	}
	ui.Box(c.Key("supply-" + label)).Height(supplyStripBarHeight).Width(supplyStripBarWidth).
		Label(accessLabel).DrawOver(func(painter *ui.Painter, r ui.Rect) {
		track := c.Theme().TextMuted
		track.A = supplyTrackAlpha
		painter.Fill(r, track, supplyStripBarHeight/2)
		if known && percent > 0 {
			width := r.W * float32(percent) / 100
			if width > r.W {
				width = r.W
			}
			painter.Fill(ui.Rect{X: r.X, Y: r.Y, W: width, H: r.H}, supplyStripColor(c, supply), supplyStripBarHeight/2)
		}
	})
}

// supplyStripColor picks the bar color: the cartridge color, the accent
// for a supply without one.
func supplyStripColor(c *ui.Context, supply core.Supply) ui.Color {
	if supply.Receptacle {
		return c.Theme().TextMuted
	}
	if color, ok := supplyInkColors[supply.Color]; ok {
		return color
	}
	return c.Theme().Accent
}

// deviceFactsReader reads what the device says about itself, behind the
// same assertion as the supplies.
type deviceFactsReader interface {
	DeviceFacts(printer string) (serial, firmware string, stateReasons []string, totalPages int, pagesKnown bool)
}

// supplyRowBarHeight shapes the Supply Levels tab's full-size bars.
const supplyRowBarHeight = 6

// sheetSupplies is the "Supply Levels" tab of the printer sheet, after the
// macOS panel of the same name: one full-width bar per supply, in the
// supply's own color, with the percent in words — and the device facts
// below, small. A printer that never answers says so in one line instead
// of pretending anything.
func (a *bridgeApp) sheetSupplies(c *ui.Context, p *core.PrinterInfo) {
	t := c.Theme()
	var supplies []core.Supply
	if reader, ok := a.backend.(deviceSupplyReader); ok {
		supplies, _ = reader.SupplyLevels(p.WindowsName)
	}
	facts := ""
	if reader, ok := a.backend.(deviceFactsReader); ok {
		serial, firmware, _, totalPages, pagesKnown := reader.DeviceFacts(p.WindowsName)
		facts = deviceFactsLine(serial, firmware, totalPages, pagesKnown)
	}
	if len(supplies) == 0 {
		captionText(c, "No readings yet. Levels appear here when the printer answers.", t.TextMuted)
		if facts != "" {
			captionText(c, facts, t.TextMuted).Margin(controlGap, 0, 0, 0)
		}
		return
	}
	ui.Column(c).Gap(controlGap).Children(func() {
		for _, supply := range supplies {
			supplyRow(c, supply)
		}
	})
	if facts != "" {
		captionText(c, facts, t.TextMuted).Margin(controlGap, 0, 0, 0)
	}
}

// supplyRow is one supply of the tab: its label and percent over a
// full-width level bar in the supply's own color. Receptacles fill up
// instead of emptying, which the percent text says.
func supplyRow(c *ui.Context, supply core.Supply) {
	t := c.Theme()
	label := supply.Label()
	if label == "" {
		label = string(supply.Kind)
	}
	percentText, percent := supplyLevelText(supply)
	ui.Column(c).Gap(2).Children(func() {
		ui.Row(c).Children(func() {
			captionText(c, label, t.Text).Grow(1).SingleLine()
			captionText(c, percentText, t.TextMuted).SingleLine()
		})
		ui.Box(c.Key("supply-row-" + label)).Height(supplyRowBarHeight).Grow(1).DrawOver(func(painter *ui.Painter, r ui.Rect) {
			track := t.TextMuted
			track.A = supplyTrackAlpha
			painter.Fill(r, track, supplyRowBarHeight/2)
			if percent > 0 {
				width := r.W * float32(percent) / 100
				if width > r.W {
					width = r.W
				}
				painter.Fill(ui.Rect{X: r.X, Y: r.Y, W: width, H: r.H}, supplyStripColor(c, supply), supplyRowBarHeight/2)
			}
		})
	})
}

// supplyLevelText renders a level as words; the percent is -1 for levels
// that are not a number, so the bar stays empty.
func supplyLevelText(supply core.Supply) (text string, percent int) {
	percent, ok := supply.Level.Percent()
	if !ok {
		return "Unknown", -1
	}
	if supply.Receptacle {
		return fmt.Sprintf("%d%% full", percent), percent
	}
	return fmt.Sprintf("%d%%", percent), percent
}

// deviceFactsLine joins the facts worth one line each, skipping the ones
// the device did not tell.
func deviceFactsLine(serial, firmware string, totalPages int, pagesKnown bool) string {
	line := ""
	if serial != "" {
		line = "Serial number: " + serial
	}
	if firmware != "" {
		if line != "" {
			line += "\n"
		}
		line += "Firmware: " + firmware
	}
	if pagesKnown {
		if line != "" {
			line += "\n"
		}
		line += fmt.Sprintf("Pages printed: %d", totalPages)
	}
	return line
}
