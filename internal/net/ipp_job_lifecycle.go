package net

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/OpenPrinting/goipp"
)

func (s *ippJobStore) transitionIPPJob(printerID string, id uint32, from, to int) (ippJob, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.indexOf(printerID, id, from)
	if i < 0 {
		return ippJob{}, false
	}
	job := &s.ippJobs[i]
	job.state = to
	if to == jobStateProcessing {
		job.processing = time.Now()
	} else if to >= jobStateCanceled {
		job.completed = time.Now()
	}
	if to != jobStatePending {
		job.stopTimer()
		s.discardDocument(job)
		job.abortSpooling()
	}
	return *job, true
}

// receiveDocument stores at most one document until the client closes the
// job. Closing atomically transfers that document to the processing caller.
// cancellable says whether the spooler honors a per-job context.
func (s *ippJobStore) receiveDocument(printerID string, id uint32, doc []byte, format string, closeJob, cancellable bool) (ippJob, []byte, goipp.Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.indexOf(printerID, id)
	if i < 0 {
		return ippJob{}, nil, goipp.StatusErrorNotFound
	}
	job := &s.ippJobs[i]
	if job.state != jobStatePending {
		return ippJob{}, nil, goipp.StatusErrorNotPossible
	}
	if len(doc) > 0 && len(job.document) > 0 {
		return ippJob{}, nil, goipp.StatusErrorMultipleJobsNotSupported
	}
	if !closeJob {
		return s.stageDocument(job, doc, format)
	}
	return s.closeForProcessing(job, doc, format, cancellable)
}

// stageDocument buffers doc on a pending job that stays open.
func (s *ippJobStore) stageDocument(job *ippJob, doc []byte, format string) (ippJob, []byte, goipp.Status) {
	if len(doc) == 0 {
		return ippJob{}, nil, goipp.StatusErrorBadRequest
	}
	if !s.fits(len(doc)) {
		return ippJob{}, nil, goipp.StatusErrorBusy
	}
	job.document = doc
	job.format = format
	s.addPending(len(doc))
	job.deadline = time.Now().Add(pendingJobTimeout)
	job.timer.Reset(pendingJobTimeout)
	return *job, nil, goipp.StatusOk
}

// closeForProcessing ends the upload phase and hands the final document
// (the new one, or the one staged earlier) to the caller for spooling.
func (s *ippJobStore) closeForProcessing(job *ippJob, doc []byte, format string, cancellable bool) (ippJob, []byte, goipp.Status) {
	if len(doc) == 0 {
		doc = job.document
		format = job.format
	} else if !s.fits(len(doc)) {
		return ippJob{}, nil, goipp.StatusErrorBusy
	}
	s.releasePending(len(job.document))
	s.addProcessing(len(doc))
	job.document = nil
	job.format = format
	job.stopTimer()
	if len(doc) == 0 {
		job.state = jobStateCompleted
		job.completed = time.Now()
	} else {
		job.state = jobStateProcessing
		job.processing = time.Now()
		job.progress = new(atomic.Int64)
		job.progress.Store(time.Now().UnixNano())
		if cancellable {
			job.ctx, job.cancel = context.WithCancel(context.Background())
		}
	}
	return *job, doc, goipp.StatusOk
}

func (s *ippJobStore) cancelIPPJob(printerID string, id uint32) goipp.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	i := s.indexOf(printerID, id)
	if i < 0 {
		return goipp.StatusErrorNotFound
	}
	job := &s.ippJobs[i]
	if !job.cancelable() {
		return goipp.StatusErrorNotPossible
	}
	s.cancelJob(job)
	return goipp.StatusOk
}

// cancelMyIPPJobs checks the entire selection before canceling anything.
func (s *ippJobStore) cancelMyIPPJobs(printerID, user string, ids []uint32) goipp.Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	indexes, status := s.selectJobsToCancel(printerID, user, ids)
	if status != goipp.StatusOk {
		return status
	}
	for _, i := range indexes {
		s.cancelJob(&s.ippJobs[i])
	}
	return goipp.StatusOk
}

// selectJobsToCancel resolves the jobs user may cancel: exactly ids when
// given, otherwise every unfinished job they own. Callers hold mu.
func (s *ippJobStore) selectJobsToCancel(printerID, user string, ids []uint32) ([]int, goipp.Status) {
	selected := make(map[uint32]bool, len(ids))
	for _, id := range ids {
		selected[id] = false
	}
	var indexes []int
	for i := range s.ippJobs {
		job := &s.ippJobs[i]
		if job.printerID != printerID {
			continue
		}
		if len(ids) > 0 {
			if _, ok := selected[job.id]; !ok {
				continue
			}
			selected[job.id] = true
			if job.user != user {
				return nil, goipp.StatusErrorNotAuthorized
			}
		} else if job.user != user || job.isFinished() {
			continue
		}
		if !job.cancelable() {
			return nil, goipp.StatusErrorNotPossible
		}
		indexes = append(indexes, i)
	}
	for _, found := range selected {
		if !found {
			return nil, goipp.StatusErrorNotFound
		}
	}
	return indexes, goipp.StatusOk
}

// cancelJob moves a cancelable job to the canceled state and releases its
// timer, document and spooler context. Callers hold mu.
func (s *ippJobStore) cancelJob(job *ippJob) {
	job.stopTimer()
	s.discardDocument(job)
	job.state = jobStateCanceled
	job.completed = time.Now()
	job.abortSpooling()
}
