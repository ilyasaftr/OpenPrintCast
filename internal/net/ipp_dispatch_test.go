package net

import (
	"testing"

	"github.com/OpenPrinting/goipp"
)

func TestEveryAdvertisedOperationHasAHandler(t *testing.T) {
	seen := map[goipp.Op]bool{}
	for _, op := range advertisedOperations {
		if opHandlers[op] == nil {
			t.Errorf("operation %v is advertised in operations-supported but has no handler", op)
		}
		if seen[op] {
			t.Errorf("operation %v is advertised twice", op)
		}
		seen[op] = true
	}
}

func TestEveryHandledOperationIsAdvertisedExceptIdentify(t *testing.T) {
	advertised := map[goipp.Op]bool{}
	for _, op := range advertisedOperations {
		advertised[op] = true
	}
	for op := range opHandlers {
		if op == goipp.OpIdentifyPrinter {
			continue
		}
		if !advertised[op] {
			t.Errorf("operation %v has a handler but clients are never told it is supported", op)
		}
	}
}

// Print-URI and Send-URI are left out on purpose: accepting a document-uri
// from a client would make the PC fetch arbitrary addresses (SSRF). The CUPS
// suites skip their steps for these operations because operations-supported
// omits them, so only this test pins what a client that sends them anyway
// gets.
func TestUnsupportedOperationsAreNotSupported(t *testing.T) {
	for _, op := range []goipp.Op{
		goipp.OpPrintURI, goipp.OpSendURI, goipp.OpHoldJob, goipp.OpReleaseJob,
		goipp.OpRestartJob, goipp.OpPausePrinter, goipp.OpResumePrinter, goipp.OpPurgeJobs,
	} {
		t.Run(op.String(), func(t *testing.T) {
			shared := testShared()
			spooler := shared.Spool.(*fakeSpooler)
			req := goipp.NewRequest(ipp11, op, 7)
			req.Operation.Add(goipp.MakeAttr("printer-uri", goipp.TagURI, goipp.String("ipp://pc/ipp/print/hp")))
			req.Operation.Add(goipp.MakeAttr("job-id", goipp.TagInteger, goipp.Integer(1)))
			req.Operation.Add(goipp.MakeAttr("document-uri", goipp.TagURI, goipp.String("http://127.0.0.1:1/internal.pdf")))
			rsp := serveIPP(req, nil, testPrinter(), shared, "pc", 631)
			if got := goipp.Status(rsp.Code); got != goipp.StatusErrorOperationNotSupported {
				t.Fatalf("status = %s, want %s", got, goipp.StatusErrorOperationNotSupported)
			}
			if rsp.RequestID != 7 || spooler.calls != 0 || len(shared.ippJobsFor("hp")) != 0 {
				t.Fatalf("request id %d, spool calls %d, jobs %d: nothing may be fetched, spooled or queued",
					rsp.RequestID, spooler.calls, len(shared.ippJobsFor("hp")))
			}
			for _, advertised := range advertisedOperations {
				if advertised == op {
					t.Fatalf("%s is refused but advertised in operations-supported", op)
				}
			}
		})
	}
}
