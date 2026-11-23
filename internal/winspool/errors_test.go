package winspool

import (
	"strings"
	"testing"
)

func TestBackendErrorMessages(t *testing.T) {
	for _, err := range []error{
		UnsupportedError("spool PDF"),
		NotFoundError("Ghost"),
		SpoolError("Office", "jammed"),
		PlatformError("StartDocPrinterW", "denied"),
	} {
		if err.Error() == "" {
			t.Fatal("empty error message")
		}
	}
	if !strings.Contains(NotFoundError("Ghost").Error(), "Ghost") {
		t.Fatal("NotFound should name the printer")
	}
}
