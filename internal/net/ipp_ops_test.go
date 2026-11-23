package net

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

type blockingSpooler struct{ started chan struct{} }

func (s *blockingSpooler) SpoolBuffer(string, []byte, string) error {
	return errors.New("uncancellable spool path")
}

func (s *blockingSpooler) SpoolJobContext(ctx context.Context, _ string, _ []byte, _ string, _ core.PrintOptions) error {
	close(s.started)
	<-ctx.Done()
	return ctx.Err()
}

func TestCancelMyJobsIsAtomicAndScoped(t *testing.T) {
	shared := testShared()
	for _, job := range []ippJob{
		{id: 1, printerID: "hp", user: "alice", state: 3, created: time.Now()},
		{id: 2, printerID: "hp", user: "bob", state: 3, created: time.Now()},
		{id: 3, printerID: "other", user: "alice", state: 3, created: time.Now()},
	} {
		if !shared.addIPPJob(job) {
			t.Fatal("failed to add job")
		}
	}
	req := goipp.NewRequest(ipp11, goipp.OpCancelMyJobs, 1)
	req.Operation.Add(goipp.MakeAttr("requesting-user-name", goipp.TagName, goipp.String("alice")))
	req.Operation.Add(goipp.MakeAttr("job-ids", goipp.TagInteger, goipp.Integer(1), goipp.Integer(2)))
	rsp := serveIPP(req, nil, testPrinter(), shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusErrorNotAuthorized {
		t.Fatalf("mixed-user status = %s", goipp.Status(rsp.Code))
	}
	if job, _ := shared.getIPPJob("hp", 1); job.state != 3 {
		t.Fatal("partially canceled an invalid selection")
	}
	req.Operation = req.Operation[:1]
	rsp = serveIPP(req, nil, testPrinter(), shared, "pc", 631)
	alice, _ := shared.getIPPJob("hp", 1)
	bob, _ := shared.getIPPJob("hp", 2)
	other, _ := shared.getIPPJob("other", 3)
	if goipp.Status(rsp.Code) != goipp.StatusOk || alice.state != 7 || bob.state != 3 || other.state != 3 {
		t.Fatalf("status %s; alice=%d bob=%d other=%d", goipp.Status(rsp.Code), alice.state, bob.state, other.state)
	}
	shared.transitionIPPJob("hp", 2, 3, 7)
	shared.transitionIPPJob("other", 3, 3, 7)
}

func TestCancelProcessingJob(t *testing.T) {
	shared := testShared()
	spooler := &blockingSpooler{started: make(chan struct{})}
	shared.SetSpooler(spooler)
	create := goipp.NewRequest(ipp11, goipp.OpCreateJob, 1)
	create.Operation.Add(goipp.MakeAttr("requesting-user-name", goipp.TagName, goipp.String("alice")))
	rsp := serveIPP(create, nil, testPrinter(), shared, "pc", 631)
	id, _ := strconv.Atoi(attrValues(t, rsp.Job, "job-id")[0])
	defer shared.cancelIPPJob("hp", uint32(id))
	send := goipp.NewRequest(ipp11, goipp.OpSendDocument, 2)
	send.Operation.Add(goipp.MakeAttr("job-id", goipp.TagInteger, goipp.Integer(id)))
	send.Operation.Add(goipp.MakeAttr("last-document", goipp.TagBoolean, goipp.Boolean(true)))
	rsp = serveIPP(send, []byte("%PDF-1.4"), testPrinter(), shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusOk {
		t.Fatalf("send status = %s", goipp.Status(rsp.Code))
	}
	select {
	case <-spooler.started:
	case <-time.After(5 * time.Second):
		t.Fatal("spool did not start")
	}
	cancel := goipp.NewRequest(ipp11, goipp.OpCancelMyJobs, 3)
	cancel.Operation.Add(goipp.MakeAttr("requesting-user-name", goipp.TagName, goipp.String("alice")))
	rsp = serveIPP(cancel, nil, testPrinter(), shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusOk {
		t.Fatalf("cancel status = %s", goipp.Status(rsp.Code))
	}
	job, _ := shared.getIPPJob("hp", uint32(id))
	entries := waitJobLog(t, shared, 1)
	if job.state != 7 || len(entries) != 1 || entries[0].Status != core.JobCancelled {
		t.Fatalf("job = %+v, log = %+v", job, entries)
	}
}

func TestIdentifyPrinter(t *testing.T) {
	shared := testShared()
	req := goipp.NewRequest(ipp11, goipp.OpIdentifyPrinter, 1)
	req.Operation.Add(goipp.MakeAttr("identify-actions", goipp.TagKeyword, goipp.String("display")))
	rsp := serveIPP(req, nil, testPrinter(), shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusErrorAttributesOrValues || shared.Spool.(*fakeSpooler).identified != 0 {
		t.Fatalf("unsupported action status = %s", goipp.Status(rsp.Code))
	}
	req.Operation = nil
	rsp = serveIPP(req, nil, testPrinter(), shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusOk || shared.Spool.(*fakeSpooler).identified != 1 {
		t.Fatalf("identify status = %s", goipp.Status(rsp.Code))
	}
	rsp = serveIPP(req, nil, testPrinter(), shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusErrorBusy {
		t.Fatalf("rate limit status = %s", goipp.Status(rsp.Code))
	}
}
