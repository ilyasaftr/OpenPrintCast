//go:build windows

package winspool

import (
	"testing"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// These tests exercise the DEVMODE option logic on an in-memory struct; they
// make no spooler calls.

func duplexDevMode() *printerDevMode {
	return &printerDevMode{fields: dmDuplex | dmCopies, duplex: dmdupHorizontal, copies: 3}
}

func TestApplyDevModeOptionsSetsRequestedSides(t *testing.T) {
	for _, tc := range []struct {
		sides core.Sides
		want  int16
	}{
		{core.OneSided, dmdupSimplex},
		{core.TwoSidedLongEdge, dmdupVertical},
		{core.TwoSidedShortEdge, dmdupHorizontal},
	} {
		mode := duplexDevMode()
		request, err := applyDevModeOptions("P", mode, core.PrintOptions{Sides: tc.sides})
		if err != nil || mode.duplex != tc.want || mode.copies != 1 {
			t.Fatalf("%s: duplex = %d, copies = %d, err = %v", tc.sides.Keyword(), mode.duplex, mode.copies, err)
		}
		if !request.honoredBy(mode) {
			t.Errorf("%s: a driver that kept the value must be honored", tc.sides.Keyword())
		}
	}
}

func TestHonoredByDetectsDriverChangingSides(t *testing.T) {
	mode := duplexDevMode()
	request, err := applyDevModeOptions("P", mode, core.PrintOptions{Sides: core.TwoSidedLongEdge})
	if err != nil {
		t.Fatal(err)
	}
	mode.duplex = dmdupSimplex // the driver fell back to one-sided
	if request.honoredBy(mode) {
		t.Fatal("a driver that reset duplex to simplex was reported as honoring two-sided")
	}
	mode.duplex, mode.fields = dmdupVertical, mode.fields&^dmDuplex
	if request.honoredBy(mode) {
		t.Fatal("a driver that dropped the duplex field was reported as honoring two-sided")
	}
}

func TestApplyDevModeOptionsRejectsTwoSidedWithoutDuplexField(t *testing.T) {
	mode := &printerDevMode{fields: dmCopies}
	if _, err := applyDevModeOptions("P", mode, core.PrintOptions{Sides: core.TwoSidedShortEdge}); err == nil {
		t.Fatal("two-sided was accepted for a driver without a duplex field")
	}
	request, err := applyDevModeOptions("P", &printerDevMode{fields: dmCopies}, core.PrintOptions{})
	if err != nil || request.duplex != 0 {
		t.Fatalf("one-sided on a driver without a duplex field: duplex = %d, err = %v", request.duplex, err)
	}
}
