package net

import (
	"bytes"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OpenPrinting/goipp"
)

func TestPrinterAttributeTrace(t *testing.T) {
	req := goipp.NewRequest(goipp.MakeVersion(2, 0), goipp.OpGetPrinterAttributes, 1)
	req.Operation.Add(goipp.MakeAttr("requesting-user-name", goipp.TagName, goipp.String("private-user")))
	req.Operation.Add(goipp.MakeAttr("requested-attributes", goipp.TagKeyword,
		goipp.String("printer-defaults"), goipp.String("printer-state"), goipp.String("missing-attribute")))
	rsp := serveIPP(req, nil, testPrinter(), testShared(), "pc", 631)
	requested, missing := printerAttributeTrace(req, rsp)
	if len(requested) != 3 || len(missing) != 1 || missing[0] != "missing-attribute" {
		t.Fatalf("requested = %v, missing = %v", requested, missing)
	}
}

func TestPrinterIconIsReachable(t *testing.T) {
	shared := testShared()
	mux := testMux(shared, "pc", 631)
	req := httptest.NewRequest(http.MethodGet, "/ipp/print/hp/icon.png", nil)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	if res.Code != http.StatusOK || res.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("icon status = %d, headers = %v", res.Code, res.Header())
	}
	img, err := png.Decode(bytes.NewReader(res.Body.Bytes()))
	if err != nil || img.Bounds().Dx() != 128 || img.Bounds().Dy() != 128 {
		t.Fatalf("icon decode = %v, image = %v", err, img)
	}
	_, _, _, alpha := img.At(0, 0).RGBA()
	if alpha != 0 {
		t.Fatal("icon background must be transparent")
	}
	attrs := printerAttrsResponse(goipp.NewRequest(ipp11, goipp.OpGetPrinterAttributes, 1), testPrinter(), shared, "pc", 631)
	if got := attrValues(t, attrs.Printer, "printer-icons"); len(got) != 1 || got[0] != "http://pc.local:631/ipp/print/hp/icon.png" {
		t.Fatalf("printer-icons = %v", got)
	}
}

// TestPrinterHandlerRoundTrip drives the HTTP layer in-memory
// (httptest request/recorder, no sockets): POST version-2.0
// Get-Printer-Attributes and expect an echoed 2.0 response with markers.
func TestPrinterHandlerRoundTrip(t *testing.T) {
	shared := testShared()
	h := &PrinterHandler{Printer: testPrinter(), Shared: shared, Host: "pc", Port: 631}
	req := goipp.NewRequest(goipp.MakeVersion(2, 0), goipp.OpGetPrinterAttributes, 9)
	req.Operation.Add(goipp.MakeAttr("attributes-charset",
		goipp.TagCharset, goipp.String("utf-8")))
	req.Operation.Add(goipp.MakeAttr("attributes-natural-language", goipp.TagLanguage, goipp.String("en")))
	req.Operation.Add(goipp.MakeAttr("printer-uri", goipp.TagURI, goipp.String("ipp://pc.local:631/ipp/print/hp")))
	body, err := req.EncodeBytes()
	if err != nil {
		t.Fatal(err)
	}
	httpReq := httptest.NewRequest(http.MethodPost, "/ipp/print/hp", bytes.NewReader(body))
	httpReq.Header.Set("Content-Type", goipp.ContentType)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httpReq)
	if rec.Code != http.StatusOK {
		t.Fatalf("HTTP status %d, want 200", rec.Code)
	}
	var rsp goipp.Message
	if err := rsp.DecodeBytes(rec.Body.Bytes()); err != nil {
		t.Fatal(err)
	}
	if rsp.Version != goipp.MakeVersion(2, 0) {
		t.Errorf("response version %s, want echo 2.0", rsp.Version)
	}
	if got := attrValues(t, rsp.Printer, "printer-uri-supported"); len(got) != 1 || got[0] != "ipp://pc.local:631/ipp/print/hp" {
		t.Errorf("printer URI = %v, want a reachable .local URI", got)
	}
}
