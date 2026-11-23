package main

import (
	"github.com/egoist/mygo/ui"
	"testing"

	"github.com/ilyasaftr/OpenPrintCast/internal/winspool"
)

func TestDescribeCondition(t *testing.T) {
	cases := []struct {
		name  string
		cond  printerCondition
		label string
		lvl   level
		icon  conditionIcon
	}{
		{"never polled", printerCondition{}, "Status unknown", levelIdle, conditionIconUnknown},
		{"unknown", printerCondition{status: winspool.PrinterStatusUnknown}, "Status unknown", levelIdle, conditionIconUnknown},
		{"ready", printerCondition{status: winspool.PrinterStatusReady, reasons: []string{"none"}}, "Ready", levelOK, conditionIconReady},
		{"printing", printerCondition{status: winspool.PrinterStatusPrinting}, "Printing", levelOK, conditionIconPrinting},
		{"paused status", printerCondition{status: winspool.PrinterStatusPaused}, "Paused", levelWarn, conditionIconPaused},
		{"paused reason", printerCondition{status: winspool.PrinterStatusOffline, reasons: []string{"paused"}}, "Paused", levelWarn, conditionIconPaused},
		{"media empty", printerCondition{status: winspool.PrinterStatusOffline, reasons: []string{"media-empty"}}, "Paper out", levelError, conditionIconAlert},
		{"media needed", printerCondition{status: winspool.PrinterStatusOffline, reasons: []string{"media-needed"}}, "Paper out", levelError, conditionIconAlert},
		{"jam", printerCondition{reasons: []string{"media-jam"}}, "Paper jam", levelError, conditionIconAlert},
		{"door", printerCondition{reasons: []string{"door-open"}}, "Cover open", levelError, conditionIconAlert},
		{"toner empty", printerCondition{reasons: []string{"marker-supply-empty"}}, "Out of toner", levelError, conditionIconAlert},
		{"output full", printerCondition{reasons: []string{"output-area-full"}}, "Output full", levelWarn, conditionIconAlert},
		{"offline status", printerCondition{status: winspool.PrinterStatusOffline}, "Offline", levelError, conditionIconAlert},
		{"offline reason", printerCondition{reasons: []string{"offline"}}, "Offline", levelError, conditionIconAlert},
		{"other", printerCondition{status: winspool.PrinterStatusReady, reasons: []string{"other"}}, "Needs attention", levelWarn, conditionIconAlert},
		{"toner low", printerCondition{status: winspool.PrinterStatusReady, reasons: []string{"marker-supply-low-warning"}}, "Toner low", levelWarn, conditionIconAlert},
		{"printing beats toner low", printerCondition{status: winspool.PrinterStatusPrinting, reasons: []string{"marker-supply-low-warning"}}, "Printing", levelOK, conditionIconPrinting},
		{"paused beats everything", printerCondition{status: winspool.PrinterStatusPaused, reasons: []string{"media-jam", "offline"}}, "Paused", levelWarn, conditionIconPaused},
		{"jam beats offline", printerCondition{status: winspool.PrinterStatusOffline, reasons: []string{"offline", "media-jam", "door-open"}}, "Paper jam", levelError, conditionIconAlert},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := describeCondition(tc.cond)
			if got.label != tc.label || got.level != tc.lvl || got.icon != tc.icon {
				t.Fatalf("describeCondition = %+v, want %q/%v/%v", got, tc.label, tc.lvl, tc.icon)
			}
			if got.tip == "" {
				t.Fatal("every condition needs a tooltip")
			}
		})
	}
}

func TestDescribeConditionTooltipsNameTheNextStep(t *testing.T) {
	cases := []struct {
		name string
		cond printerCondition
		tip  string
	}{
		{"paused", printerCondition{status: winspool.PrinterStatusPaused}, "Paused. Resume the queue in Windows Settings."},
		{"paper out", printerCondition{reasons: []string{"media-empty"}}, "Out of paper. Add paper to the printer."},
		{"jam", printerCondition{reasons: []string{"media-jam"}}, "Paper is jammed. Clear the paper jam."},
		{"cover open", printerCondition{reasons: []string{"door-open"}}, "A cover is open. Close the printer cover."},
		{"out of toner", printerCondition{reasons: []string{"marker-supply-empty"}}, "Out of toner or ink. Replace the toner or ink."},
		{"output full", printerCondition{reasons: []string{"output-area-full"}}, "The output tray is full. Empty the output tray."},
		{"offline", printerCondition{status: winspool.PrinterStatusOffline}, "Offline. Check that the printer is turned on, connected and has paper and toner."},
		{"needs attention", printerCondition{reasons: []string{"other"}}, "The printer reports a problem. Check the printer's display or Windows Settings."},
		{"unknown", printerCondition{}, "Status isn't available yet. Try Scan Again."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := describeCondition(tc.cond).tip; got != tc.tip {
				t.Fatalf("tip = %q, want %q", got, tc.tip)
			}
		})
	}
}

func TestDescribeConditionKeepsTheAlsoReasons(t *testing.T) {
	multi := describeCondition(printerCondition{reasons: []string{"door-open", "media-empty", "marker-supply-low-warning"}})
	want := "Out of paper. Add paper to the printer. Also: Cover open, Toner low."
	if multi.label != "Paper out" || multi.tip != want {
		t.Fatalf("multi-reason view = %+v, want tip %q", multi, want)
	}
}

func TestConditionIconsHaveDistinctShapes(t *testing.T) {
	seen := map[*ui.SVG]conditionIcon{}
	for _, k := range []conditionIcon{conditionIconReady, conditionIconPrinting, conditionIconPaused, conditionIconAlert, conditionIconUnknown} {
		svg := conditionSymbol(k)
		if prev, dup := seen[svg]; dup {
			t.Errorf("icons %d and %d share a shape", prev, k)
		}
		seen[svg] = k
	}
}
