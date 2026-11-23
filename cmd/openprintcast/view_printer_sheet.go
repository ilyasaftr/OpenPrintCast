package main

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

const printerSheetWidth = 460

func (a *bridgeApp) openPrinterSheet(p *core.PrinterInfo) {
	a.editingPrinter = p.WindowsName
	a.draftName, a.draftLocation = p.AirPrintName, p.Location
	a.draftVirtual = p.IsVirtual
	a.printerDialogOpen = true
}

// editedPrinter resolves the printer being edited by its Windows name; it
// returns nil when a rescan has removed it.
func (a *bridgeApp) editedPrinter() *core.PrinterInfo {
	for i := range a.printers {
		if a.printers[i].WindowsName == a.editingPrinter {
			return &a.printers[i]
		}
	}
	return nil
}

// commitPrinterSheet applies the trimmed draft to the edited printer and
// closes the sheet. A printer the draft turns virtual disappears from the
// list unless virtual printers are shown, which a toast explains.
func (a *bridgeApp) commitPrinterSheet(c *ui.Context, p *core.PrinterInfo) {
	wasVisible := a.visiblePrinter(*p)
	p.AirPrintName = strings.TrimSpace(a.draftName)
	p.Location = strings.TrimSpace(a.draftLocation)
	p.SetVirtualOverride(virtualOverrideFor(a.draftVirtual, p.DetectedVirtual))
	name, nowHidden := p.WindowsName, wasVisible && !a.visiblePrinter(*p)
	a.printerDialogOpen = false
	orderPrinters(a.printers)
	a.printersChanged()
	if nowHidden {
		c.Toast(name + " is now hidden. Turn on Show virtual printers to see it.")
	}
}

// viewPrinterDialog is the Name and Location sheet. Edits go to a draft:
// Done commits them, while Cancel, Escape and a click outside discard them.
// The queue is resolved each frame so a rescan cannot leave a stale pointer.
func (a *bridgeApp) viewPrinterDialog(c *ui.Context) {
	if !a.printerDialogOpen {
		return
	}
	p := a.editedPrinter()
	if p == nil {
		a.printerDialogOpen = false
		return
	}
	nameErr := validateAirPrintName(strings.TrimSpace(a.draftName))
	commit := false
	ui.Modal(c, &a.printerDialogOpen, func() {
		a.sheetHeader(c, p)
		if a.sheetFields(c, p, nameErr) {
			commit = true
		}
		a.sheetCompatibility(c, p)
		if a.sheetButtons(c, nameErr) {
			commit = true
		}
	}).Width(printerSheetWidth).Label("Name and Location for " + p.WindowsName)
	if commit {
		a.commitPrinterSheet(c, p)
	}
}

// sheetHeader names the printer being edited, next to its tile.
func (a *bridgeApp) sheetHeader(c *ui.Context, p *core.PrinterInfo) {
	t := c.Theme()
	ui.Row(c).Gap(rowGap).Children(func() {
		printerTile(c, p)
		ui.Column(c).Gap(captionTextGap).Grow(1).Children(func() {
			ui.Text(c, "Name and Location").FontSize(titleFontSize).Bold().SingleLine()
			ui.Text(c, p.WindowsName).TextColor(t.TextMuted).SingleLine()
		})
	})
}

// sheetFields draws the draft inputs and the virtual-printer switch, and
// reports whether Enter was pressed in an input with a valid name.
func (a *bridgeApp) sheetFields(c *ui.Context, p *core.PrinterInfo, nameErr string) (submitted bool) {
	ui.Column(c).Gap(sectionGap).Children(func() {
		ui.Field(c, "Name on devices", func() {
			in := ui.TextInput(c, &a.draftName).Placeholder(p.WindowsName)
			if in.Submitted() && nameErr == "" {
				submitted = true
			}
		}).Description("Shown on iPhone, iPad and Mac. Leave empty to use the Windows name.").Error(nameErr)
		ui.Field(c, "Location", func() {
			in := ui.TextInput(c, &a.draftLocation).Placeholder("For example: Office, 2nd floor")
			if in.Submitted() && nameErr == "" {
				submitted = true
			}
		})
		a.sheetVirtual(c, p)
	})
	return submitted
}

// sheetButtons draws Cancel and Done and reports whether Done was clicked.
// While the name is invalid Done is unavailable but stays reachable by
// keyboard and screen reader, which hear the reason as its description.
func (a *bridgeApp) sheetButtons(c *ui.Context, nameErr string) (done bool) {
	ui.Row(c).Gap(controlGap).Justify(ui.End).Margin(sheetButtonsTop, 0, 0, 0).Children(func() {
		if ui.Button(c, "Cancel").Clicked() {
			a.printerDialogOpen = false
		}
		button := ui.PrimaryButton(c, "Done").AccessDisabled(nameErr != "")
		if nameErr != "" {
			button.Description(nameErr)
		}
		if button.Clicked() && nameErr == "" {
			done = true
		}
	})
	return done
}
