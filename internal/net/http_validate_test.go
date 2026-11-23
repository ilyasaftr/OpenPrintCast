package net

import (
	"testing"

	"github.com/OpenPrinting/goipp"
)

func TestRequestTargetsAreValidated(t *testing.T) {
	request := func(op goipp.Op) *goipp.Message {
		req := goipp.NewRequest(ipp11, op, 1)
		req.Operation.Add(goipp.MakeAttr("attributes-charset", goipp.TagCharset, goipp.String("utf-8")))
		req.Operation.Add(goipp.MakeAttr("attributes-natural-language", goipp.TagLanguage, goipp.String("en")))
		return req
	}
	printerURI := goipp.MakeAttr("printer-uri", goipp.TagURI, goipp.String("ipp://pc.local:631/ipp/print/hp"))
	jobURI := goipp.MakeAttr("job-uri", goipp.TagURI, goipp.String("ipp://pc.local:631/ipp/print/hp/job/1"))
	jobID := goipp.MakeAttr("job-id", goipp.TagInteger, goipp.Integer(1))
	for _, tc := range []struct {
		name   string
		op     goipp.Op
		attrs  []goipp.Attribute
		status goipp.Status
	}{
		{"printer operation without printer", goipp.OpGetPrinterAttributes, []goipp.Attribute{jobURI}, goipp.StatusErrorBadRequest},
		{"printer operation", goipp.OpGetPrinterAttributes, []goipp.Attribute{printerURI}, goipp.StatusOk},
		{"job URI target", goipp.OpGetJobAttributes, []goipp.Attribute{jobURI}, goipp.StatusOk},
		{"job ID target", goipp.OpSendDocument, []goipp.Attribute{printerURI, jobID}, goipp.StatusOk},
		{"missing job ID", goipp.OpSendDocument, []goipp.Attribute{printerURI}, goipp.StatusErrorBadRequest},
		{"close requires printer and ID", goipp.OpCloseJob, []goipp.Attribute{jobURI}, goipp.StatusErrorBadRequest},
		{"close valid", goipp.OpCloseJob, []goipp.Attribute{printerURI, jobID}, goipp.StatusOk},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := request(tc.op)
			req.Operation = append(req.Operation, tc.attrs...)
			if got := validateIPPRequest(req); got != tc.status {
				t.Fatalf("status = %s, want %s", got, tc.status)
			}
		})
	}
	req := request(goipp.OpGetPrinterAttributes)
	req.Operation.Add(goipp.MakeAttr("printer-uri", goipp.TagURI, goipp.String("ipp://pc.local:631/ipp/print/other")))
	if got := validateIPPTarget(req, testPrinter()); got != goipp.StatusErrorNotFound {
		t.Fatalf("mismatched printer target = %s", got)
	}
	req = request(goipp.OpGetPrinterAttributes)
	req.Operation.Add(goipp.MakeAttr("printer-uri", goipp.TagURI, goipp.String("ipps://pc.local:631/ipp/print/hp")))
	if got := validateIPPTarget(req, testPrinter()); got != goipp.StatusErrorBadRequest {
		t.Fatalf("ipps target = %s; the bridge only serves plain ipp", got)
	}
}
