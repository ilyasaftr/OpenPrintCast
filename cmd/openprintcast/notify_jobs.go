package main

import (
	"sync/atomic"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// Wording of the job-failure notice. The constant ID makes a new failure
// replace one still shown, which also collapses bursts into one notice.
const (
	jobFailureNoticeID  = "job-failed"
	jobFailureTitle     = "Print job failed"
	unnamedDocument     = "A document"
	unknownFailureCause = "Something went wrong."
)

// printerNamer resolves a printer id to its record; *SharedState does.
type printerNamer interface {
	PrinterByID(id string) (core.PrinterInfo, bool)
}

// pausedChecker reports whether sharing is paused; *SharedState does.
type pausedChecker interface {
	IsPaused() bool
}

// jobFailureAnnouncer decides whether a finished job deserves a
// notification and words it. OnJob runs on the server goroutine, so the
// enabled flag is atomic and nothing UI-owned is read.
type jobFailureAnnouncer struct {
	notifier notifier
	printers printerNamer
	paused   pausedChecker
	enabled  atomic.Bool
}

// newJobFailureAnnouncer builds an announcer that starts with the given
// enabled state.
func newJobFailureAnnouncer(n notifier, printers printerNamer, paused pausedChecker, enabled bool) *jobFailureAnnouncer {
	a := &jobFailureAnnouncer{notifier: n, printers: printers, paused: paused}
	a.enabled.Store(enabled)
	return a
}

// SetEnabled turns failure notifications on or off.
func (a *jobFailureAnnouncer) SetEnabled(enabled bool) { a.enabled.Store(enabled) }

// OnJob is the JobLog observer: it notifies only for failed jobs, only
// when enabled, and not while sharing is paused (rejections are expected).
func (a *jobFailureAnnouncer) OnJob(job core.JobEntry) {
	if job.Status != core.JobFailed || !a.enabled.Load() || a.paused.IsPaused() {
		return
	}
	a.notifier.Notify(userNotice{ID: jobFailureNoticeID, Title: jobFailureTitle, Body: a.failureBody(job)})
}

// failureBody reads "<document> on <printer>: <reason>".
func (a *jobFailureAnnouncer) failureBody(job core.JobEntry) string {
	return orDefault(job.Filename, unnamedDocument) + " on " + a.printerLabel(job.PrinterID) + ": " +
		orDefault(job.Reason, unknownFailureCause)
}

// printerLabel is the printer's display name, or the raw id when unknown.
func (a *jobFailureAnnouncer) printerLabel(id string) string {
	if p, ok := a.printers.PrinterByID(id); ok {
		return p.DisplayName()
	}
	return id
}

// orDefault returns value, or fallback when value is empty.
func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
