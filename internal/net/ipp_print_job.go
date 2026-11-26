package net

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// printJobRun is the state of one Print-Job execution. reserved is set when
// the job was already accepted (Create-Job/Send-Document) and is now being
// rendered in the background.
type printJobRun struct {
	req      *goipp.Message
	doc      []byte
	printer  core.PrinterInfo
	shared   *SharedState
	host     string
	port     int
	reserved *ippJob
	echo     goipp.Version
	user     string
	filename string
	entry    core.JobEntry

	// ctx cancels spooling: the reserved job's context, or a run-local one
	// for a direct Print-Job on a backend that honors it; nil otherwise.
	ctx         context.Context
	stopContext context.CancelFunc
	unregister  func()
	// gate settles a race between a cancel request and the job finishing.
	gate cancelGate
	// logged is set once the in-flight entry is in the job log.
	logged bool
	// pages is the last page count the spooler reported (0 when none).
	pages atomic.Int64
}

// printJobResponse answers Print-Job: it reads the document bytes,
// honors the paused flag, and appends the outcome to the job log.
func printJobResponse(req *goipp.Message, doc []byte, printer core.PrinterInfo, shared *SharedState, host string, port int, reserved *ippJob) *goipp.Message {
	started := time.Now()
	run := newPrintJobRun(req, doc, printer, shared, host, port, reserved)
	defer run.release()
	if shared.IsPaused() && reserved == nil {
		componentLogger(componentSpool).Info("print job rejected: sharing is paused", "printer", printer.ID, "user", run.user)
		return run.fail(goipp.StatusErrorBusy, reasonPaused)
	}
	mime, copies, options, status := run.prepare()
	if status != goipp.StatusOk {
		return run.fail(status, rejectionReason(status))
	}
	componentLogger(componentSpool).Info("print job accepted",
		"printer", printer.ID, "user", run.user, "bytes", len(doc), "mime", mime, "copies", copies)
	if shared.Spool == nil {
		componentLogger(componentSpool).Error("print job failed: no spool backend wired", "printer", printer.ID)
		return run.fail(goipp.StatusErrorInternal, reasonNoBackend)
	}
	run.track()
	if rsp := run.spoolCopies(mime, copies, options); rsp != nil {
		return rsp
	}
	job, completed := run.recordCompletion(started)
	if !completed {
		return run.cancelled()
	}
	componentLogger(componentSpool).Info("print job completed",
		"printer", printer.ID, "user", run.user, "pages", run.pages.Load(),
		"duration", time.Since(started).Round(time.Millisecond))
	rsp := run.finish(core.JobCompleted, goipp.StatusOk)
	rsp.Job = ippJobAttrs(job, printer, host, port, shared)
	return rsp
}

func newPrintJobRun(req *goipp.Message, doc []byte, printer core.PrinterInfo, shared *SharedState, host string, port int, reserved *ippJob) *printJobRun {
	_, echo := NegotiateVersion(req.Version)
	run := &printJobRun{
		req: req, doc: doc, printer: printer, shared: shared, host: host, port: port, reserved: reserved,
		echo:     echo,
		user:     requestUser(req),
		filename: opAttrString(req.Operation, "job-name"),
	}
	if reserved != nil {
		run.user, run.filename = reserved.user, reserved.name
	}
	run.entry = core.NewJobEntry(printer.ID, run.user, run.filename, 0)
	run.ctx = reserved.cancelContext()
	return run
}

// finish logs the job with outcome and answers the client with status.
func (r *printJobRun) finish(outcome core.JobStatus, status goipp.Status) *goipp.Message {
	r.release()
	if r.gate.finish() {
		outcome, status, r.entry.Reason = core.JobCancelled, goipp.StatusErrorJobCanceled, reasonCancelled
	}
	r.entry.SetStatus(outcome)
	r.copyPagesToEntry()
	r.record()
	return statusResponse(r.echo, status, r.req.RequestID)
}

// record updates the in-flight entry, or appends the final entry when the
// job was never logged or its entry is gone from the log.
func (r *printJobRun) record() {
	if r.logged && r.shared.Jobs.Update(r.entry.ID, r.entry.Status, r.entry.Reason) {
		return
	}
	r.shared.Jobs.Push(r.entry)
}

// fail records reason and finishes the job as failed.
func (r *printJobRun) fail(status goipp.Status, reason string) *goipp.Message {
	r.entry.Reason = reason
	return r.finish(core.JobFailed, status)
}

// cancelled finishes the job as cancelled.
func (r *printJobRun) cancelled() *goipp.Message {
	r.entry.Reason = reasonCancelled
	return r.finish(core.JobCancelled, goipp.StatusErrorJobCanceled)
}

// prepare validates the document and job attributes and resolves the print
// options the spooler will receive.
func (r *printJobRun) prepare() (mime string, copies int, options core.PrintOptions, status goipp.Status) {
	declared := opAttrString(r.req.Operation, "document-format")
	decision := decideDocument(r.doc, declared)
	if !decision.accepted() {
		logDocumentDecision(goipp.Op(r.req.Code).String(), r.printer.ID, declared, r.doc, decision)
		return "", 0, options, decision.status
	}
	mime = decision.mime
	copies, valid := jobCopies(r.req)
	if !valid {
		return "", 0, options, goipp.StatusErrorAttributesOrValues
	}
	if r.printer.FolderPath != "" {
		// A folder keeps what was sent as one PDF file, however many copies.
		copies = 1
	}
	options, valid = jobOptions(r.req)
	if !valid {
		return "", 0, options, goipp.StatusErrorAttributesOrValues
	}
	options.PaperID = selectedPaper(r.req, r.shared, r.printer)
	if _, _, selectable := printQualities(r.shared, r.printer); !selectable && options.Quality == printQualityNormal {
		options.Quality = 0
	}
	if !r.printer.SupportsColor() {
		forceMonochrome(&options)
	}
	return mime, copies, options, goipp.StatusOk
}

// forceMonochrome pins the options to monochrome output.
func forceMonochrome(options *core.PrintOptions) {
	// Enforce the advertised monochrome capability even if a color
	// driver did not expose a reliable color-selection setting.
	options.Monochrome = true
	options.Color = false
	for i := range options.Overrides {
		options.Overrides[i].ColorMode = colorModeMonochrome
	}
}

// spoolCopies spools every copy and returns the failure response, or nil
// when all copies were accepted by the spooler.
func (r *printJobRun) spoolCopies(mime string, copies int, options core.PrintOptions) *goipp.Message {
	for i := 0; i < copies; i++ {
		err := r.spoolOnce(mime, options)
		if err == nil {
			continue
		}
		if r.ctx != nil && r.ctx.Err() != nil {
			return r.cancelled()
		}
		componentLogger(componentSpool).Error("print job failed while spooling",
			"printer", r.printer.ID, "user", r.user, "copy", i+1, "copies", copies,
			"windows_printer", r.printer.WindowsName, "err", err)
		return r.fail(goipp.StatusErrorInternal, spoolFailureReason(err))
	}
	return nil
}

// spoolOnce hands one copy to the richest interface the backend offers.
func (r *printJobRun) spoolOnce(mime string, options core.PrintOptions) error {
	spool, name := r.shared.Spool, r.printer.WindowsName
	ctx := r.ctx
	if r.printer.FolderPath != "" {
		capable, ok := spool.(folderJobSpooler)
		if !ok {
			return errNoFolderBackend
		}
		if ctx == nil {
			ctx = context.Background()
		}
		return capable.SpoolFolderJobContext(ctx, r.doc, mime, options, r.printer.FolderPath, r.filename)
	}
	if capable, ok := spool.(cancellableSpooler); ok && ctx != nil {
		return capable.SpoolJobContext(ctx, name, r.doc, mime, options)
	}
	if capable, ok := spool.(optionSpooler); ok {
		return capable.SpoolJob(name, r.doc, mime, options)
	}
	if options.IsDefault() {
		return spool.SpoolBuffer(name, r.doc, mime)
	}
	return errNoOptionsBackend
}

// recordCompletion marks the job completed: a new record for a direct
// Print-Job, or the processing -> completed transition of a reserved one.
// It reports false when the reserved job was cancelled meanwhile.
func (r *printJobRun) recordCompletion(started time.Time) (ippJob, bool) {
	if r.reserved != nil {
		return r.shared.transitionIPPJob(r.printer.ID, r.reserved.id, jobStateProcessing, jobStateCompleted)
	}
	if r.gate.finish() {
		return ippJob{}, false
	}
	job := newIPPJob(r.req, r.printer, r.host, r.port, jobStateCompleted)
	job.created, job.processing, job.completed = started, started, time.Now()
	r.shared.addIPPJob(job)
	return job, true
}

// cancelContext returns the cancellation context of a queued job, or nil.
func (j *ippJob) cancelContext() context.Context {
	if j == nil {
		return nil
	}
	return j.ctx
}
