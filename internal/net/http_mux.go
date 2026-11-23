package net

import (
	"encoding/binary"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/OpenPrinting/goipp"
)

const (
	printerPathPrefix = "/ipp/print/"
	// rejectedBodyDeadline caps how long a rejected request body is drained.
	rejectedBodyDeadline = 2 * time.Second
	// staleLogInterval spaces repeated log lines for one unknown printer.
	staleLogInterval = time.Minute
	probeOpPrefix    = "Get-Printer-Attributes "
)

// peekOp best-effort decodes the IPP operation of a request whose body
// will not be served (unknown printer). The body is consumed, so callers
// must not need it afterwards (404 paths don't).
func peekOp(r *http.Request) string {
	var header [8]byte
	if r.Body == nil {
		return "undecodable"
	}
	if _, err := io.ReadFull(r.Body, header[:]); err != nil {
		return "undecodable"
	}
	op := goipp.Op(binary.BigEndian.Uint16(header[2:4]))
	return op.String() + " " + goipp.MakeVersion(header[0], header[1]).String()
}

func peekRejectedOp(w http.ResponseWriter, r *http.Request) string {
	if r.Method != http.MethodPost || r.Header.Get("Content-Type") != goipp.ContentType {
		return "undecodable"
	}
	controller := http.NewResponseController(w)
	// Keep the deadline in place while net/http drains the rejected body.
	_ = controller.SetReadDeadline(time.Now().Add(rejectedBodyDeadline))
	return peekOp(r)
}

// newMux routes /ipp/print/<id> to the per-printer handlers, agreeing with
// core.ResourcePath, and answers the root (the adminurl) with a short text
// page; every other path is a 404.
func newMux(shared *SharedState, host func() string, port int) *http.ServeMux {
	router := &printerRouter{shared: shared, host: host, port: port}
	mux := http.NewServeMux()
	mux.HandleFunc(printerPathPrefix, router.serve)
	mux.HandleFunc("GET /{$}", router.serveRoot)
	return mux
}

// printerRouter resolves the printer a request path addresses.
type printerRouter struct {
	shared      *SharedState
	host        func() string
	port        int
	staleLogged sync.Map
}

// serveRoot answers the adminurl the Bonjour record points at: a plain text
// page naming the shared printers, which also shows from any browser that
// the port is reachable.
func (p *printerRouter) serveRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	var page strings.Builder
	page.WriteString("OpenPrintCast on " + p.host() + "\n")
	for _, printer := range p.shared.EnabledPrinters() {
		page.WriteString(printer.DisplayName() + "\n")
	}
	_, _ = io.WriteString(w, page.String())
}

func (p *printerRouter) serve(w http.ResponseWriter, r *http.Request) {
	id, ok := printerIDFromPath(r.URL.Path)
	if !ok {
		componentLogger(componentIPP).Info("ipp request rejected: unknown path (stale iOS entry?)",
			"op", peekRejectedOp(w, r), "path", r.URL.Path, "remote", r.RemoteAddr, "status", http.StatusNotFound)
		http.NotFound(w, r)
		return
	}
	printer, ok := p.shared.PrinterByID(id)
	if !ok || !printer.Enabled {
		p.rejectUnknownPrinter(w, r, id)
		return
	}
	h := &PrinterHandler{Printer: printer, Shared: p.shared, Host: p.host(), Port: p.port}
	h.ServeHTTP(w, r)
}

// printerIDFromPath extracts <id> from /ipp/print/<id>[/icon.png|/job/<n>];
// ok is false for any other shape.
func printerIDFromPath(urlPath string) (id string, ok bool) {
	id, suffix, hasSuffix := strings.Cut(strings.TrimPrefix(urlPath, printerPathPrefix), "/")
	validSuffix := !hasSuffix || suffix == "icon.png"
	if strings.HasPrefix(suffix, "job/") {
		validSuffix = validJobPathID(strings.TrimPrefix(suffix, "job/"))
	}
	return id, id != "" && validSuffix
}

// validJobPathID accepts a canonical positive decimal job id.
func validJobPathID(text string) bool {
	jobID, err := strconv.ParseUint(text, 10, 31)
	return err == nil && jobID > 0 && strconv.FormatUint(jobID, 10) == text
}

func (p *printerRouter) rejectUnknownPrinter(w http.ResponseWriter, r *http.Request, id string) {
	op := peekRejectedOp(w, r)
	probe := strings.HasPrefix(op, probeOpPrefix)
	// Suppress repeated discovery probes, never job submissions.
	if last, seen := p.staleLogged.Load(id); !probe || !seen || time.Since(last.(time.Time)) > staleLogInterval {
		if probe {
			p.staleLogged.Store(id, time.Now())
		}
		componentLogger(componentIPP).Info("ipp request rejected: unknown or unshared printer",
			"op", op, "printer", id, "remote", r.RemoteAddr, "status", http.StatusNotFound)
	}
	http.NotFound(w, r)
}
