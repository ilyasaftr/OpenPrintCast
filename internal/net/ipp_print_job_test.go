package net

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

func TestPrintJobSpoolFailure(t *testing.T) {
	shared := NewSharedState([]core.PrinterInfo{testPrinter()}, &core.JobLog{})
	shared.SetSpooler(&fakeSpooler{err: errors.New("spooler down")})
	req := goipp.NewRequest(ipp11, goipp.OpPrintJob, 3)
	req.Operation.Add(goipp.MakeAttr("requesting-user-name",
		goipp.TagName, goipp.String("alice")))
	rsp := serveIPP(req, []byte("%PDF-1.4"), testPrinter(), shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusOk {
		t.Fatalf("accepted spool status = %s", goipp.Status(rsp.Code))
	}
	if job := waitIPPJob(t, shared, responseJobID(t, rsp)); job.state != 8 {
		t.Fatalf("failed job state = %d", job.state)
	}
	jobs := shared.Jobs.List()
	if len(jobs) != 1 || jobs[0].Status != core.JobFailed {
		t.Fatalf("failed job log = %+v, want one failed entry", jobs)
	}
	if jobs[0].Reason != "spooler down" {
		t.Fatalf("failed job reason = %q, want the spool error text", jobs[0].Reason)
	}
}

func TestPrintJobRejectsUnsupportedFormats(t *testing.T) {
	shared := testShared()
	spooler := shared.Spool.(*fakeSpooler)
	req := goipp.NewRequest(ipp11, goipp.OpPrintJob, 3)
	req.Operation.Add(goipp.MakeAttr("document-format", goipp.TagMimeType, goipp.String("image/tiff")))
	rsp := serveIPP(req, []byte("II\x2a\x00"), testPrinter(), shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusErrorDocumentFormatNotSupported || spooler.calls != 0 {
		t.Fatalf("status=%s spool calls=%d", goipp.Status(rsp.Code), spooler.calls)
	}
	req.Code = goipp.Code(goipp.OpValidateJob)
	rsp = serveIPP(req, nil, testPrinter(), shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusErrorDocumentFormatNotSupported {
		t.Fatalf("validate status=%s", goipp.Status(rsp.Code))
	}
}

func TestPrintJobAcceptsRasterFormats(t *testing.T) {
	for _, tc := range []struct{ name, mime string }{
		{"cups-color.urf", "image/urf"},
		{"cups-gray.pwg", "image/pwg-raster"},
	} {
		t.Run(tc.mime, func(t *testing.T) {
			doc, err := os.ReadFile(filepath.Join("..", "winspool", "testdata", tc.name))
			if err != nil {
				t.Fatal(err)
			}
			shared := testShared()
			req := goipp.NewRequest(ipp11, goipp.OpPrintJob, 1)
			req.Operation.Add(goipp.MakeAttr("document-format", goipp.TagMimeType, goipp.String(tc.mime)))
			rsp := serveIPP(req, doc, testPrinter(), shared, "pc", 631)
			waitIPPJob(t, shared, responseJobID(t, rsp))
			spooler := shared.Spool.(*fakeSpooler)
			if goipp.Status(rsp.Code) != goipp.StatusOk || spooler.calls != 1 || !strings.Contains(spooler.last, "|"+tc.mime+"|") {
				t.Fatalf("status=%s spooler=%+v", goipp.Status(rsp.Code), spooler)
			}
		})
	}
}

func TestPrintJobCopies(t *testing.T) {
	shared := testShared()
	req := goipp.NewRequest(ipp11, goipp.OpPrintJob, 3)
	req.Operation.Add(goipp.MakeAttr("ipp-attribute-fidelity", goipp.TagBoolean, goipp.Boolean(true)))
	req.Job.Add(goipp.MakeAttr("copies", goipp.TagInteger, goipp.Integer(2)))
	rsp := serveIPP(req, []byte("%PDF-1.4"), testPrinter(), shared, "pc", 631)
	waitIPPJob(t, shared, responseJobID(t, rsp))
	if goipp.Status(rsp.Code) != goipp.StatusOk || shared.Spool.(*fakeSpooler).calls != 2 {
		t.Fatalf("status=%s calls=%d", goipp.Status(rsp.Code), shared.Spool.(*fakeSpooler).calls)
	}
	req.Job = nil
	req.Job.Add(goipp.MakeAttr("copies", goipp.TagInteger, goipp.Integer(100)))
	rsp = serveIPP(req, []byte("%PDF-1.4"), testPrinter(), shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusErrorAttributesOrValues || shared.Spool.(*fakeSpooler).calls != 2 {
		t.Fatalf("status=%s calls=%d", goipp.Status(rsp.Code), shared.Spool.(*fakeSpooler).calls)
	}
}

func TestUnknownColorCapabilityPrintsMonochrome(t *testing.T) {
	printer := testPrinter()
	printer.ColorCapable = nil
	shared := testShared()
	req := goipp.NewRequest(ipp11, goipp.OpPrintJob, 1)
	rsp := serveIPP(req, []byte("%PDF-1.4"), printer, shared, "pc", 631)
	waitIPPJob(t, shared, responseJobID(t, rsp))
	if !shared.Spool.(*fakeSpooler).lastOptions.Monochrome {
		t.Fatal("unknown capability should be printed as advertised: monochrome")
	}
}

func TestPrintJobPauseAndSpool(t *testing.T) {
	shared := testShared()
	p := testPrinter()
	newPrintReq := func() *goipp.Message {
		req := goipp.NewRequest(ipp11, goipp.OpPrintJob, 3)
		req.Operation.Add(goipp.MakeAttr("requesting-user-name",
			goipp.TagName, goipp.String("alice")))
		req.Operation.Add(goipp.MakeAttr("job-name",
			goipp.TagName, goipp.String("doc.pdf")))
		return req
	}
	shared.SetPaused(true)
	rsp := serveIPP(newPrintReq(), []byte("%PDF-1.4"), p, shared, "pc", 631)
	if goipp.Status(rsp.Code) == goipp.StatusOk {
		t.Error("paused Print-Job returned success, want rejection")
	}
	jobs := shared.Jobs.List()
	if len(jobs) != 1 || jobs[0].Status != core.JobFailed || jobs[0].Reason != reasonPaused {
		t.Fatalf("paused job log = %+v, want one failed entry with the paused reason", jobs)
	}
	shared.SetPaused(false)
	rsp = serveIPP(newPrintReq(), []byte("%PDF-1.4"), p, shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusOk {
		t.Fatalf("resumed Print-Job status %s, want successful-ok", goipp.Status(rsp.Code))
	}
	waitIPPJob(t, shared, responseJobID(t, rsp))
	jobs = waitJobLog(t, shared, 2)
	if len(jobs) != 2 || jobs[1].Status != core.JobCompleted {
		t.Fatalf("resumed job log = %+v, want completed entry", jobs)
	}
	if jobs[1].User != "alice" || jobs[1].Filename != "doc.pdf" || jobs[1].PrinterID != "hp" {
		t.Errorf("job entry = %+v, want user/filename/printer carried over", jobs[1])
	}
}

// directPrintJob runs printJobResponse without a reserved IPP job, as for a
// Print-Job that is spooled in the calling goroutine.
func directPrintJob(shared *SharedState, printer core.PrinterInfo, doc []byte) <-chan *goipp.Message {
	return directPrint(shared, printer, goipp.NewRequest(ipp11, goipp.OpPrintJob, 1), doc)
}

func directPrint(shared *SharedState, printer core.PrinterInfo, req *goipp.Message, doc []byte) <-chan *goipp.Message {
	done := make(chan *goipp.Message, 1)
	go func() { done <- printJobResponse(req, doc, printer, shared, "pc", 631, nil) }()
	return done
}

// waitInFlight waits for the log to show a printing entry and returns it.
func waitInFlight(t *testing.T, shared *SharedState) core.JobEntry {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		for _, e := range shared.Jobs.List() {
			if e.Status == core.JobPrinting {
				return e
			}
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("no in-flight job in the log: %+v", shared.Jobs.List())
	return core.JobEntry{}
}

func waitResponse(t *testing.T, done <-chan *goipp.Message) *goipp.Message {
	t.Helper()
	select {
	case rsp := <-done:
		return rsp
	case <-time.After(5 * time.Second):
		t.Fatal("print job did not finish")
		return nil
	}
}

func TestPrintJobIsPrintingWhileSpoolingThenCompleted(t *testing.T) {
	shared := testShared()
	spooler := &gatedSpooler{started: make(chan struct{}), release: make(chan struct{})}
	shared.SetSpooler(spooler)
	req := goipp.NewRequest(ipp11, goipp.OpPrintJob, 1)
	rsp := serveIPP(req, []byte("%PDF-1.4"), testPrinter(), shared, "pc", 631)
	<-spooler.started
	inflight := waitInFlight(t, shared)
	if entries := shared.Jobs.List(); len(entries) != 1 || inflight.Reason != "" {
		t.Fatalf("log while spooling = %+v", entries)
	}
	close(spooler.release)
	waitIPPJob(t, shared, responseJobID(t, rsp))
	entries := waitJobLog(t, shared, 1)
	if len(entries) != 1 || entries[0].ID != inflight.ID || entries[0].Status != core.JobCompleted || entries[0].Reason != "" {
		t.Fatalf("log after completion = %+v, want the same entry completed", entries)
	}
}

func TestDirectPrintJobIsPrintingThenCompleted(t *testing.T) {
	shared := testShared()
	spooler := &uncancellableSpooler{started: make(chan struct{}), release: make(chan struct{})}
	shared.SetSpooler(spooler)
	done := directPrintJob(shared, testPrinter(), []byte("%PDF-1.4"))
	<-spooler.started
	inflight := waitInFlight(t, shared)
	close(spooler.release)
	rsp := waitResponse(t, done)
	entries := shared.Jobs.List()
	if goipp.Status(rsp.Code) != goipp.StatusOk || len(entries) != 1 || entries[0].ID != inflight.ID || entries[0].Status != core.JobCompleted {
		t.Fatalf("status = %s, log = %+v", goipp.Status(rsp.Code), entries)
	}
}

func TestDirectPrintJobUnsupportedFormatRecordsReason(t *testing.T) {
	shared := testShared()
	req := goipp.NewRequest(ipp11, goipp.OpPrintJob, 1)
	req.Operation.Add(goipp.MakeAttr("document-format", goipp.TagMimeType, goipp.String("image/tiff")))
	rsp := waitResponse(t, directPrint(shared, testPrinter(), req, []byte("II\x2a\x00")))
	entries := shared.Jobs.List()
	if goipp.Status(rsp.Code) != goipp.StatusErrorDocumentFormatNotSupported || len(entries) != 1 {
		t.Fatalf("status = %s, log = %+v", goipp.Status(rsp.Code), entries)
	}
	if entries[0].Status != core.JobFailed || entries[0].Reason != reasonUnsupportedFormat {
		t.Fatalf("entry = %+v, want failed with the unsupported format reason", entries[0])
	}
}

func TestRejectedPrintJobRecordsReason(t *testing.T) {
	shared := testShared()
	req := goipp.NewRequest(ipp11, goipp.OpPrintJob, 1)
	req.Operation.Add(goipp.MakeAttr("document-format", goipp.TagMimeType, goipp.String("image/tiff")))
	serveIPP(req, []byte("II\x2a\x00"), testPrinter(), shared, "pc", 631)
	entries := shared.Jobs.List()
	if len(entries) != 1 || entries[0].Status != core.JobFailed || entries[0].Reason != reasonUnsupportedFormat {
		t.Fatalf("rejected job log = %+v", entries)
	}
}

func TestPrintJobWithoutBackendRecordsReason(t *testing.T) {
	shared := NewSharedState([]core.PrinterInfo{testPrinter()}, nil)
	req := goipp.NewRequest(ipp11, goipp.OpPrintJob, 1)
	rsp := waitResponse(t, directPrint(shared, testPrinter(), req, []byte("%PDF-1.4")))
	entries := shared.Jobs.List()
	if goipp.Status(rsp.Code) != goipp.StatusErrorInternal || len(entries) != 1 || entries[0].Reason != reasonNoBackend {
		t.Fatalf("status = %s, log = %+v", goipp.Status(rsp.Code), entries)
	}
}

func TestDirectPrintJobSpoolFailureRecordsReason(t *testing.T) {
	shared := testShared()
	shared.SetSpooler(&fakeSpooler{err: errors.New(strings.Repeat("x", 500))})
	rsp := waitResponse(t, directPrintJob(shared, testPrinter(), []byte("%PDF-1.4")))
	entries := shared.Jobs.List()
	if goipp.Status(rsp.Code) != goipp.StatusErrorInternal || len(entries) != 1 {
		t.Fatalf("status = %s, log = %+v", goipp.Status(rsp.Code), entries)
	}
	if entries[0].Status != core.JobFailed || len([]rune(entries[0].Reason)) != maxReasonRunes {
		t.Fatalf("entry = %+v, want failed with a truncated reason", entries[0])
	}
}

func TestPrintJobLogsEntryOnceWhenEvictedWhileSpooling(t *testing.T) {
	shared := testShared()
	spooler := &uncancellableSpooler{started: make(chan struct{}), release: make(chan struct{})}
	shared.SetSpooler(spooler)
	done := directPrintJob(shared, testPrinter(), []byte("%PDF-1.4"))
	<-spooler.started
	inflight := waitInFlight(t, shared)
	for i := 0; i < core.MaxLogEntries; i++ {
		shared.Jobs.Push(core.NewJobEntry("hp", "u", "f", 1))
	}
	close(spooler.release)
	waitResponse(t, done)
	entries := shared.Jobs.List()
	last := entries[len(entries)-1]
	if len(entries) != core.MaxLogEntries || last.ID != inflight.ID || last.Status != core.JobCompleted {
		t.Fatalf("evicted entry not re-logged as the final one: last = %+v, n = %d", last, len(entries))
	}
}
