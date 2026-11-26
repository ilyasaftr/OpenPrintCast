package core

import "context"

// PageCounter receives the number of pages in the document being rendered.
// The renderer calls it the moment the count is known, before the first page
// is drawn, so a job that is still printing already shows its page count.
type PageCounter func(pages int)

// pageCounterKey is the context key under which a PageCounter is carried.
type pageCounterKey struct{}

// WithPageCounter returns a context that makes ReportPageCount call f.
//
// The counter travels in the context because it is request-scoped, optional
// information that has to cross the spooler interfaces and the renderer
// without widening either: a backend that never reports simply ignores it,
// and callers that do not care pass no counter. A nil f returns ctx
// unchanged.
func WithPageCounter(ctx context.Context, f PageCounter) context.Context {
	if f == nil {
		return ctx
	}
	return context.WithValue(ctx, pageCounterKey{}, f)
}

// ReportPageCount tells the PageCounter carried by ctx, if any, that the
// document has pages pages. It does nothing when ctx is nil, carries no
// counter, or pages is not positive. Reporting the same number repeatedly is
// harmless: a job with several copies spools the same document each time.
func ReportPageCount(ctx context.Context, pages int) {
	if ctx == nil || pages <= 0 {
		return
	}
	if f, ok := ctx.Value(pageCounterKey{}).(PageCounter); ok {
		f(pages)
	}
}

// ProgressReporter receives a heartbeat when the job makes visible
// progress: a page drawn, a document handed to the queue. The watchdog
// uses the absence of heartbeats to tell a wedged print pipeline apart
// from a slow one.
type ProgressReporter func()

// progressKey is the context key under which a ProgressReporter is carried.
type progressKey struct{}

// WithProgressReporter returns a context that makes ReportProgress call f.
// It travels in the context for the same reason as the PageCounter: the
// heartbeat is request-scoped and has to cross the spooler interfaces
// without widening them. A nil f returns ctx unchanged.
func WithProgressReporter(ctx context.Context, f ProgressReporter) context.Context {
	if f == nil {
		return ctx
	}
	return context.WithValue(ctx, progressKey{}, f)
}

// ReportProgress tells the ProgressReporter carried by ctx, if any, that
// the job advanced. It does nothing when ctx is nil or carries no
// reporter. Reporting is cheap and idempotent: callers report every page
// and every handoff rather than once per job.
func ReportProgress(ctx context.Context) {
	if ctx == nil {
		return
	}
	if f, ok := ctx.Value(progressKey{}).(ProgressReporter); ok {
		f()
	}
}
