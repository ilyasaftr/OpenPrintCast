package net

import (
	"time"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// admissionStatus decides whether the printer may take a new job right now.
// formatStatus is the verdict on the document format (goipp.StatusOk when the
// format may proceed).
func (c *ippRequest) admissionStatus(formatStatus goipp.Status, attributesOK bool) goipp.Status {
	switch {
	case c.shared.IsPaused():
		return goipp.StatusErrorBusy
	case !folderAvailable(c.printer):
		return goipp.StatusErrorNotAcceptingJobs
	case formatStatus != goipp.StatusOk:
		return formatStatus
	case !attributesOK:
		return goipp.StatusErrorAttributesOrValues
	}
	return goipp.StatusOk
}

// judgeDocument decides the request's document and logs a refusal or a
// mislabeled document.
func (c *ippRequest) judgeDocument() documentDecision {
	declared := opAttrString(c.req.Operation, "document-format")
	decision := decideDocument(c.doc, declared)
	logDocumentDecision(goipp.Op(c.req.Code).String(), c.printer.ID, declared, c.doc, decision)
	return decision
}

// judgeDeclaredFormat decides a request that carries no document (Create-Job,
// Validate-Job) by its declared format alone, and logs a refusal.
func (c *ippRequest) judgeDeclaredFormat() documentDecision {
	declared := opAttrString(c.req.Operation, "document-format")
	decision := decideDeclared(declared)
	logDocumentDecision(goipp.Op(c.req.Code).String(), c.printer.ID, declared, nil, decision)
	return decision
}

// newPendingJob numbers and registers a pending job for the request.
func (c *ippRequest) newPendingJob(copies int) (ippJob, bool) {
	job := newIPPJob(c.req, c.printer, c.host, c.port, jobStatePending)
	job.copies, job.created = copies, time.Now()
	return job, c.shared.addIPPJob(job)
}

func handlePrintJob(c *ippRequest) *goipp.Message {
	decision := c.judgeDocument()
	copies, validAttributes := validJobAttributes(c.req)
	status := c.admissionStatus(decision.status, validAttributes)
	if status == goipp.StatusOk && c.shared.Spool == nil {
		status = goipp.StatusErrorInternal
	}
	rsp := c.respond(status)
	if status != goipp.StatusOk {
		c.recordRejectedJob(status)
		return rsp
	}
	job, added := c.newPendingJob(copies)
	if !added {
		rsp.Code = goipp.Code(goipp.StatusErrorTooManyJobs)
		return rsp
	}
	id := job.id
	job, document, status := c.shared.receiveIPPDocument(c.printer.ID, id, c.doc, decision.mime, true)
	if status != goipp.StatusOk {
		c.shared.dropPendingIPPJob(c.printer.ID, id)
		rsp.Code = goipp.Code(status)
		return rsp
	}
	queueIPPJob(c.req, document, c.printer, c.shared, c.host, c.port, job)
	rsp.Job = ippJobAttrs(job, c.printer, c.host, c.port, c.shared)
	return rsp
}

// recordRejectedJob logs a Print-Job that never reached the spooler.
func (c *ippRequest) recordRejectedJob(status goipp.Status) {
	entry := core.NewJobEntry(c.printer.ID, opAttrString(c.req.Operation, "requesting-user-name"), opAttrString(c.req.Operation, "job-name"), 0)
	entry.SetStatus(core.JobFailed)
	entry.Reason = rejectionReason(status)
	c.shared.Jobs.Push(entry)
}

func handleCreateJob(c *ippRequest) *goipp.Message {
	copies, validAttributes := validJobAttributes(c.req)
	status := c.admissionStatus(c.judgeDeclaredFormat().status, validAttributes)
	rsp := c.respond(status)
	if status != goipp.StatusOk {
		return rsp
	}
	job, added := c.newPendingJob(copies)
	if !added {
		rsp.Code = goipp.Code(goipp.StatusErrorTooManyJobs)
		return rsp
	}
	rsp.Job = ippJobAttrs(job, c.printer, c.host, c.port, c.shared)
	return rsp
}

func handleValidateJob(c *ippRequest) *goipp.Message {
	if decision := c.judgeDeclaredFormat(); !decision.accepted() {
		return c.respond(decision.status)
	}
	if _, valid := validJobAttributes(c.req); !valid {
		return c.respond(goipp.StatusErrorAttributesOrValues)
	}
	return c.respond(goipp.StatusOk)
}

func handleSendDocument(c *ippRequest) *goipp.Message {
	last, valid := lastDocument(c.req)
	if !valid {
		return c.respond(goipp.StatusErrorBadRequest)
	}
	return c.receiveDocument(last)
}

func handleCloseJob(c *ippRequest) *goipp.Message {
	if len(c.doc) != 0 {
		return c.respond(goipp.StatusErrorBadRequest)
	}
	return c.receiveDocument(true)
}

// lastDocument reads the mandatory last-document flag; valid is false when
// it is missing, repeated or not a boolean.
func lastDocument(req *goipp.Message) (last, valid bool) {
	found := false
	for _, attr := range req.Operation {
		if attr.Name != "last-document" {
			continue
		}
		if found || !hasSingleValue(attr, goipp.TagBoolean) {
			return false, false
		}
		found = true
	}
	value := opAttrString(req.Operation, "last-document")
	return value == "true", found && (value == "true" || value == "false")
}

// receiveDocument attaches the request's document to its job, starting the
// job when closeJob is set.
func (c *ippRequest) receiveDocument(closeJob bool) *goipp.Message {
	format := ""
	if len(c.doc) > 0 {
		decision := c.judgeDocument()
		if !decision.accepted() {
			return c.respond(decision.status)
		}
		format = decision.mime
	}
	if !folderAvailable(c.printer) {
		return c.respond(goipp.StatusErrorNotAcceptingJobs)
	}
	if c.shared.IsPaused() || c.shared.Spool == nil {
		return c.respond(goipp.StatusErrorBusy)
	}
	job, document, status := c.shared.receiveIPPDocument(c.printer.ID, requestedJobID(c.req), c.doc, format, closeJob)
	if status == goipp.StatusOk && job.state == jobStateProcessing {
		queueIPPJob(c.req, document, c.printer, c.shared, c.host, c.port, job)
	}
	rsp := c.respond(status)
	if status == goipp.StatusOk {
		rsp.Job = ippJobAttrs(job, c.printer, c.host, c.port, c.shared)
	}
	return rsp
}
