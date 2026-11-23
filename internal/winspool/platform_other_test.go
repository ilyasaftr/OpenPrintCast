//go:build !windows

package winspool

import (
	"errors"
	"strings"
	"testing"
)

// Non-Windows stubs degrade gracefully so Linux CI passes.
func TestNonWindowsStubs(t *testing.T) {
	backend := NewSystemBackend()
	if got := backend.ListPrinters(); len(got) != 0 {
		t.Fatalf("ListPrinters = %+v, want empty", got)
	}
	if got := ListPrinters(); len(got) != 0 {
		t.Fatalf("package ListPrinters = %+v, want empty", got)
	}
	var target *BackendError
	err := backend.SpoolBuffer("Office", []byte("%PDF-1.4"), "application/octet-stream")
	if !errors.As(err, &target) || target.Kind != KindUnsupported {
		t.Fatalf("SpoolBuffer err = %v, want KindUnsupported", err)
	}
	if !strings.Contains(err.Error(), "requires Windows") {
		t.Fatalf("stub error should mention Windows: %v", err)
	}
	if got := backend.QueryStatus("Office LaserJet"); got != PrinterStatusUnknown {
		t.Fatalf("QueryStatus = %q, want unknown", got)
	}
	if backend.DuplexSupported("Office") || (SystemBackend{}).DuplexSupported("Office") {
		t.Fatal("without the Windows spooler duplex must not be offered")
	}
}
