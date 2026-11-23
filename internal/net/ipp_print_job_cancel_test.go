package net

import (
	"context"
	"testing"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// gatedSpooler is cancellable and finishes a job only when released.
type gatedSpooler struct{ started, release chan struct{} }

func (s *gatedSpooler) SpoolBuffer(string, []byte, string) error { return nil }

func (s *gatedSpooler) SpoolJobContext(ctx context.Context, _ string, _ []byte, _ string, _ core.PrintOptions) error {
	close(s.started)
	select {
	case <-s.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// uncancellableSpooler blocks in SpoolBuffer and cannot honor a context.
type uncancellableSpooler struct{ started, release chan struct{} }

func (s *uncancellableSpooler) SpoolBuffer(string, []byte, string) error {
	close(s.started)
	<-s.release
	return nil
}

func newBlockingShared() (*SharedState, *blockingSpooler) {
	shared := testShared()
	spooler := &blockingSpooler{started: make(chan struct{})}
	shared.SetSpooler(spooler)
	return shared, spooler
}

func activeHandles(shared *SharedState) int {
	shared.active.mu.Lock()
	defer shared.active.mu.Unlock()
	return len(shared.active.cancels)
}

func TestCancelJobCancelsInFlightDirectPrintJob(t *testing.T) {
	shared, spooler := newBlockingShared()
	done := directPrintJob(shared, testPrinter(), []byte("%PDF-1.4"))
	<-spooler.started
	inflight := waitInFlight(t, shared)
	if !shared.CancelJob(inflight.ID) {
		t.Fatal("CancelJob = false for an in-flight cancellable job")
	}
	rsp := waitResponse(t, done)
	entries := shared.Jobs.List()
	if goipp.Status(rsp.Code) != goipp.StatusErrorJobCanceled {
		t.Fatalf("client status = %s", goipp.Status(rsp.Code))
	}
	if len(entries) != 1 || entries[0].Status != core.JobCancelled || entries[0].Reason != reasonCancelled {
		t.Fatalf("log = %+v, want one cancelled entry", entries)
	}
	if len(shared.ippJobs) != 0 {
		t.Fatalf("cancelled direct job created an IPP job: %+v", shared.ippJobs)
	}
}

func TestCancelJobCancelsInFlightDirectFolderJob(t *testing.T) {
	printer := testPrinter()
	printer.FolderPath = t.TempDir()
	spool := &folderSpooler{fakeSpooler: &fakeSpooler{}, started: make(chan string, 1), release: make(chan error, 1)}
	shared := NewSharedState([]core.PrinterInfo{printer}, nil)
	shared.SetSpooler(spool)
	done := directPrintJob(shared, printer, []byte("%PDF-1.4"))
	<-spool.started
	if !shared.CancelJob(waitInFlight(t, shared).ID) {
		t.Fatal("CancelJob = false for an in-flight folder job")
	}
	rsp := waitResponse(t, done)
	if goipp.Status(rsp.Code) != goipp.StatusErrorJobCanceled || shared.Jobs.List()[0].Status != core.JobCancelled {
		t.Fatalf("status = %s, log = %+v", goipp.Status(rsp.Code), shared.Jobs.List())
	}
}

func TestCancelJobCancelsInFlightReservedJob(t *testing.T) {
	shared, spooler := newBlockingShared()
	create := goipp.NewRequest(ipp11, goipp.OpCreateJob, 1)
	rsp := serveIPP(create, nil, testPrinter(), shared, "pc", 631)
	id := responseJobID(t, rsp)
	send := goipp.NewRequest(ipp11, goipp.OpSendDocument, 2)
	send.Operation.Add(goipp.MakeAttr("job-id", goipp.TagInteger, goipp.Integer(id)))
	send.Operation.Add(goipp.MakeAttr("last-document", goipp.TagBoolean, goipp.Boolean(true)))
	serveIPP(send, []byte("%PDF-1.4"), testPrinter(), shared, "pc", 631)
	<-spooler.started
	if !shared.CancelJob(waitInFlight(t, shared).ID) {
		t.Fatal("CancelJob = false for an in-flight reserved job")
	}
	entries := waitJobLog(t, shared, 1)
	if len(entries) != 1 || entries[0].Status != core.JobCancelled || entries[0].Reason != reasonCancelled {
		t.Fatalf("log = %+v, want one cancelled entry", entries)
	}
	get := goipp.NewRequest(ipp11, goipp.OpGetJobAttributes, 3)
	get.Operation.Add(goipp.MakeAttr("job-id", goipp.TagInteger, goipp.Integer(id)))
	rsp = serveIPP(get, nil, testPrinter(), shared, "pc", 631)
	if state := attrValues(t, rsp.Job, "job-state"); len(state) != 1 || state[0] != "7" {
		t.Fatalf("job-state = %v, want canceled (7)", state)
	}
}

func TestCancelJobFalseForUnknownAndFinishedJobs(t *testing.T) {
	shared := testShared()
	if shared.CancelJob("job-unknown") {
		t.Fatal("unknown id cancelled")
	}
	rsp := waitResponse(t, directPrintJob(shared, testPrinter(), []byte("%PDF-1.4")))
	entries := shared.Jobs.List()
	if goipp.Status(rsp.Code) != goipp.StatusOk || len(entries) != 1 {
		t.Fatalf("status = %s, log = %+v", goipp.Status(rsp.Code), entries)
	}
	if shared.CancelJob(entries[0].ID) {
		t.Fatal("finished job cancelled")
	}
}

func TestCancelJobFalseWhenBackendCannotCancel(t *testing.T) {
	shared := testShared()
	spooler := &uncancellableSpooler{started: make(chan struct{}), release: make(chan struct{})}
	shared.SetSpooler(spooler)
	done := directPrintJob(shared, testPrinter(), []byte("%PDF-1.4"))
	<-spooler.started
	inflight := waitInFlight(t, shared)
	if shared.CancelJob(inflight.ID) {
		t.Fatal("CancelJob = true for a backend that cannot cancel")
	}
	close(spooler.release)
	waitResponse(t, done)
	if got := shared.Jobs.List()[0]; got.Status != core.JobCompleted {
		t.Fatalf("entry = %+v, want completed", got)
	}
}

func TestCancelJobFalseForReservedJobWithoutContext(t *testing.T) {
	shared := testShared()
	spooler := &uncancellableSpooler{started: make(chan struct{}), release: make(chan struct{})}
	shared.SetSpooler(spooler)
	rsp := serveIPP(goipp.NewRequest(ipp11, goipp.OpPrintJob, 1), []byte("%PDF-1.4"), testPrinter(), shared, "pc", 631)
	<-spooler.started
	inflight := waitInFlight(t, shared)
	if shared.CancelJob(inflight.ID) {
		t.Fatal("CancelJob = true for a reserved job whose spooler cannot cancel")
	}
	close(spooler.release)
	waitIPPJob(t, shared, responseJobID(t, rsp))
}

func TestCancelHandleReleasedAfterFinish(t *testing.T) {
	shared, spooler := newBlockingShared()
	done := directPrintJob(shared, testPrinter(), []byte("%PDF-1.4"))
	<-spooler.started
	inflight := waitInFlight(t, shared)
	if got := activeHandles(shared); got != 1 {
		t.Fatalf("handles while in flight = %d, want 1", got)
	}
	shared.CancelJob(inflight.ID)
	waitResponse(t, done)
	if got := activeHandles(shared); got != 0 {
		t.Fatalf("handles after finish = %d, want 0", got)
	}
	if shared.CancelJob(inflight.ID) {
		t.Fatal("second CancelJob after finish returned true")
	}
}

func TestCancelJobRacesWithCompletion(t *testing.T) {
	for i := 0; i < 20; i++ {
		shared := testShared()
		spooler := &gatedSpooler{started: make(chan struct{}), release: make(chan struct{})}
		shared.SetSpooler(spooler)
		done := directPrintJob(shared, testPrinter(), []byte("%PDF-1.4"))
		<-spooler.started
		inflight := waitInFlight(t, shared)
		go close(spooler.release)
		cancelled := shared.CancelJob(inflight.ID)
		waitResponse(t, done)
		got := shared.Jobs.List()
		want := core.JobCompleted
		if cancelled {
			want = core.JobCancelled
		}
		if len(got) != 1 || got[0].Status != want {
			t.Fatalf("round %d: cancelled=%v log=%+v", i, cancelled, got)
		}
	}
}
