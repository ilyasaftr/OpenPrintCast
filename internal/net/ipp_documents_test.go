package net

import (
	"strconv"
	"testing"
	"time"

	"github.com/OpenPrinting/goipp"
)

func TestQueuedDocumentsAreBounded(t *testing.T) {
	shared := testShared()
	shared.processingBytes = maxPendingDocumentBytes
	req := goipp.NewRequest(ipp11, goipp.OpPrintJob, 1)
	rsp := serveIPP(req, []byte("%PDF-1.4"), testPrinter(), shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusErrorBusy || len(shared.ippJobsFor("hp")) != 0 {
		t.Fatalf("full queue: status=%s jobs=%v", goipp.Status(rsp.Code), shared.ippJobsFor("hp"))
	}
}

func TestCreateSendAndCancelJob(t *testing.T) {
	shared := testShared()
	printer := testPrinter()
	create := func() uint32 {
		req := goipp.NewRequest(ipp11, goipp.OpCreateJob, 1)
		req.Job.Add(goipp.MakeAttr("copies", goipp.TagInteger, goipp.Integer(2)))
		rsp := serveIPP(req, nil, printer, shared, "pc", 631)
		value := attrValues(t, rsp.Job, "job-id")
		if len(value) != 1 {
			t.Fatalf("create job response: %v", rsp)
		}
		id, err := strconv.ParseUint(value[0], 10, 32)
		if err != nil {
			t.Fatal(err)
		}
		return uint32(id)
	}
	request := func(op goipp.Op, id uint32) *goipp.Message {
		req := goipp.NewRequest(ipp11, op, 2)
		req.Operation.Add(goipp.MakeAttr("job-id", goipp.TagInteger, goipp.Integer(id)))
		return req
	}
	id := create()
	rsp := serveIPP(request(goipp.OpCancelJob, id), nil, printer, shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusOk {
		t.Fatal("cancel failed")
	}
	send := request(goipp.OpSendDocument, id)
	send.Operation.Add(goipp.MakeAttr("last-document", goipp.TagBoolean, goipp.Boolean(true)))
	if rsp = serveIPP(send, []byte("%PDF-1.4"), printer, shared, "pc", 631); goipp.Status(rsp.Code) != goipp.StatusErrorNotPossible {
		t.Fatal("canceled job accepted a document")
	}
	id = create()
	send = request(goipp.OpSendDocument, id)
	send.Operation.Add(goipp.MakeAttr("last-document", goipp.TagBoolean, goipp.Boolean(true)))
	rsp = serveIPP(send, []byte("%PDF-1.4"), printer, shared, "pc", 631)
	waitIPPJob(t, shared, id)
	if goipp.Status(rsp.Code) != goipp.StatusOk || shared.Spool.(*fakeSpooler).calls != 2 {
		t.Fatalf("send status=%s calls=%d", goipp.Status(rsp.Code), shared.Spool.(*fakeSpooler).calls)
	}
	job, ok := shared.getIPPJob(printer.ID, id)
	if !ok || job.state != 9 {
		t.Fatalf("job = %+v", job)
	}
	if rsp = serveIPP(send, []byte("%PDF-1.4"), printer, shared, "pc", 631); goipp.Status(rsp.Code) != goipp.StatusErrorNotPossible {
		t.Fatal("accepted a second document")
	}
}

func TestCloseJobWithStagedDocument(t *testing.T) {
	for _, closeOp := range []goipp.Op{goipp.OpCloseJob, goipp.OpSendDocument} {
		t.Run(closeOp.String(), func(t *testing.T) {
			shared := testShared()
			create := goipp.NewRequest(ipp11, goipp.OpCreateJob, 1)
			create.Operation.Add(goipp.MakeAttr("job-name", goipp.TagName, goipp.String("original.pdf")))
			rsp := serveIPP(create, nil, testPrinter(), shared, "pc", 631)
			id, _ := strconv.Atoi(attrValues(t, rsp.Job, "job-id")[0])

			send := goipp.NewRequest(ipp11, goipp.OpSendDocument, 2)
			send.Operation.Add(goipp.MakeAttr("job-id", goipp.TagInteger, goipp.Integer(id)))
			send.Operation.Add(goipp.MakeAttr("last-document", goipp.TagBoolean, goipp.Boolean(false)))
			doc := []byte("%PDF-1.4")
			rsp = serveIPP(send, doc, testPrinter(), shared, "pc", 631)
			if goipp.Status(rsp.Code) != goipp.StatusOk || shared.Spool.(*fakeSpooler).calls != 0 {
				t.Fatalf("staging status %s", goipp.Status(rsp.Code))
			}
			if rsp = serveIPP(send, doc, testPrinter(), shared, "pc", 631); goipp.Status(rsp.Code) != goipp.StatusErrorMultipleJobsNotSupported {
				t.Fatalf("second document status %s", goipp.Status(rsp.Code))
			}
			close := goipp.NewRequest(ipp11, closeOp, 3)
			close.Operation.Add(goipp.MakeAttr("job-id", goipp.TagInteger, goipp.Integer(id)))
			if closeOp == goipp.OpSendDocument {
				close.Operation.Add(goipp.MakeAttr("last-document", goipp.TagBoolean, goipp.Boolean(true)))
			}
			rsp = serveIPP(close, nil, testPrinter(), shared, "pc", 631)
			waitIPPJob(t, shared, uint32(id))
			if goipp.Status(rsp.Code) != goipp.StatusOk || shared.Spool.(*fakeSpooler).calls != 1 || shared.pendingBytes != 0 {
				t.Fatalf("close status %s, calls %d, pending %d", goipp.Status(rsp.Code), shared.Spool.(*fakeSpooler).calls, shared.pendingBytes)
			}
			if got := attrValues(t, rsp.Job, "job-name"); len(got) != 1 || got[0] != "original.pdf" {
				t.Fatalf("job-name = %v", got)
			}
		})
	}
}

func TestEmptyLastDocumentIsLegal(t *testing.T) {
	shared := testShared()
	create := goipp.NewRequest(ipp11, goipp.OpCreateJob, 1)
	rsp := serveIPP(create, nil, testPrinter(), shared, "pc", 631)
	id := responseJobID(t, rsp)
	send := goipp.NewRequest(ipp11, goipp.OpSendDocument, 2)
	send.Operation.Add(goipp.MakeAttr("job-id", goipp.TagInteger, goipp.Integer(id)))
	send.Operation.Add(goipp.MakeAttr("last-document", goipp.TagBoolean, goipp.Boolean(false)))
	rsp = serveIPP(send, nil, testPrinter(), shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusErrorBadRequest {
		t.Fatalf("empty non-final document status = %s", goipp.Status(rsp.Code))
	}
	send.Operation[1] = goipp.MakeAttr("last-document", goipp.TagBoolean, goipp.Boolean(true))
	rsp = serveIPP(send, nil, testPrinter(), shared, "pc", 631)
	job, _ := shared.getIPPJob("hp", id)
	if goipp.Status(rsp.Code) != goipp.StatusOk || job.state != 9 || shared.Spool.(*fakeSpooler).calls != 0 {
		t.Fatalf("empty final status = %s, job state = %d", goipp.Status(rsp.Code), job.state)
	}
}

func TestPendingDocumentExpiresAndReleasesMemory(t *testing.T) {
	shared := testShared()
	job := ippJob{id: 1, printerID: "hp", state: 3, created: time.Now()}
	if !shared.addIPPJob(job) {
		t.Fatal("failed to add pending job")
	}
	if _, _, status := shared.receiveIPPDocument("hp", 1, []byte("%PDF-1.4"), "application/pdf", false); status != goipp.StatusOk {
		t.Fatalf("stage status = %s", status)
	}
	shared.mu.Lock()
	shared.ippJobs[0].deadline = time.Now().Add(-time.Second)
	shared.mu.Unlock()
	shared.expireIPPJob("hp", 1)
	got, _ := shared.getIPPJob("hp", 1)
	if got.state != 8 || len(got.document) != 0 || shared.pendingBytes != 0 {
		t.Fatalf("expired job = %+v, pending = %d", got, shared.pendingBytes)
	}
}
