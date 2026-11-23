package main

import (
	"github.com/egoist/mygo/ui"
)

// Scan Again button wording and metrics. The button keeps one width
// whether it says Scan Again or Scanning…, so the toolbar never shifts.
const (
	scanButtonLabel       = "Scan Again"
	scanButtonBusyLabel   = "Scanning…"
	scanButtonTip         = "Look for printers added to Windows (F5)"
	scanButtonBusyTip     = "Looking for printers added to Windows"
	scanButtonBusyName    = "Scanning for printers"
	scanButtonWideWidth   = 132
	scanButtonNarrowWidth = 46
)

// scanTick moves the scan state to this frame's time, tells screen readers
// about a change, and schedules the frame that reveals a held-back result
// or fades an old one.
func (a *bridgeApp) scanTick(c *ui.Context) {
	scan := a.scanState()
	scan.Advance(c.Now())
	if text := scan.TakeAnnouncement(); text != "" {
		c.Announce(text)
	}
	if wake := scan.NextWake(c.Now()); wake > 0 {
		c.After(wake)
	}
}

// scanRequested reports whether the user asked for a scan by the F5 key or
// Ctrl+R, the shortcuts the button also announces.
func scanRequested(c *ui.Context) bool {
	return c.Shortcut(0, ui.KeyF5) || c.Shortcut(ui.Cmd, ui.KeyR)
}

// scanButton is the Scan Again button. It reacts at once to a click: while
// a scan runs it shows a spinner and "Scanning…" in the same box, tells
// assistive technology it is busy and unavailable (keeping the keyboard
// focus where it was), and ignores further clicks.
func (a *bridgeApp) scanButton(c *ui.Context, wide bool) {
	busy := a.scanState().Busy()
	label, tip, name := scanButtonLabel, scanButtonTip, scanButtonLabel
	if busy {
		label, tip, name = scanButtonBusyLabel, scanButtonBusyTip, scanButtonBusyName
	}
	width := float32(scanButtonNarrowWidth)
	if wide {
		width = scanButtonWideWidth
	}
	button := ui.Button(c, "").Label(name).Tooltip(tip).Width(width).Busy(busy).AccessDisabled(busy).
		KeyShortcuts(0, ui.KeyF5).KeyShortcuts(ui.Cmd, ui.KeyR).Children(func() {
		if busy {
			ui.Spinner(c).Size(scanIconSize, scanIconSize).Label(name)
		} else {
			ui.Icon(c, refreshIcon).FontSize(scanIconSize)
		}
		if wide {
			ui.Text(c, label).SingleLine().TextColor(scanLabelColor(c.Theme(), busy))
		}
	})
	if button.Clicked() || scanRequested(c) {
		a.startScan(c.Now())
	}
}

// scanLabelColor mutes the label while the button is unavailable.
func scanLabelColor(t *ui.Theme, busy bool) ui.Color {
	if busy {
		return t.TextMuted
	}
	return t.Text
}

// scanStatusCaption is the scan's inline status beside the Printers
// heading: what is happening or what the last scan found. A failure also
// carries an alert symbol, so it is not told by color alone.
func (a *bridgeApp) scanStatusCaption(c *ui.Context) {
	status := a.scanState().Status()
	if status.Text == "" {
		return
	}
	t := c.Theme()
	color := t.Text
	switch {
	case status.Failed:
		color = t.Danger
	case status.Busy:
		color = t.TextMuted
	}
	ui.Row(c).Gap(statusCellGap).Shrink(0).Children(func() {
		if status.Failed {
			ui.Icon(c, alertIcon).FontSize(captionFontSize).TextColor(color)
		}
		captionText(c, status.Text, color).SingleLine()
	})
}
