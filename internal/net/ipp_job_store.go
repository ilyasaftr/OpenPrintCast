package net

import (
	"context"
	"sync"
	"time"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

type ippJob struct {
	id         uint32
	printerID  string
	uri        string
	user       string
	name       string
	state      int
	copies     int
	template   goipp.Attributes
	created    time.Time
	processing time.Time
	completed  time.Time
	document   []byte
	format     string
	deadline   time.Time
	timer      *time.Timer
	ctx        context.Context
	cancel     context.CancelFunc
}

const pendingJobTimeout = 10 * time.Minute

// stopTimer disarms the pending-job expiry timer.
func (j *ippJob) stopTimer() {
	if j.timer != nil {
		j.timer.Stop()
		j.timer = nil
	}
}

// abortSpooling aborts the spooler context of a processing job.
func (j *ippJob) abortSpooling() {
	if j.cancel != nil {
		j.cancel()
		j.cancel = nil
	}
}

// cancelable reports whether the job can still be stopped: pending, or
// processing under a spooler that honors cancellation.
func (j *ippJob) cancelable() bool {
	return j.state == jobStatePending || (j.state == jobStateProcessing && j.cancel != nil)
}

// ippJobStore holds the per-printer IPP jobs and the memory budget of
// their documents. mu guards the jobs and the budget together so a job's
// document and its accounting always change atomically.
type ippJobStore struct {
	mu      sync.RWMutex
	ippJobs []ippJob
	documentBudget
}

// indexOf returns the position of the first job matching printerID and id,
// restricted to the given states when any are listed, or -1. Callers hold mu.
func (s *ippJobStore) indexOf(printerID string, id uint32, states ...int) int {
	for i := range s.ippJobs {
		job := &s.ippJobs[i]
		if job.printerID != printerID || job.id != id {
			continue
		}
		if len(states) == 0 || containsState(states, job.state) {
			return i
		}
	}
	return -1
}

func containsState(states []int, state int) bool {
	for _, s := range states {
		if s == state {
			return true
		}
	}
	return false
}

// discardDocument drops the job's buffered document and its pending
// accounting. Callers hold mu.
func (s *ippJobStore) discardDocument(job *ippJob) {
	s.releasePending(len(job.document))
	job.document = nil
}

func (s *ippJobStore) expireIPPJob(printerID string, id uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.indexOf(printerID, id, jobStatePending)
	if i < 0 {
		return
	}
	job := &s.ippJobs[i]
	if remaining := time.Until(job.deadline); remaining > 0 {
		job.timer.Reset(remaining)
		return
	}
	s.discardDocument(job)
	job.state = jobStateAborted
	job.completed = time.Now()
	job.stopTimer()
}

func (s *ippJobStore) addIPPJob(job ippJob) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.ippJobs) >= core.MaxLogEntries {
		s.evictFinishedJob()
	}
	if len(s.ippJobs) >= core.MaxLogEntries && job.state == jobStatePending {
		return false
	}
	if job.state == jobStatePending {
		job.deadline = time.Now().Add(pendingJobTimeout)
		job.timer = time.AfterFunc(pendingJobTimeout, func() {
			s.expireIPPJob(job.printerID, job.id)
		})
	}
	s.ippJobs = append(s.ippJobs, job)
	return true
}

// evictFinishedJob removes the oldest finished job, if any. Callers hold mu.
func (s *ippJobStore) evictFinishedJob() {
	for i := range s.ippJobs {
		if s.ippJobs[i].isFinished() {
			s.removeAt(i)
			return
		}
	}
}

func (s *ippJobStore) removeAt(i int) {
	s.ippJobs = append(s.ippJobs[:i], s.ippJobs[i+1:]...)
}

func (s *ippJobStore) getIPPJob(printerID string, id uint32) (ippJob, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, job := range s.ippJobs {
		if job.id == id && job.printerID == printerID {
			return job, true
		}
	}
	return ippJob{}, false
}

func (s *ippJobStore) ippJobsFor(printerID string) []ippJob {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var jobs []ippJob
	for _, job := range s.ippJobs {
		if job.printerID == printerID {
			jobs = append(jobs, job)
		}
	}
	return jobs
}

func (s *ippJobStore) dropPendingIPPJob(printerID string, id uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.indexOf(printerID, id, jobStatePending)
	if i < 0 {
		return
	}
	job := &s.ippJobs[i]
	if job.timer != nil {
		job.timer.Stop()
	}
	s.releasePending(len(job.document))
	s.removeAt(i)
}

func (s *ippJobStore) releaseIPPDocument(size int) {
	s.mu.Lock()
	s.releaseProcessing(size)
	s.mu.Unlock()
}
