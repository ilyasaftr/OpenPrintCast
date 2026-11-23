package net

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// SharedState is the handoff between the UI thread (which owns the
// printer selection and the paused flag) and the network thread (which
// advertises printers and serves IPP).
//
// It composes the focused stores that each own one kind of state and
// their own locking.
type SharedState struct {
	printersMu sync.RWMutex
	printers   []core.PrinterInfo

	ippJobStore
	printerIdentity
	observations printerObservations
	listen       listenState
	identify     identifyThrottle
	uploads      uploadSlots
	active       activeJobs

	// Jobs is the bounded history appended by the IPP handler.
	Jobs *core.JobLog

	// Paused rejects new documents while set.
	Paused atomic.Bool

	// Spool renders accepted documents; nil means no backend is wired and
	// every Print-Job fails closed instead of pretending to print.
	Spool Spooler

	// changed wakes the network thread when the printer selection is
	// replaced; capacity 1 coalesces bursts of edits.
	changed chan struct{}
}

// NewSharedState builds shared state around printers; a nil log gets a
// fresh empty one so handlers never nil-dereference it.
func NewSharedState(printers []core.PrinterInfo, jobs *core.JobLog) *SharedState {
	if jobs == nil {
		jobs = &core.JobLog{}
	}
	return &SharedState{printers: printers, Jobs: jobs, changed: make(chan struct{}, 1), uploads: newUploadSlots(maxConcurrentUploads)}
}

// SetSpooler installs the OS spool backend (called once at startup).
func (s *SharedState) SetSpooler(spooler Spooler) { s.Spool = spooler }

// Changed signals that SetPrinters replaced the selection since the last
// receive. Signals coalesce, so a receiver re-reads EnabledPrinters.
func (s *SharedState) Changed() <-chan struct{} { return s.changed }

// SetPrinters replaces the shared printer selection.
func (s *SharedState) SetPrinters(printers []core.PrinterInfo) {
	s.printersMu.Lock()
	s.printers = printers
	s.printersMu.Unlock()
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

// PrinterByID returns the shared record for id, or false when unknown.
func (s *SharedState) PrinterByID(id string) (core.PrinterInfo, bool) {
	s.printersMu.RLock()
	defer s.printersMu.RUnlock()
	for _, p := range s.printers {
		if p.ID == id {
			return p, true
		}
	}
	return core.PrinterInfo{}, false
}

// EnabledPrinters snapshots the printers currently selected for sharing.
func (s *SharedState) EnabledPrinters() []core.PrinterInfo {
	s.printersMu.RLock()
	defer s.printersMu.RUnlock()
	var out []core.PrinterInfo
	for _, p := range s.printers {
		if p.Enabled {
			out = append(out, p)
		}
	}
	return out
}

// ListenStatus reports whether the IPP server is listening right now.
func (s *SharedState) ListenStatus() ListenStatus { return s.listen.current() }

// ObserveListen registers fn to be called, on the server's goroutine, after
// every change of the listening status.
func (s *SharedState) ObserveListen(fn func(ListenStatus)) { s.listen.observe(fn) }

// IsPaused reports whether sharing is paused.
func (s *SharedState) IsPaused() bool { return s.Paused.Load() }

// SetPaused flips the paused flag.
func (s *SharedState) SetPaused(paused bool) { s.Paused.Store(paused) }

// receiveIPPDocument stores a document through the job store, letting the
// installed spooler decide whether the job gets a cancellable context.
func (s *SharedState) receiveIPPDocument(printerID string, id uint32, doc []byte, format string, closeJob bool) (ippJob, []byte, goipp.Status) {
	_, cancellable := s.Spool.(cancellableSpooler)
	return s.receiveDocument(printerID, id, doc, format, closeJob, cancellable)
}

func (s *SharedState) observePrinter(id, config string, state int, reasons string) printerObservation {
	return s.observations.observe(id, config, state, reasons)
}

func (s *SharedState) allowIdentify() bool { return s.identify.allow() }

func (s *SharedState) beginUpload(ctx context.Context) bool { return s.uploads.acquire(ctx) }

func (s *SharedState) endUpload() { s.uploads.release() }
