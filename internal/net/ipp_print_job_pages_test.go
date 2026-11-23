package net

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// reportingSpooler is a cancellable and folder-capable spooler that reports
// pages through the context, as the real renderer does, and then optionally
// blocks until released or cancelled before returning err.
type reportingSpooler struct {
	pages    int
	err      error
	hold     bool
	reported chan struct{} // closed after the first report
	release  chan struct{} // ends a held spool
	calls    atomic.Int32
	once     sync.Once
}

func newReportingSpooler(pages int, hold bool) *reportingSpooler {
	return &reportingSpooler{pages: pages, hold: hold, reported: make(chan struct{}), release: make(chan struct{})}
}

func (s *reportingSpooler) SpoolBuffer(string, []byte, string) error { return nil }

func (s *reportingSpooler) SpoolJobContext(ctx context.Context, _ string, _ []byte, _ string, _ core.PrintOptions) error {
	s.calls.Add(1)
	core.ReportPageCount(ctx, s.pages)
	s.once.Do(func() { close(s.reported) })
	if !s.hold {
		return s.err
	}
	select {
	case <-s.release:
		return s.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *reportingSpooler) SpoolFolderJobContext(ctx context.Context, data []byte, mime string, options core.PrintOptions, _, _ string) error {
	return s.SpoolJobContext(ctx, core.FolderPrinterName, data, mime, options)
}

func sharedWith(spooler Spooler) *SharedState {
	shared := testShared()
	shared.SetSpooler(spooler)
	return shared
}

func TestPageCountShowsWhileJobIsStillPrinting(t *testing.T) {
	spooler := newReportingSpooler(12, true)
	shared := sharedWith(spooler)
	done := directPrintJob(shared, testPrinter(), []byte("%PDF-1.4"))
	<-spooler.reported
	if got := waitInFlight(t, shared); got.Pages != 12 || got.Status != core.JobPrinting {
		t.Fatalf("in-flight entry = %+v, want 12 pages while printing", got)
	}
	close(spooler.release)
	waitResponse(t, done)
	entries := shared.Jobs.List()
	if len(entries) != 1 || entries[0].Status != core.JobCompleted || entries[0].Pages != 12 {
		t.Fatalf("log = %+v, want one completed entry with 12 pages", entries)
	}
}

func TestPageCountIsNotMultipliedByCopies(t *testing.T) {
	spooler := newReportingSpooler(5, false)
	shared := sharedWith(spooler)
	req := goipp.NewRequest(ipp11, goipp.OpPrintJob, 1)
	req.Operation.Add(goipp.MakeAttr("ipp-attribute-fidelity", goipp.TagBoolean, goipp.Boolean(true)))
	req.Job.Add(goipp.MakeAttr("copies", goipp.TagInteger, goipp.Integer(3)))
	waitResponse(t, directPrint(shared, testPrinter(), req, []byte("%PDF-1.4")))
	entries := shared.Jobs.List()
	if spooler.calls.Load() != 3 || len(entries) != 1 || entries[0].Pages != 5 {
		t.Fatalf("spool calls = %d, log = %+v, want 3 copies and 5 pages", spooler.calls.Load(), entries)
	}
}

func TestCancelledJobKeepsReportedPageCount(t *testing.T) {
	spooler := newReportingSpooler(7, true)
	shared := sharedWith(spooler)
	done := directPrintJob(shared, testPrinter(), []byte("%PDF-1.4"))
	<-spooler.reported
	if !shared.CancelJob(waitInFlight(t, shared).ID) {
		t.Fatal("CancelJob = false for an in-flight job")
	}
	waitResponse(t, done)
	entries := shared.Jobs.List()
	if len(entries) != 1 || entries[0].Status != core.JobCancelled || entries[0].Pages != 7 {
		t.Fatalf("log = %+v, want one cancelled entry with 7 pages", entries)
	}
}

func TestFailedJobKeepsCountOnlyIfItWasReported(t *testing.T) {
	for name, tc := range map[string]struct{ pages, want int }{
		"failed before counting":  {0, 0},
		"failed after the report": {4, 4},
	} {
		t.Run(name, func(t *testing.T) {
			spooler := newReportingSpooler(tc.pages, false)
			spooler.err = context.DeadlineExceeded
			shared := sharedWith(spooler)
			waitResponse(t, directPrintJob(shared, testPrinter(), []byte("%PDF-1.4")))
			entries := shared.Jobs.List()
			if len(entries) != 1 || entries[0].Status != core.JobFailed || entries[0].Pages != tc.want {
				t.Fatalf("log = %+v, want one failed entry with %d pages", entries, tc.want)
			}
		})
	}
}

func TestEvictedEntryIsPushedAgainWithItsPageCount(t *testing.T) {
	spooler := newReportingSpooler(9, true)
	shared := sharedWith(spooler)
	done := directPrintJob(shared, testPrinter(), []byte("%PDF-1.4"))
	<-spooler.reported
	for i := 0; i < core.MaxLogEntries; i++ {
		shared.Jobs.Push(core.NewJobEntry("hp", "u", "filler.pdf", 1))
	}
	close(spooler.release)
	waitResponse(t, done)
	entries := shared.Jobs.List()
	last := entries[len(entries)-1]
	if last.Filename != "" || last.Status != core.JobCompleted || last.Pages != 9 {
		t.Fatalf("last entry = %+v, want the completed job with 9 pages", last)
	}
}

func TestReservedJobReportsPageCount(t *testing.T) {
	shared := sharedWith(newReportingSpooler(3, false))
	rsp := serveIPP(goipp.NewRequest(ipp11, goipp.OpPrintJob, 1), []byte("%PDF-1.4"), testPrinter(), shared, "pc", 631)
	waitIPPJob(t, shared, responseJobID(t, rsp))
	if entries := waitJobLog(t, shared, 1); entries[0].Pages != 3 {
		t.Fatalf("log = %+v, want 3 pages", entries)
	}
}

func TestFolderJobReportsPageCount(t *testing.T) {
	printer := testPrinter()
	printer.FolderPath = t.TempDir()
	shared := NewSharedState([]core.PrinterInfo{printer}, nil)
	shared.SetSpooler(newReportingSpooler(6, false))
	waitResponse(t, directPrintJob(shared, printer, []byte("%PDF-1.4")))
	if entries := shared.Jobs.List(); len(entries) != 1 || entries[0].Pages != 6 {
		t.Fatalf("log = %+v, want 6 pages", entries)
	}
}

func TestContextlessSpoolerLeavesPageCountUnknown(t *testing.T) {
	shared := testShared()
	waitResponse(t, directPrintJob(shared, testPrinter(), []byte("%PDF-1.4")))
	if entries := shared.Jobs.List(); len(entries) != 1 || entries[0].Status != core.JobCompleted || entries[0].Pages != 0 {
		t.Fatalf("log = %+v, want a completed entry with unknown pages", entries)
	}
}
