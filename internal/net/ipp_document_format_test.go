package net

import (
	"slices"
	"strings"
	"testing"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

const (
	mimePDF  = core.MIMEPDF
	mimeURF  = core.MIMEURF
	mimePWG  = core.MIMEPWG
	mimeJPEG = core.MIMEJPEG
	mimePNG  = core.MIMEPNG
)

var (
	pdfData  = []byte("%PDF-1.4\n")
	jpegData = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0x10}
	pngData  = []byte("\x89PNG\r\n\x1a\n")
	urfData  = []byte("UNIRAST\x00\x00\x00\x00\x01")
	pwgData  = []byte("RaS2")
)

// documentCase is one declared x sniffed combination with the exact outcome
// of a data-carrying request (Print-Job, Send-Document) and of Validate-Job,
// which has no data and so judges the declared type alone.
type documentCase struct {
	name, declared string
	data           []byte
	want           goipp.Status
	mime           string // the type the spooler receives when accepted
	wantValidate   goipp.Status
	rule           documentRule
}

const (
	statusOK        = goipp.StatusOk
	statusNotSup    = goipp.StatusErrorDocumentFormatNotSupported
	statusFormatErr = goipp.StatusErrorDocumentFormatError
)

var documentCases = []documentCase{
	{"canonical pdf", "application/pdf", pdfData, statusOK, mimePDF, statusOK, ruleDeclaredMatchesData},
	{"canonical jpeg", "image/jpeg", jpegData, statusOK, mimeJPEG, statusOK, ruleDeclaredMatchesData},
	{"canonical png", "image/png", pngData, statusOK, mimePNG, statusOK, ruleDeclaredMatchesData},
	{"canonical urf", "image/urf", urfData, statusOK, mimeURF, statusOK, ruleDeclaredMatchesData},
	{"canonical pwg", "image/pwg-raster", pwgData, statusOK, mimePWG, statusOK, ruleDeclaredMatchesData},
	{"cups pdf alias", "application/vnd.cups-pdf", pdfData, statusOK, mimePDF, statusOK, ruleDeclaredMatchesData},
	{"cups pdf banner alias", "application/vnd.cups-pdf-banner", pdfData, statusOK, mimePDF, statusOK, ruleDeclaredMatchesData},
	{"x-pdf alias", "application/x-pdf", pdfData, statusOK, mimePDF, statusOK, ruleDeclaredMatchesData},
	{"jpg alias", "image/jpg", jpegData, statusOK, mimeJPEG, statusOK, ruleDeclaredMatchesData},
	{"upper case", "APPLICATION/PDF", pdfData, statusOK, mimePDF, statusOK, ruleDeclaredMatchesData},
	{"charset parameter", "application/pdf; charset=binary", pdfData, statusOK, mimePDF, statusOK, ruleDeclaredMatchesData},
	{"pdf after line breaks", "application/pdf", append([]byte("\r\n"), pdfData...), statusOK, mimePDF, statusOK, ruleDeclaredMatchesData},
	{"octet-stream pdf", "application/octet-stream", pdfData, statusOK, mimePDF, statusOK, ruleAutoSensed},
	{"octet-stream urf", "application/octet-stream", urfData, statusOK, mimeURF, statusOK, ruleAutoSensed},
	{"octet-stream jpeg", "application/octet-stream", jpegData, statusOK, mimeJPEG, statusOK, ruleAutoSensed},
	{"absent pdf", "", pdfData, statusOK, mimePDF, statusOK, ruleAutoSensed},
	{"absent pwg", "", pwgData, statusOK, mimePWG, statusOK, ruleAutoSensed},
	{"mismatch jpeg declared pdf", "application/pdf", jpegData, statusOK, mimeJPEG, statusOK, ruleTrustedData},
	{"mismatch pdf declared png", "image/png", pdfData, statusOK, mimePDF, statusOK, ruleTrustedData},
	{"unknown type with pdf data", "application/postscript", pdfData, statusNotSup, "", statusNotSup, ruleUnsupportedDeclared},
	{"tiff declared", "image/tiff", []byte("II\x2a\x00"), statusNotSup, "", statusNotSup, ruleUnsupportedDeclared},
	{"cups raster declared", "application/vnd.cups-raster", []byte("RaSt"), statusNotSup, "", statusNotSup, ruleUnsupportedDeclared},
	{"pclm declared", "application/PCLm", pdfData, statusNotSup, "", statusNotSup, ruleUnsupportedDeclared},
	{"known type with unknown data", "application/pdf", []byte("hello world"), statusFormatErr, "", statusOK, ruleDataNotDeclared},
	{"truncated signature", "image/jpeg", []byte{0xFF, 0xD8}, statusFormatErr, "", statusOK, ruleDataNotDeclared},
	{"empty body declared", "application/pdf", nil, statusFormatErr, "", statusOK, ruleDataNotDeclared},
	{"octet-stream unknown data", "application/octet-stream", []byte("%!PS-Adobe"), statusNotSup, "", statusOK, ruleUnrecognizedData},
	{"absent unknown data", "", []byte("hello"), statusNotSup, "", statusOK, ruleUnrecognizedData},
	{"absent empty body", "", nil, statusNotSup, "", statusOK, ruleUnrecognizedData},
}

func TestDecideDocumentTable(t *testing.T) {
	for _, tc := range documentCases {
		t.Run(tc.name, func(t *testing.T) {
			got := decideDocument(tc.data, tc.declared)
			if got.status != tc.want || got.mime != tc.mime || got.rule != tc.rule {
				t.Fatalf("decideDocument(%q) = %+v, want status %s, mime %q, rule %q", tc.declared, got, tc.want, tc.mime, tc.rule)
			}
			if declared := decideDeclared(tc.declared); declared.status != tc.wantValidate {
				t.Fatalf("decideDeclared(%q) = %s, want %s", tc.declared, declared.status, tc.wantValidate)
			}
		})
	}
}

func formatRequest(op goipp.Op, id uint32, declared string) *goipp.Message {
	req := goipp.NewRequest(ipp11, op, id)
	if declared != "" {
		req.Operation.Add(goipp.MakeAttr("document-format", goipp.TagMimeType, goipp.String(declared)))
	}
	return req
}

// submitCreateSend runs Create-Job and Send-Document and returns the Send-Document
// response. The declared format rides on Send-Document, as CUPS sends it.
func submitCreateSend(t *testing.T, shared *SharedState, declared string, data []byte) *goipp.Message {
	t.Helper()
	rsp := serveIPP(goipp.NewRequest(ipp11, goipp.OpCreateJob, 1), nil, testPrinter(), shared, "pc", 631)
	id := responseJobID(t, rsp)
	send := formatRequest(goipp.OpSendDocument, 2, declared)
	send.Operation.Add(goipp.MakeAttr("job-id", goipp.TagInteger, goipp.Integer(id)))
	send.Operation.Add(goipp.MakeAttr("last-document", goipp.TagBoolean, goipp.Boolean(true)))
	rsp = serveIPP(send, data, testPrinter(), shared, "pc", 631)
	if goipp.Status(rsp.Code) == goipp.StatusOk {
		waitIPPJob(t, shared, id)
	}
	return rsp
}

func requireSpooled(t *testing.T, shared *SharedState, status goipp.Status, tc documentCase) {
	t.Helper()
	spooler := shared.Spool.(*fakeSpooler)
	if status != tc.want {
		t.Fatalf("status = %s, want %s", status, tc.want)
	}
	if tc.want == statusOK && (spooler.calls != 1 || !strings.Contains(spooler.last, "|"+tc.mime+"|")) {
		t.Fatalf("spooler = %+v, want one call as %s", spooler, tc.mime)
	}
	if tc.want != statusOK && spooler.calls != 0 {
		t.Fatalf("a refused document reached the spooler: %+v", spooler)
	}
}

func TestDocumentFormatsAcrossOperations(t *testing.T) {
	for _, tc := range documentCases {
		t.Run(tc.name, func(t *testing.T) {
			shared := testShared()
			rsp := serveIPP(formatRequest(goipp.OpPrintJob, 1, tc.declared), tc.data, testPrinter(), shared, "pc", 631)
			if tc.want == statusOK {
				waitIPPJob(t, shared, responseJobID(t, rsp))
			}
			requireSpooled(t, shared, goipp.Status(rsp.Code), tc)

			if len(tc.data) > 0 { // an empty last document closes the job without printing
				shared = testShared()
				rsp = submitCreateSend(t, shared, tc.declared, tc.data)
				requireSpooled(t, shared, goipp.Status(rsp.Code), tc)
			}

			rsp = serveIPP(formatRequest(goipp.OpValidateJob, 1, tc.declared), nil, testPrinter(), testShared(), "pc", 631)
			if got := goipp.Status(rsp.Code); got != tc.wantValidate {
				t.Fatalf("Validate-Job = %s, want %s", got, tc.wantValidate)
			}
		})
	}
}

func TestCreateJobJudgesTheDeclaredFormat(t *testing.T) {
	for declared, want := range map[string]goipp.Status{
		"image/urf": statusOK, "application/vnd.cups-pdf": statusOK, "application/octet-stream": statusOK,
		"": statusOK, "application/postscript": statusNotSup,
	} {
		rsp := serveIPP(formatRequest(goipp.OpCreateJob, 1, declared), nil, testPrinter(), testShared(), "pc", 631)
		if got := goipp.Status(rsp.Code); got != want {
			t.Errorf("Create-Job declared %q = %s, want %s", declared, got, want)
		}
	}
}

func TestPrinterAttributesRejectUnsupportedDocumentFormat(t *testing.T) {
	rsp := serveIPP(formatRequest(goipp.OpGetPrinterAttributes, 1, "application/postscript"), nil, testPrinter(), testShared(), "pc", 631)
	if got := goipp.Status(rsp.Code); got != statusNotSup {
		t.Fatalf("Get-Printer-Attributes for PostScript = %s", got)
	}
}

// TestAdvertisedFormatsAreTheAcceptedFormats ties what the printer says to
// what it does: document-format-supported and the TXT pdl are built from the
// registry, and each advertised type passes the same check a job does.
func TestAdvertisedFormatsAreTheAcceptedFormats(t *testing.T) {
	registry := core.AdvertisedMIMEs()
	rsp := serveIPP(goipp.NewRequest(ipp11, goipp.OpGetPrinterAttributes, 1), nil, testPrinter(), testShared(), "pc", 631)
	supported := attrValues(t, rsp.Printer, "document-format-supported")
	if want := append(slices.Clone(registry), mimeOctetStream); !slices.Equal(supported, want) {
		t.Fatalf("document-format-supported = %v, want the registry plus octet-stream %v", supported, want)
	}
	if got := attrValues(t, rsp.Printer, "document-format-default"); !slices.Equal(got, []string{mimeOctetStream}) {
		t.Fatalf("document-format-default = %v", got)
	}
	pdl := core.BuildAirPrintTXT(testPrinter(), "uuid", "host", "192.0.2.1", 631)["pdl"]
	if !slices.Equal(strings.Split(pdl, ","), registry) {
		t.Fatalf("pdl = %q, want the registry %v", pdl, registry)
	}
	for _, mime := range supported {
		if d := decideDeclared(mime); !d.accepted() {
			t.Errorf("advertised %s is refused: %+v", mime, d)
		}
	}
	for _, unsupported := range []string{"application/postscript", "application/vnd.cups-raster", "application/PCLm", "application/openxps", "image/heic"} {
		if slices.Contains(supported, unsupported) || strings.Contains(pdl, unsupported) {
			t.Errorf("%s is advertised but cannot be printed", unsupported)
		}
	}
}

func TestRejectionLogNamesTheRule(t *testing.T) {
	logs := captureLogs(t)
	declared := "application/vnd.cups-raster"
	doc := []byte("2SaR\x00\x00\x01\x02 and a long body that must not be logged")
	logDocumentDecision("Send-Document", "bridge-print-to-folder", declared, doc, decideDocument(doc, declared))
	out := logs.String()
	for _, want := range []string{
		"document format rejected", "op=Send-Document", "printer=bridge-print-to-folder",
		"declared=application/vnd.cups-raster", "detected=application/octet-stream",
		`rule="declared type is not a supported format"`, "status=client-error-document-format-not-supported",
		"magic=325361520000010220616e64",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "long body") {
		t.Errorf("the document content leaked into the log:\n%s", out)
	}
}

func TestEachRejectingRuleIsLogged(t *testing.T) {
	for _, tc := range documentCases {
		if tc.want == statusOK && tc.rule != ruleTrustedData {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			logs := captureLogs(t)
			serveIPP(formatRequest(goipp.OpPrintJob, 1, tc.declared), tc.data, testPrinter(), testShared(), "pc", 631)
			if out := logs.String(); !strings.Contains(out, string(tc.rule)) {
				t.Fatalf("log lacks the rule %q:\n%s", tc.rule, out)
			}
		})
	}
}
