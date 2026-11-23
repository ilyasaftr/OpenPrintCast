package net

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"runtime"
	"slices"
	"testing"

	"github.com/OpenPrinting/goipp"
)

func gzipped(t *testing.T, data []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	w := gzip.NewWriter(&out)
	if _, err := w.Write(data); err != nil || w.Close() != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func deflated(t *testing.T, data []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	w := zlib.NewWriter(&out)
	if _, err := w.Write(data); err != nil || w.Close() != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// rawDeflated is headerless DEFLATE (RFC 1951), which is what the 'deflate'
// keyword means in RFC 8011 and what CUPS sends.
func rawDeflated(t *testing.T, data []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	w, _ := flate.NewWriter(&out, flate.DefaultCompression)
	if _, err := w.Write(data); err != nil || w.Close() != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestInflateDocument(t *testing.T) {
	doc := bytes.Repeat([]byte("%PDF-1.4 page "), 100)
	const limit = 4096
	truncated := gzipped(t, doc)
	truncated = truncated[:len(truncated)-6]
	corrupt := gzipped(t, doc)
	corrupt[len(corrupt)-5] ^= 0xFF // CRC-32 of the data
	cases := []struct {
		name, keyword string
		in            []byte
		want          goipp.Status
		out           []byte
	}{
		{"gzip", compressionGzip, gzipped(t, doc), statusOK, doc},
		{"deflate is raw RFC 1951", compressionDeflate, rawDeflated(t, doc), statusOK, doc},
		{"zlib-wrapped deflate is tolerated", compressionDeflate, deflated(t, doc), statusOK, doc},
		{"none is untouched", compressionNone, doc, statusOK, doc},
		{"absent is untouched", "", doc, statusOK, doc},
		{"garbage gzip", compressionGzip, []byte("not gzip at all"), goipp.StatusErrorCompressionError, nil},
		{"garbage deflate", compressionDeflate, []byte("not zlib at all"), goipp.StatusErrorCompressionError, nil},
		{"truncated deflate", compressionDeflate, rawDeflated(t, doc)[:20], goipp.StatusErrorCompressionError, nil},
		{"truncated zlib", compressionDeflate, deflated(t, doc)[:20], goipp.StatusErrorCompressionError, nil},
		{"gzip sent as deflate", compressionDeflate, gzipped(t, doc), goipp.StatusErrorCompressionError, nil},
		{"truncated gzip", compressionGzip, truncated, goipp.StatusErrorCompressionError, nil},
		{"bad gzip checksum", compressionGzip, corrupt, goipp.StatusErrorCompressionError, nil},
		{"no data stays no data", compressionGzip, []byte{}, statusOK, []byte{}},
		{"over the cap", compressionGzip, gzipped(t, bytes.Repeat([]byte{'a'}, limit+1)), goipp.StatusErrorRequestEntity, nil},
		{"exactly the cap", compressionGzip, gzipped(t, bytes.Repeat([]byte{'a'}, limit)), statusOK, bytes.Repeat([]byte{'a'}, limit)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, status := inflateDocument(tc.in, tc.keyword, limit)
			if status != tc.want || !bytes.Equal(out, tc.out) {
				t.Fatalf("inflateDocument = %d bytes, %s; want %d bytes, %s", len(out), status, len(tc.out), tc.want)
			}
		})
	}
}

// A decompression bomb is cut off at the cap: the 256 MiB it claims is never
// allocated.
func TestInflateDocumentNeverAllocatesTheClaimedSize(t *testing.T) {
	const claimed, limit = 256 << 20, 1 << 20
	bomb := gzipped(t, make([]byte, claimed))
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, status := inflateDocument(bomb, compressionGzip, limit)
	runtime.ReadMemStats(&after)
	if status != goipp.StatusErrorRequestEntity {
		t.Fatalf("status = %s", status)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 16*limit {
		t.Fatalf("inflating a bomb allocated %d bytes for a %d byte cap", allocated, limit)
	}
}

func compressedRequest(op goipp.Op, keyword string) *goipp.Message {
	req := formatRequest(op, 1, "application/pdf")
	if keyword != "" {
		req.Operation.Add(goipp.MakeAttr("compression", goipp.TagKeyword, goipp.String(keyword)))
	}
	return req
}

func TestCompressedDocumentsAcrossOperations(t *testing.T) {
	plain := append(pdfData, "body"...)
	cases := []struct {
		name, keyword string
		body          []byte
		want          goipp.Status
	}{
		{"gzip", compressionGzip, gzipped(t, plain), statusOK},
		{"deflate", compressionDeflate, rawDeflated(t, plain), statusOK},
		{"zlib-wrapped deflate", compressionDeflate, deflated(t, plain), statusOK},
		{"none", compressionNone, plain, statusOK},
		{"absent", "", plain, statusOK},
		{"bad gzip stream", compressionGzip, []byte("%PDF-1.4 but not gzip"), goipp.StatusErrorCompressionError},
		{"gzip declared deflate sent", compressionDeflate, gzipped(t, plain), goipp.StatusErrorCompressionError},
		{"compress is unsupported", "compress", plain, goipp.StatusErrorCompressionNotSupported},
		{"unknown keyword", "brotli", plain, goipp.StatusErrorCompressionNotSupported},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			shared := testShared()
			rsp := serveIPP(compressedRequest(goipp.OpPrintJob, tc.keyword), tc.body, testPrinter(), shared, "pc", 631)
			if tc.want == statusOK {
				waitIPPJob(t, shared, responseJobID(t, rsp))
			}
			if got := goipp.Status(rsp.Code); got != tc.want {
				t.Fatalf("Print-Job = %s, want %s", got, tc.want)
			}
			spooled := shared.Spool.(*fakeSpooler).calls == 1
			if spooled != (tc.want == statusOK) {
				t.Fatalf("spooled = %v for status %s", spooled, tc.want)
			}

			shared = testShared()
			rsp = serveIPP(goipp.NewRequest(ipp11, goipp.OpCreateJob, 1), nil, testPrinter(), shared, "pc", 631)
			id := responseJobID(t, rsp)
			send := compressedRequest(goipp.OpSendDocument, tc.keyword)
			send.Operation.Add(goipp.MakeAttr("job-id", goipp.TagInteger, goipp.Integer(id)))
			send.Operation.Add(goipp.MakeAttr("last-document", goipp.TagBoolean, goipp.Boolean(true)))
			if rsp = serveIPP(send, tc.body, testPrinter(), shared, "pc", 631); goipp.Status(rsp.Code) != tc.want {
				t.Fatalf("Send-Document = %s, want %s", goipp.Status(rsp.Code), tc.want)
			}
		})
	}
}

func TestValidateJobChecksTheCompressionKeyword(t *testing.T) {
	for keyword, want := range map[string]goipp.Status{
		compressionGzip: statusOK, compressionDeflate: statusOK, compressionNone: statusOK,
		"compress": goipp.StatusErrorCompressionNotSupported,
	} {
		rsp := serveIPP(compressedRequest(goipp.OpValidateJob, keyword), nil, testPrinter(), testShared(), "pc", 631)
		if got := goipp.Status(rsp.Code); got != want {
			t.Errorf("Validate-Job with compression %q = %s, want %s", keyword, got, want)
		}
	}
}

func TestCompressionSupportedIsAdvertised(t *testing.T) {
	rsp := serveIPP(goipp.NewRequest(ipp11, goipp.OpGetPrinterAttributes, 1), nil, testPrinter(), testShared(), "pc", 631)
	got := attrValues(t, rsp.Printer, "compression-supported")
	if want := []string{"none", "deflate", "gzip"}; !slices.Equal(got, want) {
		t.Fatalf("compression-supported = %v, want %v", got, want)
	}
	for _, keyword := range got {
		if !compressionSupported(keyword) {
			t.Errorf("%s is advertised but not supported", keyword)
		}
	}
}

func TestDecompressionFailureIsLogged(t *testing.T) {
	logs := captureLogs(t)
	serveIPP(compressedRequest(goipp.OpPrintJob, compressionGzip), []byte("junk"), testPrinter(), testShared(), "pc", 631)
	for _, want := range []string{"document decompression failed", "compression=gzip", "client-error-compression-error"} {
		if out := logs.String(); !bytes.Contains([]byte(out), []byte(want)) {
			t.Errorf("log lacks %q:\n%s", want, out)
		}
	}
}
