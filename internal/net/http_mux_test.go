package net

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenPrinting/goipp"
)

func TestPeekOpReadsOnlyIPPHeader(t *testing.T) {
	req := goipp.NewRequest(ipp11, goipp.OpPrintJob, 1)
	data, err := req.EncodeBytes()
	if err != nil {
		t.Fatal(err)
	}
	body := bytes.NewReader(append(data, bytes.Repeat([]byte("x"), 1024)...))
	httpReq := httptest.NewRequest(http.MethodPost, "/ipp/print/missing", body)
	before := body.Len()
	if got := peekOp(httpReq); got != "Print-Job 1.1" {
		t.Fatalf("peeked operation = %q", got)
	}
	if consumed := before - body.Len(); consumed != 8 {
		t.Fatalf("peek consumed %d bytes, want only the header", consumed)
	}
}

func TestCUPSIPPWorkflow(t *testing.T) {
	spooler := &fakeSpooler{}
	uri, shared, tool := startConformanceServer(t, spooler)
	a4 := tool.document(t, requestDocument)
	for _, tc := range []struct{ testFile, document string }{
		{"get-printer-attributes.test", a4},
		{"print-job-and-wait.test", a4},
		{"create-job.test", a4},
		{"create-job.test", absPath(t, filepath.Join("..", "winspool", "testdata", "cups-color.urf"))},
		{"create-job.test", absPath(t, filepath.Join("..", "winspool", "testdata", "cups-color.pwg"))},
	} {
		output, err := tool.command("-tv", "-f", tc.document, uri, tool.suite(tc.testFile)).CombinedOutput()
		if err != nil {
			t.Fatalf("ipptool %s %s: %v\n%s", tc.testFile, tc.document, err, output)
		}
	}
	if output, err := tool.command("-tv", "-f", a4, uri, absPath(t, "ippe-jobs.test")).CombinedOutput(); err != nil {
		t.Fatalf("ipptool job lifecycle: %v\n%s", err, output)
	}
	for _, job := range shared.ippJobsFor("hp") {
		waitIPPJob(t, shared, job.id)
	}
	spooler.mu.Lock()
	got := spooler.calls
	spooler.mu.Unlock()
	if got != 5 {
		t.Fatalf("spool calls = %d, want 5", got)
	}
	// The ratchet judges the full ipp-2.0 suite; this run only needs it to
	// be reachable and to keep exiting cleanly.
	if _, err := os.Stat(tool.suite("ipp-2.0.test")); err == nil {
		if output, err := tool.command("-V", "2.0", "-t", "-f", a4, uri, tool.suite("ipp-2.0.test")).CombinedOutput(); err != nil {
			t.Fatalf("CUPS IPP 2.0 suite against simulated printer: %v\n%s", err, output)
		}
	}
	if diagnostic := os.Getenv("IPP_DIAGNOSTIC"); diagnostic != "" {
		output, err := tool.command("-t", "-f", a4, uri, diagnostic).CombinedOutput()
		if err != nil {
			t.Fatalf("IPP diagnostic: %v\n%s", err, output)
		}
	}
}

// absPath returns the absolute form of a path, because ipptool runs in the
// directory of the sample documents.
func absPath(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

func TestMuxRoutesAndRejects(t *testing.T) {
	shared := testShared()
	mux := testMux(shared, "pc", 631)
	// Unknown printer -> 404 without touching IPP.
	r := httptest.NewRequest(http.MethodPost, "/ipp/print/nope", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown printer status %d, want 404", w.Code)
	}
	// Unsupported method on a known printer -> 405 from the handler.
	r = httptest.NewRequest(http.MethodPut, "/ipp/print/hp", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET status %d, want 405", w.Code)
	}
}

func TestUnknownPrinterStillLogsPrintJobAfterProbe(t *testing.T) {
	logs := captureLogs(t)

	mux := testMux(testShared(), "pc", 631)
	for _, op := range []goipp.Op{goipp.OpGetPrinterAttributes, goipp.OpGetPrinterAttributes, goipp.OpPrintJob} {
		msg := goipp.NewRequest(goipp.MakeVersion(2, 0), op, 1)
		body, err := msg.EncodeBytes()
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/ipp/print/printer-3", bytes.NewReader(body))
		req.Header.Set("Content-Type", goipp.ContentType)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status %d, want 404", op, rec.Code)
		}
	}
	if strings.Count(logs.String(), "Get-Printer-Attributes") != 1 ||
		!strings.Contains(logs.String(), `msg="ipp request rejected: unknown or unshared printer" component=ipp op="Print-Job 2.0" printer=printer-3`) {
		t.Fatalf("unexpected unknown-printer log: %s", logs.String())
	}
}

// testMux is the router a test serves, for a fixed host label.
func testMux(shared *SharedState, host string, port int) *http.ServeMux {
	return newMux(shared, func() string { return host }, port)
}

func TestRootPageNamesTheSharedPrinters(t *testing.T) {
	mux := testMux(testShared(), "pc", 631)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	body := rec.Body.String()
	if rec.Code != http.StatusOK || !strings.HasPrefix(body, "OpenPrintCast on pc\n") || !strings.Contains(body, testPrinter().DisplayName()) {
		t.Fatalf("root page = %d %q", rec.Code, body)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/elsewhere", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("other path status %d, want 404", rec.Code)
	}
}
