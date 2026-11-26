package net

import (
	"strconv"

	"github.com/OpenPrinting/goipp"
)

// which-jobs values (RFC 8011 section 4.2.6.1).
const (
	whichJobsAll          = "all"
	whichJobsCompleted    = "completed"
	whichJobsNotCompleted = "not-completed"
)

func handleCancelJob(c *ippRequest) *goipp.Message {
	return c.respond(c.shared.cancelIPPJob(c.printer.ID, requestedJobID(c.req)))
}

func handleCancelMyJobs(c *ippRequest) *goipp.Message {
	ids, status := requestedJobIDList(c.req)
	if status == goipp.StatusOk {
		status = c.shared.cancelMyIPPJobs(c.printer.ID, requestUser(c.req), ids)
	}
	return c.respond(status)
}

// requestedJobIDList reads the optional job-ids list of Cancel-My-Jobs.
func requestedJobIDList(req *goipp.Message) ([]uint32, goipp.Status) {
	var ids []uint32
	for _, attr := range req.Operation {
		if attr.Name != "job-ids" {
			continue
		}
		if len(attr.Values) == 0 {
			return nil, goipp.StatusErrorBadRequest
		}
		for _, value := range attr.Values {
			id, ok := value.V.(goipp.Integer)
			if !ok || value.T != goipp.TagInteger || id <= 0 {
				return nil, goipp.StatusErrorBadRequest
			}
			ids = append(ids, uint32(id))
		}
	}
	return ids, goipp.StatusOk
}

func handleGetJobAttributes(c *ippRequest) *goipp.Message {
	job, ok := c.shared.getIPPJob(c.printer.ID, requestedJobID(c.req))
	if !ok {
		return c.respond(goipp.StatusErrorNotFound)
	}
	rsp := c.respond(goipp.StatusOk)
	rsp.Job = requestedAttributes(c.req, ippJobAttrs(job, c.printer, c.host, c.port, c.shared), nil)
	return rsp
}

// jobFilter holds the Get-Jobs selection criteria.
type jobFilter struct {
	which    string
	onlyMine bool
	user     string
	limit    int
}

func newJobFilter(req *goipp.Message) jobFilter {
	limit, _ := strconv.Atoi(opAttrString(req.Operation, "limit"))
	return jobFilter{
		which:    opAttrString(req.Operation, "which-jobs"),
		onlyMine: opAttrString(req.Operation, "my-jobs") == "true",
		user:     opAttrString(req.Operation, "requesting-user-name"),
		limit:    limit,
	}
}

// accepts reports whether job should be listed after selected jobs.
func (f jobFilter) accepts(job ippJob, selected int) bool {
	if f.onlyMine && job.user != f.user || f.limit > 0 && selected >= f.limit {
		return false
	}
	switch f.which {
	case whichJobsAll:
		return true
	case whichJobsCompleted:
		return job.isFinished()
	case "", whichJobsNotCompleted:
		return !job.isFinished()
	default:
		return false
	}
}

func handleGetJobs(c *ippRequest) *goipp.Message {
	rsp := c.respond(goipp.StatusOk)
	filter := newJobFilter(c.req)
	selected := 0
	rsp.Groups = append(rsp.Groups, goipp.Group{Tag: goipp.TagOperationGroup, Attrs: rsp.Operation})
	for _, job := range c.shared.ippJobsFor(c.printer.ID) {
		if !filter.accepts(job, selected) {
			continue
		}
		attrs := ippJobAttrs(job, c.printer, c.host, c.port, c.shared)
		defaults := attrs[:2]
		attrs = requestedAttributes(c.req, attrs, defaults)
		rsp.Groups = append(rsp.Groups, goipp.Group{Tag: goipp.TagJobGroup, Attrs: attrs})
		selected++
	}
	return rsp
}
