package winspool

import (
	"testing"
)

func TestIPPStateFromWindowsBits(t *testing.T) {
	state, reasons := IPPStateFromWinspoolBits(statusPrinting | statusPaperOut | statusTonerLow)
	if state != 5 || len(reasons) != 3 || reasons[0] != "media-empty" || reasons[1] != "media-needed" || reasons[2] != "marker-supply-low-warning" {
		t.Fatalf("state=%d reasons=%v", state, reasons)
	}
	state, reasons = IPPStateFromWinspoolBits(0)
	if state != 3 || len(reasons) != 1 || reasons[0] != "none" {
		t.Fatalf("idle state=%d reasons=%v", state, reasons)
	}
}

func TestStatusBitsZeroMeansReady(t *testing.T) {
	if got := StatusFromWinspoolBits(0); got != PrinterStatusReady {
		t.Fatalf("got %q", got)
	}
}

func TestStatusBitsPausedBeatsEverything(t *testing.T) {
	bits := statusPaused | statusPrinting | statusOffline
	if got := StatusFromWinspoolBits(bits); got != PrinterStatusPaused {
		t.Fatalf("got %q", got)
	}
}

func TestStatusBitsFaultsMeanOffline(t *testing.T) {
	for _, bits := range []uint32{
		statusOffline, statusError, statusPaperJam, statusPaperOut,
		statusDoorOpen, statusUserIntervention, statusPendingDeletion,
		statusManualFeed, statusNoToner, statusOutOfMemory, statusServerUnknown,
	} {
		if got := StatusFromWinspoolBits(bits); got != PrinterStatusOffline {
			t.Fatalf("bits=%#X got %q, want offline", bits, got)
		}
	}
}

func TestStatusBitsActivityMeansPrinting(t *testing.T) {
	for _, bits := range []uint32{
		statusPrinting, statusIOActive, statusBusy, statusProcessing, statusWaiting,
	} {
		if got := StatusFromWinspoolBits(bits); got != PrinterStatusPrinting {
			t.Fatalf("bits=%#X got %q, want printing", bits, got)
		}
	}
	// Fault beats activity.
	if got := StatusFromWinspoolBits(statusPrinting | statusPaperOut); got != PrinterStatusOffline {
		t.Fatalf("fault+activity got %q, want offline", got)
	}
}

func TestStatusBitsDegradedStaysReady(t *testing.T) {
	bits := statusInitializing | statusWarmingUp | statusTonerLow | statusPowerSave
	if got := StatusFromWinspoolBits(bits); got != PrinterStatusReady {
		t.Fatalf("got %q, want ready", got)
	}
}

func TestStatusCodesAndAcceptsJobs(t *testing.T) {
	if PrinterStatusReady.AsCode() != "ready" || PrinterStatusUnknown.AsCode() != "unknown" {
		t.Fatal("bad status codes")
	}
	for status, want := range map[PrinterStatus]bool{
		PrinterStatusReady: true, PrinterStatusPrinting: true,
		PrinterStatusPaused: false, PrinterStatusOffline: false, PrinterStatusUnknown: false,
	} {
		if got := status.AcceptsJobs(); got != want {
			t.Fatalf("%q AcceptsJobs = %v", status, got)
		}
	}
}
