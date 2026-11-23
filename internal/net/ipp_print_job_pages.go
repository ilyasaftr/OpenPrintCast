package net

import "github.com/ilyasaftr/OpenPrintCast/internal/core"

// countPages makes the spool context carry a core.PageCounter, so a renderer
// that learns the document's page count reports it to the job log at once,
// while the job is still printing. A run without a context has nothing to
// carry the counter, so its page count stays unknown.
func (r *printJobRun) countPages() {
	if r.ctx != nil {
		r.ctx = core.WithPageCounter(r.ctx, r.recordPages)
	}
}

// recordPages is the run's core.PageCounter. The spooler may call it from
// its own goroutine and once per copy, so the count lives in an atomic field
// instead of r.entry, which only the run's goroutine touches.
func (r *printJobRun) recordPages(pages int) {
	r.pages.Store(int64(pages))
	r.shared.Jobs.SetPages(r.entry.ID, pages)
}

// copyPagesToEntry carries the last reported page count into r.entry, so the
// final record keeps it even when the in-flight entry is gone from the log
// and the entry is pushed again.
func (r *printJobRun) copyPagesToEntry() {
	if pages := int(r.pages.Load()); pages > 0 {
		r.entry.Pages = pages
	}
}
