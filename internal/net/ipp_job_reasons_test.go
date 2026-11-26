package net

import (
	"slices"
	"testing"
)

// reasonsForJob computes the job-state-reasons of one job in the given
// state against a spooler reporting the given driver status.
func reasonsForJob(state, driverState int, driverReasons []string) []string {
	spooler := &fakeSpooler{reportedState: driverState, reportedReasons: driverReasons}
	shared := NewSharedState(nil, nil)
	shared.Spool = spooler
	job := ippJob{state: state}
	return jobStateReasons(job, testPrinter(), shared)
}

// Terminal states keep their fixed RFC 8011 reasons regardless of what the
// driver reports.
func TestJobStateReasonsTerminalStates(t *testing.T) {
	for state, want := range map[int]string{
		jobStateCompleted: "job-completed-successfully",
		jobStateCanceled:  "job-canceled-by-user",
		jobStateAborted:   "aborted-by-system",
		jobStatePending:   "none",
	} {
		got := reasonsForJob(state, printerStateStopped, []string{"media-needed"})
		if len(got) != 1 || got[0] != want {
			t.Errorf("state %d: reasons = %v, want [%s]", state, got, want)
		}
	}
}

// A processing job on a healthy printer has no special reason.
func TestJobStateReasonsProcessingHealthy(t *testing.T) {
	got := reasonsForJob(jobStateProcessing, printerStateIdle, nil)
	if len(got) != 1 || got[0] != "none" {
		t.Fatalf("reasons = %v, want [none]", got)
	}
}

// A processing job on a stopped queue says so: polling clients learn why
// the job cannot advance instead of spinning without explanation.
func TestJobStateReasonsProcessingPrinterStopped(t *testing.T) {
	got := reasonsForJob(jobStateProcessing, printerStateStopped, nil)
	if !slices.Contains(got, "printer-stopped") {
		t.Fatalf("reasons = %v, want printer-stopped", got)
	}
}

// A processing job whose printer needs media reports the RFC job-side
// reason for it.
func TestJobStateReasonsProcessingMediaNeeded(t *testing.T) {
	got := reasonsForJob(jobStateProcessing, printerStateProcessing, []string{"media-needed"})
	if !slices.Contains(got, "resources-are-not-ready") {
		t.Fatalf("reasons = %v, want resources-are-not-ready", got)
	}
}

// Without shared state nothing live can be known: no reason is invented.
func TestJobStateReasonsNilShared(t *testing.T) {
	got := jobStateReasons(ippJob{state: jobStateProcessing}, testPrinter(), nil)
	if len(got) != 1 || got[0] != "none" {
		t.Fatalf("reasons = %v, want [none]", got)
	}
}
