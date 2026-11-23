package net

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/OpenPrinting/goipp"
	pdfium "github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
	"github.com/ilyasaftr/OpenPrintCast/internal/winspool"
)

// verifierPool is the PDFium pool that reopens saved files; it starts once
// for the whole test run.
var verifierPool = sync.OnceValues(func() (pdfium.Pool, error) {
	return webassembly.Init(webassembly.Config{MaxTotal: 1, MinIdle: 1, MaxIdle: 1})
})

// savedPageCount reopens a saved PDF with PDFium and returns its page count.
func savedPageCount(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := verifierPool()
	if err != nil {
		t.Fatal(err)
	}
	return pageCountOf(t, pool, data)
}

func pageCountOf(t *testing.T, pool pdfium.Pool, data []byte) int {
	t.Helper()
	instance, err := pool.GetInstance(30 * time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer instance.Close()
	doc, err := instance.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		t.Fatalf("PDFium cannot open the saved PDF: %v", err)
	}
	defer instance.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
	count, err := instance.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		t.Fatal(err)
	}
	return count.PageCount
}

// cupsDocument returns a sample document of the CUPS test suite, skipping
// the test when scripts/fetch-cups-testdata.sh has not been run.
func cupsDocument(t *testing.T, name string) []byte {
	t.Helper()
	env := ipptoolEnv{testDir: cupsTestDir(), dataDir: testDataDir()}
	path, ok := env.findDocument(name)
	if !ok {
		t.Skipf("%s not found: run %s", name, fetchScript)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// folderShared serves the folder destination with the real print backend.
func folderShared(t *testing.T) (core.PrinterInfo, *SharedState) {
	t.Helper()
	printer := folderTestPrinter(t)
	shared := NewSharedState([]core.PrinterInfo{printer}, nil)
	shared.SetSpooler(winspool.NewSystemBackend())
	return printer, shared
}

// pdfiumStartTimeout bounds a folder job that starts the PDFium runtime,
// which compiles its WebAssembly module and is slow on a busy machine.
const pdfiumStartTimeout = time.Minute

// waitFinishedJobs waits for the one logged job to finish.
func waitFinishedJobs(t *testing.T, shared *SharedState) []core.JobEntry {
	t.Helper()
	deadline := time.Now().Add(pdfiumStartTimeout)
	for time.Now().Before(deadline) {
		if entries := shared.Jobs.List(); len(entries) == 1 && allFinished(entries) {
			return entries
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the job did not finish: %+v", shared.Jobs.List())
	return nil
}

func waitSavedPDF(t *testing.T, shared *SharedState, printer core.PrinterInfo) string {
	t.Helper()
	entries := waitFinishedJobs(t, shared)
	if entries[0].Status != core.JobCompleted {
		t.Fatalf("job log = %+v", entries)
	}
	paths, err := filepath.Glob(filepath.Join(printer.FolderPath, "*.pdf"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("saved files = %v, error = %v", paths, err)
	}
	if rest, _ := os.ReadDir(printer.FolderPath); len(rest) != 1 {
		t.Fatalf("folder holds more than the saved PDF: %v", rest)
	}
	return paths[0]
}

func TestPrintJobToFolderSavesAPDFThatReopens(t *testing.T) {
	document := cupsDocument(t, requestDocument)
	printer, shared := folderShared(t)
	req := goipp.NewRequest(ipp11, goipp.OpPrintJob, 1)
	req.Operation.Add(goipp.MakeAttr("job-name", goipp.TagName, goipp.String(requestDocument)))
	rsp := serveIPP(req, document, printer, shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusOk {
		t.Fatalf("Print-Job: %s", goipp.Status(rsp.Code))
	}
	path := waitSavedPDF(t, shared, printer)
	if pages := savedPageCount(t, path); pages != 1 {
		t.Fatalf("saved PDF has %d pages, want 1", pages)
	}
	if base := filepath.Base(path); len(base) < len("onepage-a4-1.pdf") || base[:len("onepage-a4-")] != "onepage-a4-" {
		t.Fatalf("saved name %q is not derived from the job name", base)
	}
}

func TestCreateJobAndSendDocumentToFolderSavesAPDFThatReopens(t *testing.T) {
	document := cupsDocument(t, requestDocument)
	printer, shared := folderShared(t)
	rsp := serveIPP(goipp.NewRequest(ipp11, goipp.OpCreateJob, 1), nil, printer, shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusOk {
		t.Fatalf("Create-Job: %s", goipp.Status(rsp.Code))
	}
	id := responseJobID(t, rsp)
	send := goipp.NewRequest(ipp11, goipp.OpSendDocument, 2)
	send.Operation.Add(goipp.MakeAttr("job-id", goipp.TagInteger, goipp.Integer(id)))
	send.Operation.Add(goipp.MakeAttr("last-document", goipp.TagBoolean, goipp.Boolean(true)))
	if rsp = serveIPP(send, document, printer, shared, "pc", 631); goipp.Status(rsp.Code) != goipp.StatusOk {
		t.Fatalf("Send-Document: %s", goipp.Status(rsp.Code))
	}
	path := waitSavedPDF(t, shared, printer)
	if pages := savedPageCount(t, path); pages != 1 {
		t.Fatalf("saved PDF has %d pages, want 1", pages)
	}
}

func TestPrintJobToFolderReportsUnreadablePDFs(t *testing.T) {
	printer, shared := folderShared(t)
	rsp := serveIPP(goipp.NewRequest(ipp11, goipp.OpPrintJob, 1), []byte("%PDF-1.4\nbroken"), printer, shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusOk {
		t.Fatalf("Print-Job: %s", goipp.Status(rsp.Code))
	}
	entries := waitFinishedJobs(t, shared)
	if entries[0].Status != core.JobFailed || entries[0].Reason != "The PDF could not be read (it may be password-protected or damaged)" {
		t.Fatalf("job log = %+v", entries)
	}
	if rest, _ := os.ReadDir(printer.FolderPath); len(rest) != 0 {
		t.Fatalf("a failed job left %v in the folder", rest)
	}
}

// The same job as TestPrintJobToFolderSavesAPDFThatReopens, sent the way iOS
// sends it: as IPP/2.0.
func TestPrintJobToFolderAtIPP20SavesAPDF(t *testing.T) {
	document := cupsDocument(t, requestDocument)
	printer, shared := folderShared(t)
	req := goipp.NewRequest(ipp20, goipp.OpPrintJob, 1)
	req.Operation.Add(goipp.MakeAttr("job-name", goipp.TagName, goipp.String(requestDocument)))
	rsp := serveIPP(req, document, printer, shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusOk {
		t.Fatalf("Print-Job at IPP/2.0: %s", goipp.Status(rsp.Code))
	}
	if pages := savedPageCount(t, waitSavedPDF(t, shared, printer)); pages != 1 {
		t.Fatalf("saved PDF has %d pages, want 1", pages)
	}
}
