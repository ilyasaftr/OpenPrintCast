package net

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

type folderSpooler struct {
	*fakeSpooler
	started chan string
	release chan error
}

func (s *folderSpooler) SpoolFolderJobContext(ctx context.Context, data []byte, mime string, options core.PrintOptions, folder, jobName string) error {
	s.started <- mime + "|" + folder + "|" + jobName
	select {
	case err := <-s.release:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestFolderJobUsesFolderBackendAndWaitsForCompletion(t *testing.T) {
	printer := testPrinter()
	printer.FolderPath = t.TempDir()
	spool := &folderSpooler{fakeSpooler: &fakeSpooler{}, started: make(chan string, 1), release: make(chan error, 1)}
	shared := NewSharedState([]core.PrinterInfo{printer}, nil)
	shared.SetSpooler(spool)
	req := goipp.NewRequest(ipp11, goipp.OpPrintJob, 1)
	req.Operation.Add(goipp.MakeAttr("job-name", goipp.TagName, goipp.String("original.pdf")))
	rsp := serveIPP(req, []byte("%PDF-1.4"), printer, shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusOk {
		t.Fatalf("submit: %v", goipp.Status(rsp.Code))
	}
	id := responseJobID(t, rsp)
	if got := <-spool.started; got != "application/pdf|"+printer.FolderPath+"|original.pdf" {
		t.Fatalf("folder spool arguments: %q", got)
	}
	job, ok := shared.getIPPJob(printer.ID, id)
	if !ok || job.state != 5 {
		t.Fatalf("job completed before folder output: %+v", job)
	}
	spool.release <- nil
	if job := waitIPPJob(t, shared, id); job.state != 9 {
		t.Fatalf("folder job state = %d", job.state)
	}
	if spool.calls != 0 {
		t.Fatal("folder job fell back to ordinary printer spooling")
	}
}

func TestFolderUnavailableRejectsJobs(t *testing.T) {
	printer := testPrinter()
	printer.FolderPath = filepath.Join(t.TempDir(), "missing")
	shared := NewSharedState([]core.PrinterInfo{printer}, nil)
	shared.SetSpooler(&fakeSpooler{})
	attrs := printerAttrsResponse(goipp.NewRequest(ipp11, goipp.OpGetPrinterAttributes, 1), printer, shared, "pc", 631)
	if got := attrValues(t, attrs.Printer, "printer-is-accepting-jobs"); len(got) != 1 || got[0] != "false" {
		t.Fatalf("unavailable folder accepting jobs: %v", got)
	}
	for _, op := range []goipp.Op{goipp.OpPrintJob, goipp.OpCreateJob} {
		req := goipp.NewRequest(ipp11, op, 1)
		rsp := serveIPP(req, []byte("%PDF-1.4"), printer, shared, "pc", 631)
		if goipp.Status(rsp.Code) != goipp.StatusErrorNotAcceptingJobs {
			t.Fatalf("%s status = %v", op, goipp.Status(rsp.Code))
		}
	}
}

func TestFolderJobFailsWhenOutputFails(t *testing.T) {
	printer := testPrinter()
	printer.FolderPath = t.TempDir()
	spool := &folderSpooler{fakeSpooler: &fakeSpooler{}, started: make(chan string, 1), release: make(chan error, 1)}
	shared := NewSharedState([]core.PrinterInfo{printer}, nil)
	shared.SetSpooler(spool)
	req := goipp.NewRequest(ipp11, goipp.OpPrintJob, 1)
	rsp := serveIPP(req, []byte("%PDF-1.4"), printer, shared, "pc", 631)
	id := responseJobID(t, rsp)
	<-spool.started
	spool.release <- errors.New("cannot write output")
	if job := waitIPPJob(t, shared, id); job.state != 8 {
		t.Fatalf("failed folder job state = %d", job.state)
	}
	if jobs := waitJobLog(t, shared, 1); jobs[0].Status != core.JobFailed {
		t.Fatalf("failed folder job log = %+v", jobs)
	}
}
