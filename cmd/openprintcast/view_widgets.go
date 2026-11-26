package main

import (
	"github.com/egoist/mygo/ui"
)

// Layout metrics follow the Windows spacing rhythm (4 px grid): 16 from
// the window edge, 12 between content areas, 8 between related controls.
const (
	pagePadding   = 16
	sectionGap    = 12
	controlGap    = 8
	settingsWidth = 640
	rowInset      = 56
	settingInset  = 14
)

// The app mark in the header shows from headerMarkMinWidth, where the
// tagline still fits beside it and the status controls.
const (
	headerMarkSize     = 40
	headerMarkMinWidth = 800
)

// Typography and row metrics shared by the grouped lists.
const (
	headlineFontSize = 20
	titleFontSize    = 16
	captionFontSize  = 12

	groupRadius         = 10
	rowGap              = 12
	rowPaddingV         = 10
	rowPaddingH         = 14
	listRowMinHeight    = 52
	settingRowMinHeight = 44
	captionTextGap      = 2
	groupCaptionPadding = 4
	groupSpacing        = 4

	statusDotSize       = 8
	hairline            = 1
	fullyRounded        = 999
	pillGap             = 6
	pillPaddingV        = 4
	pillPaddingH        = 10
	bannerGap           = 10
	bannerPaddingV      = 10
	bannerPaddingH      = 12
	statusCellGap       = 6
	sheetButtonsTop     = 4
	emptyStatePadding   = 24
	iconButtonIconSize  = 18
	emptyStateIconSize  = 56
	emptyStateBodyWidth = 420
)

// levelColor maps a severity to a theme color; it is always shown next to
// a text label, never alone.
func levelColor(t *ui.Theme, l level) ui.Color {
	switch l {
	case levelOK:
		return t.Success
	case levelWarn:
		return t.Warning
	case levelError:
		return t.Danger
	default:
		return t.TextMuted
	}
}

// statusDot is the small colored marker before a status label.
func statusDot(c *ui.Context, l level) {
	ui.Box(c).Size(statusDotSize, statusDotSize).Radius(statusDotSize / 2).Background(levelColor(c.Theme(), l))
}

// captionText is the small secondary text used for details and captions.
func captionText(c *ui.Context, text string, color ui.Color) ui.Element {
	return ui.Text(c, text).FontSize(captionFontSize).TextColor(color)
}

// groupHeader and groupFooter are the small secondary captions above and
// below a grouped list, as in System Settings.
func groupHeader(c *ui.Context, title string, trailing func()) ui.Element {
	t := c.Theme()
	return ui.Row(c).Padding(0, groupCaptionPadding).Children(func() {
		captionText(c, title, t.TextMuted).Bold().SingleLine().Grow(1)
		if trailing != nil {
			trailing()
		}
	})
}

func groupFooter(c *ui.Context, text string, color ui.Color) {
	captionText(c, text, color).Padding(0, groupCaptionPadding)
}

// groupSpacer separates one group (footer) from the next group's header.
func groupSpacer(c *ui.Context) { ui.Box(c).Height(groupSpacing) }

// listGroup is an inset grouped list: one rounded container whose rows are
// separated by hairlines inset to the text edge.
func listGroup(c *ui.Context, rows ...func()) { listGroupInset(c, rowInset, rows...) }

func listGroupInset(c *ui.Context, inset float32, rows ...func()) {
	t := c.Theme()
	ui.Column(c).Radius(groupRadius).Background(t.Surface).Border(hairline, t.Border).Clip().Children(func() {
		for i, row := range rows {
			if i > 0 {
				ui.Box(c).Height(1).Background(t.Border).MarginStart(inset)
			}
			row()
		}
	})
}

// listRow lays out one row: a leading tile, a title with an optional
// secondary line, and trailing controls that stay at the same edge for
// every row so the eye can scan down them.
func listRow(c *ui.Context, tile func(), title func(), detail string, detailColor ui.Color, trailing func()) {
	listRowWithNote(c, tile, title, detail, detailColor, nil, trailing)
}

// listRowWithNote is a listRow with an optional note (nil: none) drawn
// under the secondary line, for rows that need a warning of their own.
func listRowWithNote(c *ui.Context, tile func(), title func(), detail string, detailColor ui.Color, note, trailing func()) {
	ui.Row(c).Gap(rowGap).Padding(rowPaddingV, rowPaddingH).MinHeight(listRowMinHeight).Children(func() {
		tile()
		ui.Column(c).Gap(captionTextGap).Grow(1).MinWidth(0).Children(func() {
			title()
			captionText(c, detail, detailColor).SingleLine()
			if note != nil {
				note()
			}
		})
		trailing()
	})
}

// settingRow is one row of a grouped form: the title and its explanation
// at the leading edge, the control at the trailing edge.
func settingRow(c *ui.Context, title, detail string, control func()) {
	settingRowDetails(c, title, func() {
		if detail != "" {
			captionText(c, detail, c.Theme().TextMuted)
		}
	}, control)
}

// settingRowDetails is a settingRow whose explanation is drawn by details,
// which may stack several lines. The row is keyed by its title: element
// identity is positional otherwise, and a switch or input here would
// inherit the animation and interaction state of whatever sat in its place
// before — another section's switch visibly flipping to its own value.
func settingRowDetails(c *ui.Context, title string, details, control func()) {
	ui.Row(c.Key(title)).Gap(rowGap).Padding(rowPaddingV, rowPaddingH).MinHeight(settingRowMinHeight).Children(func() {
		ui.Column(c).Gap(captionTextGap).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, title)
			details()
		})
		ui.Row(c).Gap(controlGap).Shrink(0).Children(control)
	})
}

// valueRow is a read-only fact: label at the leading edge, selectable
// value at the trailing edge in secondary text.
func valueRow(c *ui.Context, label, value string) {
	ui.Row(c).Gap(rowGap).Padding(rowPaddingV, rowPaddingH).MinHeight(settingRowMinHeight).Children(func() {
		ui.Text(c, label).SingleLine().Shrink(0)
		ui.Text(c, value).TextColor(c.Theme().TextMuted).Grow(1).MinWidth(0).SingleLine().TextAlign(ui.End).
			Selectable().Tooltip(value)
	})
}

// infoButton is a borderless icon button at the end of a row; HIG reserves
// the info button for revealing more about the row's content.
func infoButton(c *ui.Context, label, tip string) bool {
	return iconButton(c, infoIcon, label, tip)
}

// iconButton is a borderless, toolbar-style button showing only its icon; label
// is its accessible name and tip the tooltip. It reports a click.
func iconButton(c *ui.Context, icon *ui.SVG, label, tip string) bool {
	clicked := false
	ui.Toolbar(c, func() {
		clicked = ui.Button(c, "").Label(label).Tooltip(tip).Children(func() {
			ui.Icon(c, icon).FontSize(iconButtonIconSize)
		}).Clicked()
	}).Shrink(0)
	return clicked
}

// emptyState fills a page with a centered icon, explanation and, when
// there is one, the next step: an empty page is never left blank.
func emptyState(c *ui.Context, icon *ui.SVG, title, body string, action func()) {
	ui.Column(c).Grow(1).Center().Gap(controlGap).Padding(emptyStatePadding).Children(func() {
		iconTileSized(c, icon, tileGray, emptyStateIconSize)
		ui.Text(c, title).FontSize(titleFontSize).Bold().Margin(controlGap, 0, 0, 0)
		ui.Text(c, body).TextColor(c.Theme().TextMuted).MaxWidth(emptyStateBodyWidth).TextAlign(ui.Center)
		if action != nil {
			groupSpacer(c)
			action()
		}
	})
}
