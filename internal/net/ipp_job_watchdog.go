package net

import (
	"context"
	"time"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

const (
	// stuckJobTimeout is how long a processing job may go without any
	// visible progress before the watchdog declares it wedged and aborts
	// it. The measure is inactivity, not total time: a healthy long print
	// keeps writing pages and never trips it, while a job blocked inside
	// a wedged driver call makes no progress at all. Fifteen minutes is
	// orders of magnitude beyond the slowest legitimate page render and
	// is in the spirit of CUPS' MaxJobTime stuck-job cancellation
	// (cupsd.conf, default three hours of total job time), tightened
	// because inactivity is a far stronger signal than elapsed time.
	stuckJobTimeout = 15 * time.Minute
	// watchdogInterval is how often the watchdog scans in-flight jobs.
	watchdogInterval = 30 * time.Second
)

// stuckJob is a processing job the watchdog declared wedged.
type stuckJob struct {
	printerID string
	id        uint32
	user      string
	entryID   string
	elapsed   time.Duration
	idle      time.Duration
}

// abortStuckIPPJobs aborts every processing job whose progress heartbeat
// has been silent for longer than stuckJobTimeout and returns what it
// aborted. Aborting follows RFC 8011 section 5.3.7: the job moves to the
// aborted state with the aborted-by-system reason, which is what clients
// poll to learn the job is finished rather than spinning forever.
//
// Cancellation is best-effort: a goroutine blocked inside a spooler
// syscall cannot be interrupted, so the job's worker may keep running
// after the abort. That is safe — its completion transition finds no
// processing job and does nothing — at the price of the leaked render
// slot, which a process restart reclaims.
func (s *ippJobStore) abortStuckIPPJobs(now time.Time) []stuckJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	var stuck []stuckJob
	for i := range s.ippJobs {
		job := &s.ippJobs[i]
		if job.state != jobStateProcessing || job.progress == nil {
			continue
		}
		last := time.Unix(0, job.progress.Load())
		idle := now.Sub(last)
		if idle < stuckJobTimeout {
			continue
		}
		stuck = append(stuck, stuckJob{
			printerID: job.printerID,
			id:        job.id,
			user:      job.user,
			entryID:   job.entryID,
			elapsed:   now.Sub(job.processing),
			idle:      idle,
		})
		job.stopTimer()
		s.discardDocument(job)
		job.state = jobStateAborted
		job.completed = now
		job.abortSpooling()
	}
	return stuck
}

// watchdogLoop scans for wedged jobs until ctx ends.
func watchdogLoop(ctx context.Context, shared *SharedState) {
	ticker := time.NewTicker(watchdogInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			for _, job := range shared.abortStuckIPPJobs(now) {
				componentLogger(componentSpool).Error("print job aborted: no progress, the Windows print pipeline appears stuck",
					"printer", job.printerID, "job", job.id, "user", job.user,
					"elapsed", job.elapsed.Round(time.Second), "idle", job.idle.Round(time.Second))
				// The wedged worker never reaches its own finish, so the
				// watchdog closes the visible history entry for it.
				if job.entryID != "" {
					shared.Jobs.Update(job.entryID, core.JobFailed, "aborted: the print pipeline made no progress")
				}
			}
		}
	}
}

// startWatchdog runs the watchdog in the background; it stops with ctx.
func startWatchdog(ctx context.Context, shared *SharedState) {
	go watchdogLoop(ctx, shared)
}
