package net

import (
	"errors"
	"strings"
	"testing"

	"github.com/OpenPrinting/goipp"
)

func TestRejectionReason(t *testing.T) {
	for status, want := range map[goipp.Status]string{
		goipp.StatusErrorBusy:                       "Sharing was paused",
		goipp.StatusErrorDocumentFormatNotSupported: "Unsupported document format",
		goipp.StatusErrorAttributesOrValues:         "Unsupported print options",
		goipp.StatusErrorInternal:                   "No printer backend is available",
		goipp.StatusErrorNotAcceptingJobs:           "Printer is not accepting jobs",
		goipp.StatusErrorNotFound:                   "Print job was rejected",
	} {
		if got := rejectionReason(status); got != want {
			t.Errorf("rejectionReason(%s) = %q, want %q", status, got, want)
		}
	}
}

func TestSpoolFailureReasonUsesErrorText(t *testing.T) {
	if got := spoolFailureReason(errors.New("  spooler\n down\t now ")); got != "spooler down now" {
		t.Fatalf("reason = %q", got)
	}
	if got := spoolFailureReason(errors.New("  ")); got != reasonSpoolFailed {
		t.Fatalf("blank error reason = %q", got)
	}
}

func TestSpoolFailureReasonTruncatesByRunes(t *testing.T) {
	long := strings.Repeat("é", maxReasonRunes+40)
	got := []rune(spoolFailureReason(errors.New(long)))
	if len(got) != maxReasonRunes || string(got[len(got)-1:]) != reasonEllipsis {
		t.Fatalf("truncated reason has %d runes, tail %q", len(got), string(got[len(got)-1:]))
	}
	exact := strings.Repeat("x", maxReasonRunes)
	if got := spoolFailureReason(errors.New(exact)); got != exact {
		t.Fatalf("reason at the limit was altered: %q", got)
	}
}
