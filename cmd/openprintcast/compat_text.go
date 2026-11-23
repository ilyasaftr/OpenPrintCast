package main

import (
	"slices"
	"strings"

	bridgenet "github.com/ilyasaftr/OpenPrintCast/internal/net"
)

// Wording of the AirPrint compatibility warning, in one place. The row line
// is short; the tooltip and accessible description carry the explanation.
const (
	compatRowLine      = "iPhone, iPad and Mac can't print here"
	compatNextStep     = "Update or reinstall the driver, or try a different driver for this printer, then choose Scan Again."
	compatExplanation  = "AirPrint needs the printer driver to report paper sizes, page orientation and print resolution. This driver didn't report: "
	compatSheetTitle   = "Compatibility"
	compatChecking     = "Checking…"
	compatNotChecked   = "Not checked"
	compatReportedSfx  = ": reported"
	compatMissingSfx   = ": not reported"
	compatRowLabelHead = "Compatibility of "
)

// compatRequirement is one thing a driver must report, in the order the
// warning lists them.
type compatRequirement struct {
	key    string // stable key from the net package
	inline string // as named inside a sentence
	title  string // as named at the start of a sheet row
}

// compatRequirements lists the requirements in display order.
var compatRequirements = []compatRequirement{
	{bridgenet.GapKeyPaperSizes, "paper sizes", "Paper sizes"},
	{bridgenet.GapKeyOrientation, "page orientation", "Page orientation"},
	{bridgenet.GapKeyResolution, "print resolution", "Print resolution"},
}

// missingRequirements returns the requirements named by gaps, in display
// order; keys it does not know are ignored.
func missingRequirements(gaps []string) []compatRequirement {
	var missing []compatRequirement
	for _, r := range compatRequirements {
		if slices.Contains(gaps, r.key) {
			missing = append(missing, r)
		}
	}
	return missing
}

// joinNaturally joins items as prose: "a", "a and b", "a, b and c".
func joinNaturally(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	return strings.Join(items[:len(items)-1], ", ") + " and " + items[len(items)-1]
}

// compatibilityDetail is the full explanation of a warning: what AirPrint
// needs, what this driver did not report, and what to do about it.
func compatibilityDetail(gaps []string) string {
	var names []string
	for _, r := range missingRequirements(gaps) {
		names = append(names, r.inline)
	}
	return compatExplanation + joinNaturally(names) + ". " + compatNextStep
}

// requirementStatus is the sheet row text of one requirement.
func requirementStatus(r compatRequirement, missing bool) string {
	if missing {
		return r.title + compatMissingSfx
	}
	return r.title + compatReportedSfx
}
