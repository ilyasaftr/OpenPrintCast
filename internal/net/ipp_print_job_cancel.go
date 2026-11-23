package net

import (
	"context"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// CancelJob cancels the in-flight job with the given job-log id. It reports
// whether a cancel handle existed and stopped the job; it is false for
// unknown or finished jobs and for spoolers that cannot cancel. It never
// waits for the network or the spooler.
func (s *SharedState) CancelJob(id string) bool { return s.active.cancel(id) }

// track publishes the run as an in-flight Printing entry that can be
// cancelled until release is called.
func (r *printJobRun) track() {
	r.useDirectContext()
	r.countPages()
	r.unregister = r.shared.active.register(r.entry.ID, r.cancelHandle())
	r.entry.SetStatus(core.JobPrinting)
	r.shared.Jobs.Push(r.entry)
	r.logged = true
}

// release drops the cancel handle and frees the run-local context. It is
// idempotent and safe before track.
func (r *printJobRun) release() {
	if r.unregister != nil {
		r.unregister()
		r.unregister = nil
	}
	if r.stopContext != nil {
		r.stopContext()
	}
}

// useDirectContext gives a direct Print-Job its own cancellable context when
// the backend honors one.
func (r *printJobRun) useDirectContext() {
	if r.reserved != nil || !r.backendHonorsContext() {
		return
	}
	r.ctx, r.stopContext = context.WithCancel(context.Background())
}

// backendHonorsContext reports whether the spool path this run will take
// accepts a cancellation context.
func (r *printJobRun) backendHonorsContext() bool {
	if r.printer.FolderPath != "" {
		_, ok := r.shared.Spool.(folderJobSpooler)
		return ok
	}
	_, ok := r.shared.Spool.(cancellableSpooler)
	return ok
}

// cancelHandle returns the func that stops this run, or nil when the run
// cannot be cancelled.
func (r *printJobRun) cancelHandle() func() bool {
	if r.reserved != nil {
		return r.reservedCancelHandle()
	}
	if r.ctx == nil {
		return nil
	}
	return func() bool {
		if !r.gate.cancel() {
			return false
		}
		r.stopContext()
		return true
	}
}

// reservedCancelHandle cancels through the IPP job store so the IPP-visible
// state becomes canceled, as for an IPP Cancel-Job.
func (r *printJobRun) reservedCancelHandle() func() bool {
	if r.reserved.ctx == nil {
		return nil
	}
	printerID, jobID := r.printer.ID, r.reserved.id
	return func() bool {
		return r.shared.cancelIPPJob(printerID, jobID) == goipp.StatusOk
	}
}
