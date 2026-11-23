package main

import (
	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// automaticButtonInset is the padding of a toolbar button; it is pulled
// back so the button's text lines up with the caption above it.
const automaticButtonInset = 14

// virtualOverrideFor is the override to store for the chosen value: nil
// (automatic) when it equals what detection found, else the chosen value.
func virtualOverrideFor(chosen, detected bool) *bool {
	if chosen == detected {
		return nil
	}
	return &chosen
}

// virtualCaption explains where the virtual-printer state comes from.
func virtualCaption(overridden bool, reason string) string {
	switch {
	case overridden && reason != "":
		return "Set by you. Detected: " + reason + "."
	case overridden:
		return "Set by you."
	case reason != "":
		return "Detected automatically: " + reason + "."
	}
	return "Detected automatically."
}

// sheetVirtual is the "Virtual printer" switch of the sheet. It edits the
// draft only; Done stores it. While the draft differs from the detected
// value, a button returns to automatic detection.
func (a *bridgeApp) sheetVirtual(c *ui.Context, p *core.PrinterInfo) {
	overridden := a.draftVirtual != p.DetectedVirtual
	ui.Row(c).Gap(rowGap).AlignItems(ui.Start).Children(func() {
		ui.Column(c).Gap(captionTextGap).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, "Virtual printer")
			captionText(c, virtualCaption(overridden, p.VirtualReason), c.Theme().TextMuted)
			if overridden {
				a.useAutomaticDetectionButton(c, p)
			}
		})
		ui.Switch(c, &a.draftVirtual).Label("Treat " + p.WindowsName + " as a virtual printer")
	})
}

// useAutomaticDetectionButton resets the draft to the detected value.
func (a *bridgeApp) useAutomaticDetectionButton(c *ui.Context, p *core.PrinterInfo) {
	ui.Toolbar(c, func() {
		if ui.Button(c, "Use Automatic Detection").TextColor(accentForeground(c.Theme())).Clicked() {
			a.draftVirtual = p.DetectedVirtual
		}
	}).Shrink(0).MarginStart(-automaticButtonInset)
}
