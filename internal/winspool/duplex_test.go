package winspool

import (
	"sync"
	"testing"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// TestDevModeDuplexMapping pins the IPP sides -> DEVMODE dmDuplex table.
//
// Microsoft documents dmDuplex as DMDUP_SIMPLEX 1 ("Normal (nonduplex)
// printing"), DMDUP_VERTICAL 2 ("Long-edge binding, that is, the long edge of
// the page is vertical") and DMDUP_HORIZONTAL 3 ("Short-edge binding, that is,
// the long edge of the page is horizontal"). Drivers apply the value to the
// sheet, not to the dmOrientation page: Word's "binding setting does not change
// when the page orientation changes", Chromium maps LONG_EDGE to
// DMDUP_VERTICAL and SHORT_EDGE to DMDUP_HORIZONTAL for both orientations, and
// Mozilla bug 1692316 calls the constants "page-orientation-agnostic". IPP
// two-sided-long-edge names the long edge of the medium, so the mapping is the
// same for DMORIENT_PORTRAIT and DMORIENT_LANDSCAPE.
func TestDevModeDuplexMapping(t *testing.T) {
	for _, tc := range []struct {
		sides core.Sides
		want  int16
	}{
		{core.OneSided, 1},
		{core.TwoSidedLongEdge, 2},
		{core.TwoSidedShortEdge, 3},
	} {
		if got := devModeDuplex(tc.sides); got != tc.want {
			t.Errorf("devModeDuplex(%s) = %d, want %d", tc.sides.Keyword(), got, tc.want)
		}
	}
}

func TestDevModeDuplexValuesMatchWingdi(t *testing.T) {
	if dmdupSimplex != 1 || dmdupVertical != 2 || dmdupHorizontal != 3 {
		t.Fatalf("DMDUP_* = %d, %d, %d, want 1, 2, 3", dmdupSimplex, dmdupVertical, dmdupHorizontal)
	}
}

func TestTwoSidedJobsKeepDocumentOrder(t *testing.T) {
	for _, tc := range []struct {
		name     string
		reverse  bool
		sides    core.Sides
		reversed bool
	}{
		{"one-sided reversed", true, core.OneSided, true},
		{"one-sided in order", false, core.OneSided, false},
		{"long edge ignores reverse", true, core.TwoSidedLongEdge, false},
		{"short edge ignores reverse", true, core.TwoSidedShortEdge, false},
	} {
		settings := RenderSettings{ReversePages: tc.reverse, options: core.PrintOptions{Sides: tc.sides}}
		if got := settings.reversesOrder(); got != tc.reversed {
			t.Errorf("%s: reversesOrder = %t, want %t", tc.name, got, tc.reversed)
		}
	}
}

func TestProbeCachedProbesOncePerPrinter(t *testing.T) {
	var (
		mu    sync.RWMutex
		cache capabilityCache[bool]
		calls int
	)
	probe := func(printer string) bool {
		calls++
		return printer == "duplex"
	}
	for i := 0; i < 3; i++ {
		if !probeCached(&mu, &cache, "duplex", probe) || probeCached(&mu, &cache, "simplex", probe) {
			t.Fatal("probe answers were not cached per printer")
		}
	}
	if calls != 2 {
		t.Fatalf("probe ran %d times, want once per printer", calls)
	}
}
