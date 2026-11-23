package net

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

func jobTime(name string, value time.Time) goipp.Attribute {
	if value.IsZero() {
		return voidAttr(name, goipp.TagNoValue)
	}
	return intAttr(name, int(value.Sub(startedAt).Seconds())+1)
}

// isFinished reports whether the job reached a terminal state.
func (j ippJob) isFinished() bool { return j.state >= jobStateCanceled }

func jobURI(printer core.PrinterInfo, host string, port int, id uint32) string {
	return fmt.Sprintf("%s/job/%d", printerURI(printer, host, port), id)
}

// newIPPJob numbers a job for the request's user and name in the given state.
func newIPPJob(req *goipp.Message, printer core.PrinterInfo, host string, port int, state int) ippJob {
	id := nextJobID.Add(1)
	return ippJob{
		id:        id,
		printerID: printer.ID,
		uri:       jobURI(printer, host, port, id),
		user:      requestUser(req),
		name:      opAttrString(req.Operation, "job-name"),
		state:     state,
		template:  req.Job.DeepCopy(),
	}
}

func jobStateReason(state int) string {
	// The bridge's work is complete once Windows accepts the job. This does
	// not mean that the physical printer has finished printing.
	switch state {
	case jobStateCanceled:
		return "job-canceled-by-user"
	case jobStateAborted:
		return "aborted-by-system"
	case jobStateCompleted:
		return "job-completed-successfully"
	default:
		return "none"
	}
}

func ippJobAttrs(job ippJob, printer core.PrinterInfo, host string, port int) goipp.Attributes {
	attrs := goipp.Attributes{
		goipp.MakeAttr("job-id", goipp.TagInteger, goipp.Integer(int32(job.id))),
		stringsAttr("job-uri", goipp.TagURI, job.uri),
		stringsAttr("job-printer-uri", goipp.TagURI, printerURI(printer, host, port)),
		stringsAttr("job-name", goipp.TagName, job.name),
		stringsAttr("job-originating-user-name", goipp.TagName, job.user),
		goipp.MakeAttr("job-state", goipp.TagEnum, goipp.Integer(job.state)),
		keywordAttr("job-state-reasons", jobStateReason(job.state)),
		jobTime("time-at-creation", job.created),
		jobTime("time-at-processing", job.processing),
		jobTime("time-at-completed", job.completed),
		intAttr("job-printer-up-time", printerUptime()),
	}
	return append(attrs, job.template.DeepCopy()...)
}

func requestedJobID(req *goipp.Message) uint32 {
	if value := opAttrString(req.Operation, "job-id"); value != "" {
		return parseJobID(value)
	}
	if u, err := url.Parse(opAttrString(req.Operation, "job-uri")); err == nil {
		_, value, ok := strings.Cut(u.Path, "/job/")
		if ok && !strings.Contains(value, "/") {
			return parseJobID(value)
		}
	}
	return 0
}

// parseJobID reads a decimal job id; malformed text yields 0 (no such job).
func parseJobID(text string) uint32 {
	n, _ := strconv.ParseUint(text, 10, 32)
	return uint32(n)
}

// queueIPPJob returns control to the IPP client while rendering and the
// Windows spooler run. The document's memory remains accounted for until the
// worker has finished, including when cancellation is requested.
func queueIPPJob(req *goipp.Message, doc []byte, printer core.PrinterInfo, shared *SharedState, host string, port int, job ippJob) {
	copyReq := *req
	copyReq.Job = job.template.DeepCopy()
	copyReq.Operation = withDocumentFormat(req.Operation.DeepCopy(), job.format)
	go func() {
		defer shared.releaseIPPDocument(len(doc))
		rsp := printJobResponse(&copyReq, doc, printer, shared, host, port, &job)
		if goipp.Status(rsp.Code) != goipp.StatusOk {
			shared.transitionIPPJob(printer.ID, job.id, jobStateProcessing, jobStateAborted)
		}
	}()
}

// withDocumentFormat sets document-format in attrs, replacing any existing one.
func withDocumentFormat(attrs goipp.Attributes, format string) goipp.Attributes {
	replaced := false
	for i := range attrs {
		if attrs[i].Name == "document-format" {
			attrs[i] = stringsAttr("document-format", goipp.TagMimeType, format)
			replaced = true
		}
	}
	if !replaced {
		attrs.Add(stringsAttr("document-format", goipp.TagMimeType, format))
	}
	return attrs
}
