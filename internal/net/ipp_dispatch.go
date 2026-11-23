package net

import (
	"slices"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// ippRequest is everything an operation handler needs to answer one request.
type ippRequest struct {
	req     *goipp.Message
	doc     []byte
	printer core.PrinterInfo
	shared  *SharedState
	host    string
	port    int
	echo    goipp.Version
}

// respond builds the bare response for status, echoing the negotiated version.
func (c *ippRequest) respond(status goipp.Status) *goipp.Message {
	return statusResponse(c.echo, status, c.req.RequestID)
}

// opHandler answers one IPP operation.
type opHandler func(c *ippRequest) *goipp.Message

// opHandlers maps each implemented operation to its handler; adding an
// operation means adding an entry here (and to advertisedOperations).
var opHandlers = map[goipp.Op]opHandler{
	goipp.OpGetPrinterAttributes: handleGetPrinterAttributes,
	goipp.OpIdentifyPrinter:      handleIdentifyPrinter,
	goipp.OpPrintJob:             handlePrintJob,
	goipp.OpCreateJob:            handleCreateJob,
	goipp.OpSendDocument:         handleSendDocument,
	goipp.OpCloseJob:             handleCloseJob,
	goipp.OpCancelJob:            handleCancelJob,
	goipp.OpCancelMyJobs:         handleCancelMyJobs,
	goipp.OpValidateJob:          handleValidateJob,
	goipp.OpGetJobAttributes:     handleGetJobAttributes,
	goipp.OpGetJobs:              handleGetJobs,
}

// advertisedOperations is the operations-supported order; Identify-Printer
// follows only when the spooler can identify.
var advertisedOperations = []goipp.Op{
	goipp.OpPrintJob,
	goipp.OpValidateJob,
	goipp.OpCreateJob,
	goipp.OpSendDocument,
	goipp.OpCloseJob,
	goipp.OpCancelJob,
	goipp.OpCancelMyJobs,
	goipp.OpGetPrinterAttributes,
	goipp.OpGetJobAttributes,
	goipp.OpGetJobs,
}

// documentOps carry a document, so password and compression apply.
var documentOps = map[goipp.Op]bool{
	goipp.OpPrintJob:     true,
	goipp.OpSendDocument: true,
	goipp.OpValidateJob:  true,
}

// jobTemplateOps carry job-template attributes that may be unsupported.
var jobTemplateOps = map[goipp.Op]bool{
	goipp.OpPrintJob:    true,
	goipp.OpValidateJob: true,
	goipp.OpCreateJob:   true,
}

// serveIPP dispatches one decoded IPP request to its per-printer logic.
// It is pure (no sockets) so unit tests exercise the full protocol path.
func serveIPP(req *goipp.Message, doc []byte, printer core.PrinterInfo, shared *SharedState, host string, port int) *goipp.Message {
	echo, versionOK := versionResponse(req.Version, shared, printer)
	c := &ippRequest{req: req, doc: doc, printer: printer, shared: shared, host: host, port: port, echo: echo}
	if req.RequestID == 0 || req.RequestID > maxRequestID {
		return c.respond(goipp.StatusErrorBadRequest)
	}
	if !versionOK {
		return c.respond(goipp.StatusErrorVersionNotSupported)
	}
	op := goipp.Op(req.Code)
	if documentOps[op] {
		if rsp := c.checkDocumentAttributes(); rsp != nil {
			return rsp
		}
	}
	var ignored goipp.Attributes
	if jobTemplateOps[op] {
		var rsp *goipp.Message
		if ignored, rsp = c.applyJobTemplate(); rsp != nil {
			return rsp
		}
	}
	rsp := c.dispatch(op)
	if len(ignored) > 0 && goipp.Status(rsp.Code) == goipp.StatusOk {
		rsp.Code = goipp.Code(goipp.StatusOkIgnoredOrSubstituted)
		rsp.Unsupported = ignored
	}
	return rsp
}

func (c *ippRequest) dispatch(op goipp.Op) *goipp.Message {
	handler, ok := opHandlers[op]
	if !ok {
		return c.respond(goipp.StatusErrorOperationNotSupported)
	}
	return handler(c)
}

// checkDocumentAttributes rejects document passwords and unsupported
// compression, and expands a compressed document in place; it returns nil
// when the request may proceed.
func (c *ippRequest) checkDocumentAttributes() *goipp.Message {
	for _, attr := range c.req.Operation {
		if attr.Name == "document-password" {
			rsp := c.respond(goipp.StatusErrorAttributesOrValues)
			rsp.Unsupported.Add(attr)
			return rsp
		}
	}
	compression := opAttrString(c.req.Operation, "compression")
	if compression != "" && !compressionSupported(compression) {
		return c.respond(goipp.StatusErrorCompressionNotSupported)
	}
	doc, status := inflateDocument(c.doc, compression, maxDocumentBytes)
	if status != goipp.StatusOk {
		componentLogger(componentIPP).Warn("document decompression failed", "op", goipp.Op(c.req.Code).String(),
			"printer", c.printer.ID, "compression", compression, "bytes", len(c.doc), "status", status.String())
		return c.respond(status)
	}
	c.doc = doc
	return nil
}

// applyJobTemplate honors ipp-attribute-fidelity. Unsupported job attributes
// fail a fidelity request; otherwise they are dropped from c.req and
// returned so the caller can report them next to a successful status.
func (c *ippRequest) applyJobTemplate() (ignored goipp.Attributes, rsp *goipp.Message) {
	fidelity, valid := attributeFidelity(c.req)
	if !valid {
		return nil, c.respond(goipp.StatusErrorBadRequest)
	}
	ignored = unsupportedJobOptions(c.req, c.shared, c.printer)
	if len(ignored) == 0 {
		return nil, nil
	}
	if fidelity {
		rsp = c.respond(goipp.StatusErrorAttributesOrValues)
		rsp.Unsupported = ignored
		return ignored, rsp
	}
	copyReq := *c.req
	copyReq.Job = withoutNamed(c.req.Job, ignored)
	c.req = &copyReq
	return ignored, nil
}

// attributeFidelity reads ipp-attribute-fidelity; valid is false for a
// repeated or malformed attribute.
func attributeFidelity(req *goipp.Message) (fidelity, valid bool) {
	present := false
	for _, attr := range req.Operation {
		if attr.Name != "ipp-attribute-fidelity" {
			continue
		}
		if present || !hasSingleValue(attr, goipp.TagBoolean) {
			return false, false
		}
		value, ok := attr.Values[0].V.(goipp.Boolean)
		if !ok {
			return false, false
		}
		fidelity, present = bool(value), true
	}
	return fidelity, true
}

// withoutNamed copies attrs minus every attribute named like one in excluded.
func withoutNamed(attrs, excluded goipp.Attributes) goipp.Attributes {
	var kept goipp.Attributes
	for _, attr := range attrs {
		if !slices.ContainsFunc(excluded, func(e goipp.Attribute) bool { return e.Name == attr.Name }) {
			kept.Add(attr)
		}
	}
	return kept
}
