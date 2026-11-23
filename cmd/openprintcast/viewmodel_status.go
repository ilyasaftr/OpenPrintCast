package main

import (
	"slices"
	"strings"

	"github.com/ilyasaftr/OpenPrintCast/internal/winspool"
)

// conditionIcon names the symbol of a printer condition; icons.go maps it
// to a shape, so every state reads without color.
type conditionIcon int

const (
	conditionIconReady conditionIcon = iota
	conditionIconPrinting
	conditionIconPaused
	conditionIconAlert
	conditionIconUnknown
)

// conditionView is how a printer condition is presented: a short label, a
// tooltip that spells it out and names the next step, a severity and a symbol.
type conditionView struct {
	label string
	tip   string
	level level
	icon  conditionIcon
}

// conditionRule describes one reportable condition and when it applies.
type conditionRule struct {
	conditionView
	matches func(printerCondition) bool
}

// has reports whether any of the IPP reason keywords is present.
func (c printerCondition) has(keywords ...string) bool {
	return slices.ContainsFunc(c.reasons, func(r string) bool { return slices.Contains(keywords, r) })
}

// conditionRules lists the reportable conditions, most severe first; the
// first match names the condition and the rest are mentioned in its tip.
var conditionRules = []conditionRule{
	{conditionView{"Paused", "Paused. Resume the queue in Windows Settings.", levelWarn, conditionIconPaused},
		func(c printerCondition) bool { return c.status == winspool.PrinterStatusPaused || c.has("paused") }},
	{conditionView{"Paper out", "Out of paper. Add paper to the printer.", levelError, conditionIconAlert},
		func(c printerCondition) bool { return c.has("media-empty", "media-needed") }},
	{conditionView{"Paper jam", "Paper is jammed. Clear the paper jam.", levelError, conditionIconAlert},
		func(c printerCondition) bool { return c.has("media-jam") }},
	{conditionView{"Cover open", "A cover is open. Close the printer cover.", levelError, conditionIconAlert},
		func(c printerCondition) bool { return c.has("door-open") }},
	{conditionView{"Out of toner", "Out of toner or ink. Replace the toner or ink.", levelError, conditionIconAlert},
		func(c printerCondition) bool { return c.has("marker-supply-empty") }},
	{conditionView{"Output full", "The output tray is full. Empty the output tray.", levelWarn, conditionIconAlert},
		func(c printerCondition) bool { return c.has("output-area-full") }},
	{conditionView{"Offline", "Offline. Check that the printer is turned on, connected and has paper and toner.", levelError, conditionIconAlert},
		func(c printerCondition) bool { return c.status == winspool.PrinterStatusOffline || c.has("offline") }},
	{conditionView{"Needs attention", "The printer reports a problem. Check the printer's display or Windows Settings.", levelWarn, conditionIconAlert},
		func(c printerCondition) bool { return c.has("other") }},
	{conditionView{"Printing", "The printer is printing a job.", levelOK, conditionIconPrinting},
		func(c printerCondition) bool { return c.status == winspool.PrinterStatusPrinting }},
	{conditionView{"Toner low", "Toner or ink is running low.", levelWarn, conditionIconAlert},
		func(c printerCondition) bool { return c.has("marker-supply-low-warning") }},
}

var (
	readyCondition   = conditionView{"Ready", "The printer is ready to print.", levelOK, conditionIconReady}
	unknownCondition = conditionView{"Status unknown", "Status isn't available yet. Try Scan Again.", levelIdle, conditionIconUnknown}
)

// describeCondition turns a printer condition into its presentation. The
// most severe problem decides the label; further problems are listed in
// the tooltip.
func describeCondition(c printerCondition) conditionView {
	var found []conditionView
	for _, rule := range conditionRules {
		if rule.matches(c) {
			found = append(found, rule.conditionView)
		}
	}
	if len(found) == 0 {
		return baseCondition(c.status)
	}
	view := found[0]
	if len(found) > 1 {
		view.tip += " Also: " + joinLabels(found[1:]) + "."
	}
	return view
}

// baseCondition is the presentation of a printer with no problem to report.
func baseCondition(s winspool.PrinterStatus) conditionView {
	if s == winspool.PrinterStatusReady {
		return readyCondition
	}
	return unknownCondition
}

// joinLabels lists the labels of the views, comma separated.
func joinLabels(views []conditionView) string {
	labels := make([]string, len(views))
	for i, v := range views {
		labels[i] = v.label
	}
	return strings.Join(labels, ", ")
}
