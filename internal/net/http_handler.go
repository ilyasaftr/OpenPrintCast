package net

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

const (
	// maxIPPMessageBytes bounds the encoded IPP message that precedes a
	// document in the request body.
	maxIPPMessageBytes = 1 << 20
	// maxTraceLen bounds each logged client-supplied string and list.
	maxTraceLen = 80
)

// PrinterHandler serves IPP for one shared printer over HTTP, following
// the goipp serve pattern from the spike: goipp decodes the message off
// the request body and whatever bytes remain are the document.
type PrinterHandler struct {
	Printer core.PrinterInfo
	Shared  *SharedState
	Host    string
	Port    int
}

// ServeHTTP implements http.Handler.
func (h *PrinterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.HasSuffix(r.URL.Path, "/icon.png") {
		serveIcon(w, r)
		return
	}
	if r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "OpenPrintCast: %s\n", h.Printer.DisplayName())
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("Content-Type") != goipp.ContentType {
		http.Error(w, "need application/ipp", http.StatusUnsupportedMediaType)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxDocumentBytes+maxIPPMessageBytes)
	var req goipp.Message
	if err := req.Decode(io.LimitReader(r.Body, maxIPPMessageBytes)); err != nil {
		componentLogger(componentIPP).Warn("ipp decode failed", "remote", r.RemoteAddr, "err", err)
		http.Error(w, "ipp decode: "+err.Error(), http.StatusBadRequest)
		return
	}
	// Operation-level trace: proves which IPP verbs iOS actually sends and
	// which printer each targets (stale iOS entries hit unknown ids).
	op := goipp.Op(req.Code)
	componentLogger(componentIPP).Info("ipp request",
		"op", op.String(), "version", req.Version.String(), "path", r.URL.Path, "remote", r.RemoteAddr)
	var rsp *goipp.Message
	if status := h.validate(&req); status != goipp.StatusOk {
		rsp = h.errorResponse(&req, status)
	} else {
		carriesDocument := op == goipp.OpPrintJob || op == goipp.OpSendDocument
		if carriesDocument {
			if !h.Shared.beginUpload(r.Context()) {
				return
			}
			defer h.Shared.endUpload()
		}
		var ok bool
		if rsp, ok = h.serveBody(w, r, &req, carriesDocument); !ok {
			return
		}
	}
	componentLogger(componentIPP).Info("ipp response",
		"op", op.String(), "path", r.URL.Path, "status", goipp.Status(rsp.Code).String())
	if op == goipp.OpGetPrinterAttributes && strings.Contains(r.UserAgent(), "PrintKit") {
		logPrintKitAttributes(r, &req, rsp)
	}
	out, err := rsp.EncodeBytes()
	if err != nil {
		http.Error(w, "ipp encode: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", goipp.ContentType)
	w.Write(out)
}

func serveIcon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("Content-Length", strconv.Itoa(len(printerIcon)))
	if r.Method == http.MethodGet {
		_, _ = w.Write(printerIcon)
	}
}

func (h *PrinterHandler) validate(req *goipp.Message) goipp.Status {
	if status := validateIPPRequest(req); status != goipp.StatusOk {
		return status
	}
	return validateIPPTarget(req, h.Printer)
}

func (h *PrinterHandler) errorResponse(req *goipp.Message, status goipp.Status) *goipp.Message {
	echo, _ := versionResponse(req.Version, h.Shared, h.Printer)
	return statusResponse(echo, status, req.RequestID)
}

// serveBody reads the document that follows the IPP message and answers the
// request. ok is false when an HTTP error has already been written.
func (h *PrinterHandler) serveBody(w http.ResponseWriter, r *http.Request, req *goipp.Message, carriesDocument bool) (rsp *goipp.Message, ok bool) {
	limit := int64(1)
	if carriesDocument {
		limit = maxDocumentBytes + 1
	}
	doc, err := io.ReadAll(io.LimitReader(r.Body, limit))
	if err != nil {
		http.Error(w, "unable to read document", http.StatusBadRequest)
		return nil, false
	}
	if len(doc) > maxDocumentBytes {
		http.Error(w, "document too large", http.StatusRequestEntityTooLarge)
		return nil, false
	}
	if len(doc) > 0 && !carriesDocument && goipp.Op(req.Code) != goipp.OpCloseJob {
		return h.errorResponse(req, goipp.StatusErrorBadRequest), true
	}
	return serveIPP(req, doc, h.Printer, h.Shared, h.Host, h.Port), true
}

func logPrintKitAttributes(r *http.Request, req, rsp *goipp.Message) {
	requested, missing := printerAttributeTrace(req, rsp)
	format := opAttrString(req.Operation, "document-format")
	if len(format) > maxTraceLen {
		format = format[:maxTraceLen]
	}
	componentLogger(componentIPP).Info("ipp printkit attributes", "path", r.URL.Path,
		"format", format, "requested", requested, "missing", missing, "returned", len(rsp.Printer))
}

// printerAttributeTrace logs only requested attribute names, not document or
// user data. Limits keep untrusted client requests from flooding the log.
func printerAttributeTrace(req, rsp *goipp.Message) (requested, missing []string) {
	present := make(map[string]bool, len(rsp.Printer))
	for _, attr := range rsp.Printer {
		present[attr.Name] = true
	}
	for _, attr := range req.Operation {
		if attr.Name != "requested-attributes" {
			continue
		}
		for _, value := range attr.Values {
			if len(requested) == maxTraceLen {
				return
			}
			name := value.V.String()
			if len(name) > maxTraceLen {
				name = name[:maxTraceLen]
			}
			requested = append(requested, name)
			if !isAttributeGroup(name) && !present[name] {
				missing = append(missing, name)
			}
		}
		break
	}
	return
}

// isAttributeGroup reports whether name selects a group of attributes
// rather than a single one.
func isAttributeGroup(name string) bool {
	return attributeGroupNames[name] || name == "printer-defaults"
}
