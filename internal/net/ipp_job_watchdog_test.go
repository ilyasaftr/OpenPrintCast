package net

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/OpenPrinting/goipp"
)

// processingTestJob builds a store with one processing job whose progress
// stamp is set to progressAt. The job carries a cancellable context, as
// jobs on the Windows backend do.
func processingTestStore(t *testing.T, progressAt time.Time) (*ippJobStore, *ippJob) {
	t.Helper()
	s := &ippJobStore{}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	job := ippJob{
		id:         1,
		printerID:  "hp",
		user:       "mobile",
		state:      jobStateProcessing,
		processing: progressAt,
		ctx:        ctx,
		cancel:     cancel,
		entryID:    "entry-1",
		progress:   new(atomic.Int64),
	}
	job.progress.Store(progressAt.UnixNano())
	s.ippJobs = append(s.ippJobs, job)
	return s, &s.ippJobs[0]
}

// A job with no progress for longer than the timeout is aborted and its
// spool context is cancelled, so the client stops seeing an endless
// processing state.
func TestWatchdogAbortsStuckJob(t *testing.T) {
	now := time.Now()
	s, _ := processingTestStore(t, now.Add(-stuckJobTimeout-time.Minute))
	stuck := s.abortStuckIPPJobs(now)
	if len(stuck) != 1 {
		t.Fatalf("stuck = %d, want 1", len(stuck))
	}
	job := s.ippJobs[0]
	if job.state != jobStateAborted {
		t.Fatalf("state = %d, want aborted", job.state)
	}
	if job.completed.IsZero() {
		t.Fatal("completed was not set")
	}
	if job.ctx.Err() == nil {
		t.Fatal("spool context was not cancelled")
	}
	if stuck[0].entryID != "entry-1" || stuck[0].user != "mobile" {
		t.Fatalf("stuck = %+v", stuck[0])
	}
}

// A job that keeps making progress is never touched, however long it runs.
func TestWatchdogSparesProgressingJob(t *testing.T) {
	now := time.Now()
	// Entered processing an hour ago but heartbeated a minute ago: the
	// inactivity measure, not the total time, decides.
	s, _ := processingTestStore(t, now.Add(-time.Minute))
	s.ippJobs[0].processing = now.Add(-time.Hour)
	if stuck := s.abortStuckIPPJobs(now); len(stuck) != 0 {
		t.Fatalf("stuck = %+v, want none", stuck)
	}
	if s.ippJobs[0].state != jobStateProcessing {
		t.Fatalf("state = %d, want still processing", s.ippJobs[0].state)
	}
}

// Jobs outside processing are never the watchdog's business.
func TestWatchdogIgnoresOtherStates(t *testing.T) {
	now := time.Now()
	s, _ := processingTestStore(t, now.Add(-time.Hour))
	s.ippJobs[0].state = jobStateCompleted
	if stuck := s.abortStuckIPPJobs(now); len(stuck) != 0 {
		t.Fatalf("stuck = %+v, want none", stuck)
	}
	if s.ippJobs[0].state != jobStateCompleted {
		t.Fatal("completed job was modified")
	}
}

// Entering processing through Send-Document completion stamps the progress
// clock, so the watchdog measures inactivity from the spool start even
// before the first heartbeat.
func TestCloseForProcessingStampsProgress(t *testing.T) {
	s := &ippJobStore{}
	pending := ippJob{id: 7, printerID: "hp", state: jobStatePending}
	s.ippJobs = append(s.ippJobs, pending)
	before := time.Now()
	job, _, status := s.receiveDocument("hp", 7, []byte("%PDF-1.4 x"), "application/pdf", true, true)
	if status != goipp.StatusOk {
		t.Fatalf("receiveDocument status = %s", status)
	}
	if job.progress == nil {
		t.Fatal("processing job has no progress clock")
	}
	stamp := time.Unix(0, job.progress.Load())
	if stamp.Before(before.Add(-time.Second)) || stamp.After(time.Now().Add(time.Second)) {
		t.Fatalf("progress stamp %v is not the transition time", stamp)
	}
}
