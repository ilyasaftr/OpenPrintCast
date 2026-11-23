package main

import (
	"github.com/egoist/mygo/ui"
)

// Light-theme colors darkened to meet WCAG 2.x AA (4.5:1 for text, 3:1 for
// symbols) on both the window and the grouped-list background. The toolkit's
// own light values fall short: muted text reads 4.40:1 on a list row, warning
// orange 2.90:1 and success green 3.00:1.
var (
	lightTextMuted = ui.Hex("#686870")
	lightWarning   = ui.Hex("#b04f08")
	lightDanger    = ui.Hex("#c62828")
	lightSuccess   = ui.Hex("#15803d")
)

// Dark-theme fill accent. The toolkit paints primary buttons, checked
// switches and checkboxes with Theme.Accent and puts white on it, which on
// the stock #3b82f6 is only 3.68:1. The fill is darkened so white text
// reaches 4.5:1 while it still stands 3:1 clear of the list-row surface
// (switch tracks and checkboxes); the two states are darker still, so white
// keeps its contrast while hovered or pressed. The lighter stock accent
// stays the color of text and icons on dark backgrounds (accentForeground).
var (
	darkFillAccent        = ui.Hex("#2a6af0")
	darkFillAccentHover   = ui.Hex("#2563eb")
	darkFillAccentPressed = ui.Hex("#1d4ed8")
)

// accessibleTheme returns a copy of base whose colors meet AA contrast: the
// light theme's text colors are darkened and the dark theme gets a fill
// accent white text can sit on. A color the desktop's own contrast or
// accent settings changed is left as the user chose it.
func accessibleTheme(base *ui.Theme) *ui.Theme {
	t := *base
	if t.Dark {
		fillDarkAccent(&t)
		return &t
	}
	stock := ui.LightTheme()
	darken := func(current *ui.Color, was, now ui.Color) {
		if *current == was {
			*current = now
		}
	}
	darken(&t.TextMuted, stock.TextMuted, lightTextMuted)
	darken(&t.Warning, stock.Warning, lightWarning)
	darken(&t.Danger, stock.Danger, lightDanger)
	darken(&t.Success, stock.Success, lightSuccess)
	return &t
}

// fillDarkAccent swaps the stock dark accent and its hover and pressed
// states for the fill accent; any other accent (the desktop's own) stays.
func fillDarkAccent(t *ui.Theme) {
	stock := ui.DarkTheme()
	if t.Accent != stock.Accent {
		return
	}
	t.Accent = darkFillAccent
	if t.AccentHover == stock.AccentHover {
		t.AccentHover = darkFillAccentHover
	}
	if t.AccentPressed == stock.AccentPressed {
		t.AccentPressed = darkFillAccentPressed
	}
}

// accentForeground is the accent for text, links and icons drawn on the
// window or a list row, as opposed to Theme.Accent, which fills controls.
// In the dark theme it is the stock accent the fill replaced.
func accentForeground(t *ui.Theme) ui.Color {
	if t.Dark && t.Accent == darkFillAccent {
		return ui.DarkTheme().Accent
	}
	return t.Accent
}

// Badge colors. The badge is a small red capsule whose number is the signal;
// its fill still has to carry the number legibly: white on it reaches
// 4.5:1. The light theme's danger color already does. The dark theme's
// (#ef4444, 3.76:1 with white) is too light for text, so the badge takes a
// deeper red there, keeping the theme's danger for symbols on dark rows.
var (
	darkBadgeDanger = ui.Hex("#dc2626")
	badgeTextColor  = ui.Hex("#ffffff")
)

// badgeColors is the fill and the text color of a count badge. A danger
// color the desktop changed (a contrast setting) is kept as it is.
func badgeColors(t *ui.Theme) (fill, text ui.Color) {
	fill = t.Danger
	if t.Dark && fill == ui.DarkTheme().Danger {
		fill = darkBadgeDanger
	}
	return fill, badgeTextColor
}
