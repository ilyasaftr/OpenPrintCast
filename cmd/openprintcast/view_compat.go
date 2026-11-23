package main

import (
	"slices"

	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// compatState is how a printer's compatibility check reads on screen.
type compatState int

const (
	compatPending compatState = iota // not checked yet
	compatUnknown                    // checked, but could not be determined
	compatReady                      // the driver reports everything
	compatGaps                       // the driver leaves something out
)

// compatibilityOf reads the cached check of p. It never queries the driver:
// the compatibility monitor fills the cache off the UI thread.
func (a *bridgeApp) compatibilityOf(p *core.PrinterInfo) (compatState, []string) {
	if p.IsFolderDestination() {
		return compatReady, nil
	}
	r, ok := a.printerCompat.Result(p.ID)
	switch {
	case !ok:
		return compatPending, nil
	case !r.known:
		return compatUnknown, nil
	case len(r.gaps) > 0:
		return compatGaps, r.gaps
	}
	return compatReady, nil
}

// compatibilityNote is the warning line of a printer row, or nil when the
// row needs none: only a shared printer whose driver leaves out something
// IPP/2.0 needs gets one, because sharing off advertises nothing.
func (a *bridgeApp) compatibilityNote(c *ui.Context, p *core.PrinterInfo) func() {
	state, gaps := a.compatibilityOf(p)
	if !p.Enabled || state != compatGaps {
		return nil
	}
	t := c.Theme()
	return func() {
		ui.Row(c).Gap(statusCellGap).AlignItems(ui.Start).Children(func() {
			ui.Icon(c, alertIcon).FontSize(captionFontSize).TextColor(t.Warning).Shrink(0)
			captionText(c, compatRowLine, t.Warning).Grow(1).MinWidth(0)
		}).Label(compatRowLabelHead + p.WindowsName + ": " + compatRowLine).Tooltip(compatibilityDetail(gaps))
	}
}

// sheetCompatibility is the "Compatibility" section of the printer sheet:
// one row per requirement, with a symbol and words, and the next step when
// something is missing.
func (a *bridgeApp) sheetCompatibility(c *ui.Context, p *core.PrinterInfo) {
	t := c.Theme()
	state, gaps := a.compatibilityOf(p)
	ui.Column(c).Gap(captionTextGap).Children(func() {
		captionText(c, compatSheetTitle, t.TextMuted).Bold().SingleLine()
		switch state {
		case compatReady, compatGaps:
			for _, r := range compatRequirements {
				requirementRow(c, r, slices.Contains(gaps, r.key))
			}
		default:
			compatibilityPlaceholder(c, p, state)
		}
		if state == compatGaps {
			captionText(c, compatNextStep, t.TextMuted)
		}
	})
}

// requirementRow is one requirement with its symbol and its status text.
func requirementRow(c *ui.Context, r compatRequirement, missing bool) {
	t := c.Theme()
	icon, color := checkCircleIcon, t.Success
	if missing {
		icon, color = alertIcon, t.Warning
	}
	ui.Row(c).Gap(statusCellGap).Children(func() {
		ui.Icon(c, icon).FontSize(captionFontSize).TextColor(color).Shrink(0)
		captionText(c, requirementStatus(r, missing), t.TextMuted).SingleLine()
	})
}

// compatibilityPlaceholder says the check has no answer yet, without a
// warning: "Checking…" for a shared printer that is about to be checked,
// "Not checked" otherwise.
func compatibilityPlaceholder(c *ui.Context, p *core.PrinterInfo, state compatState) {
	text := compatNotChecked
	if state == compatPending && p.Enabled {
		text = compatChecking
	}
	t := c.Theme()
	ui.Row(c).Gap(statusCellGap).Children(func() {
		ui.Icon(c, helpIcon).FontSize(captionFontSize).TextColor(t.TextMuted).Shrink(0)
		captionText(c, text, t.TextMuted).SingleLine()
	})
}
