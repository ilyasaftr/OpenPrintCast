package winspool

import (
	"context"
	"errors"
	"testing"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

func TestCancelledSpoolJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := NewSystemBackend().SpoolJobContext(ctx, "printer", []byte("%PDF-1.4"), "application/pdf", core.PrintOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled spool error = %v", err)
	}
}

func TestFolderSpoolRejectsUnsafeInputs(t *testing.T) {
	backend := NewSystemBackend()
	if err := backend.SpoolFolderJobContext(context.Background(), []byte("%PDF-"), "application/pdf", core.PrintOptions{}, "relative", "test.pdf"); err == nil {
		t.Fatal("relative output folder was accepted")
	}
	if err := backend.SpoolFolderJobContext(context.Background(), []byte("raw"), "application/octet-stream", core.PrintOptions{}, t.TempDir(), "test.pdf"); err == nil {
		t.Fatal("raw bytes were accepted for PDF output")
	}
}

func TestSpoolRejectsEmptyInputsBeforePlatform(t *testing.T) {
	backend := NewSystemBackend()
	var target *BackendError
	if err := backend.SpoolBuffer("", []byte("data"), "application/pdf"); !errors.As(err, &target) ||
		target.Kind != KindNotFound {
		t.Fatalf("empty printer err = %v", err)
	}
	if err := backend.SpoolBuffer("   ", []byte("data"), "application/pdf"); !errors.As(err, &target) ||
		target.Kind != KindNotFound {
		t.Fatalf("blank printer err = %v", err)
	}
	if err := backend.SpoolBuffer("Office", nil, "application/pdf"); !errors.As(err, &target) ||
		target.Kind != KindSpool {
		t.Fatalf("empty data err = %v", err)
	}
}

func TestQueryStatusBlankIsUnknownWithoutTouchingSpooler(t *testing.T) {
	backend := NewSystemBackend()
	if got := backend.QueryStatus(""); got != PrinterStatusUnknown {
		t.Fatalf("got %q", got)
	}
	if got := backend.QueryStatus("   "); got != PrinterStatusUnknown {
		t.Fatalf("got %q", got)
	}
	if got := QueryStatus(""); got != PrinterStatusUnknown {
		t.Fatalf("package QueryStatus got %q", got)
	}
}
