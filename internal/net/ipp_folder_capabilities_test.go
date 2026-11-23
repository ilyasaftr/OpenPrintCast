package net

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// tripwireSpooler offers every capability a Windows backend can, with rich
// answers, and records the printer name of every call. A folder destination
// must never reach it by name.
type tripwireSpooler struct {
	mu     sync.Mutex
	names  []string
	folder []string // mime of every folder job
}

func (s *tripwireSpooler) saw(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.names = append(s.names, name)
}

func (s *tripwireSpooler) SpoolBuffer(p string, _ []byte, _ string) error { s.saw(p); return nil }

func (s *tripwireSpooler) SpoolJob(p string, _ []byte, _ string, _ core.PrintOptions) error {
	s.saw(p)
	return nil
}

func (s *tripwireSpooler) SpoolJobContext(_ context.Context, p string, _ []byte, _ string, _ core.PrintOptions) error {
	s.saw(p)
	return nil
}

func (s *tripwireSpooler) SpoolFolderJobContext(_ context.Context, _ []byte, mime string, _ core.PrintOptions, _, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.folder = append(s.folder, mime)
	return nil
}

func (s *tripwireSpooler) DefaultMedia(p string) (int, int, bool) {
	s.saw(p)
	return 14800, 21000, true
}

func (s *tripwireSpooler) DefaultMediaDetails(p string) (core.MediaDetails, bool) {
	s.saw(p)
	return core.MediaDetails{Width: 14800, Height: 21000, DPIX: 600, DPIY: 600}, true
}

func (s *tripwireSpooler) SupportedMedia(p string) []core.MediaSize {
	s.saw(p)
	return []core.MediaSize{{Width: 14800, Height: 21000, PaperID: 11}}
}

func (s *tripwireSpooler) MediaReady(p string) bool { s.saw(p); return false }

func (s *tripwireSpooler) OrientationSupported(p string) bool { s.saw(p); return true }

func (s *tripwireSpooler) DuplexSupported(p string) bool { s.saw(p); return true }

func (s *tripwireSpooler) PrintQualities(p string) ([]int, int, bool) {
	s.saw(p)
	return []int{printQualityDraft, printQualityNormal, printQualityHigh}, printQualityNormal, true
}

func (s *tripwireSpooler) SupplyLevels(p string) ([]core.Supply, string) {
	s.saw(p)
	return []core.Supply{{Name: "Black", Level: 5}}, "10.0.0.9"
}

func (s *tripwireSpooler) IPPStatus(p string) (int, []string, bool) {
	s.saw(p)
	return printerStateStopped, []string{"toner-empty"}, true
}

func (s *tripwireSpooler) PagesPerMinute(p string) (int, bool) { s.saw(p); return 30, true }

func (s *tripwireSpooler) ColorPagesPerMinute(p string) (int, bool) { s.saw(p); return 20, true }

func (s *tripwireSpooler) IdentifyPrinter(p string) error { s.saw(p); return nil }

// folderTestPrinter is the destination as the application publishes it.
func folderTestPrinter(t *testing.T) core.PrinterInfo {
	t.Helper()
	color, duplex := true, false
	printer := core.NewPrinterInfo("bridge-print-to-folder", core.FolderPrinterName, "Print to Folder (pc)", "", true)
	printer.ColorCapable, printer.DuplexCapable = &color, &duplex
	printer.FolderPath = t.TempDir()
	return printer
}

func requireNoWindowsCalls(t *testing.T, spool *tripwireSpooler) {
	t.Helper()
	spool.mu.Lock()
	defer spool.mu.Unlock()
	if len(spool.names) != 0 {
		t.Fatalf("the folder destination reached the print backend with %q", spool.names)
	}
}

func folderAttrs(t *testing.T, printer core.PrinterInfo, shared *SharedState) goipp.Attributes {
	t.Helper()
	req := goipp.NewRequest(ipp11, goipp.OpGetPrinterAttributes, 1)
	rsp := serveIPP(req, nil, printer, shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusOk {
		t.Fatalf("Get-Printer-Attributes: %s", goipp.Status(rsp.Code))
	}
	return rsp.Printer
}

func TestFolderDestinationNeverReachesTheBackendByName(t *testing.T) {
	printer := folderTestPrinter(t)
	spool := &tripwireSpooler{}
	shared := NewSharedState([]core.PrinterInfo{printer}, nil)
	shared.SetSpooler(spool)

	folderAttrs(t, printer, shared)
	for _, name := range []string{"print-quality", "orientation-requested", "sides", "media"} {
		req := goipp.NewRequest(ipp11, goipp.OpValidateJob, 2)
		req.Operation.Add(goipp.MakeAttr("ipp-attribute-fidelity", goipp.TagBoolean, goipp.Boolean(true)))
		req.Job.Add(goipp.MakeAttr(name, goipp.TagKeyword, goipp.String("x")))
		serveIPP(req, nil, printer, shared, "pc", 631)
	}
	serveIPP(goipp.NewRequest(ipp11, goipp.OpIdentifyPrinter, 3), nil, printer, shared, "pc", 631)
	for _, op := range []goipp.Op{goipp.OpPrintJob, goipp.OpCreateJob} {
		serveIPP(goipp.NewRequest(ipp11, op, 4), []byte("%PDF-1.4"), printer, shared, "pc", 631)
	}
	requireNoWindowsCalls(t, spool)
	folderAttrs(t, testPrinter(), shared)
	if len(spool.names) == 0 {
		t.Fatal("the tripwire does not record the calls of an ordinary printer")
	}
	if txt := core.BuildAirPrintTXT(printer, "uuid", "pc", "10.0.0.1", 631); txt["product"] != "(Print to Folder)" {
		t.Fatalf("product = %q", txt["product"])
	}
}

func TestFolderDestinationAnswersWithItsOwnCapabilities(t *testing.T) {
	printer := folderTestPrinter(t)
	spool := &tripwireSpooler{}
	shared := NewSharedState([]core.PrinterInfo{printer}, nil)
	shared.SetSpooler(spool)
	attrs := folderAttrs(t, printer, shared)
	want := map[string][]string{
		"media-default":                   {"iso_a4_210x297mm"},
		"media-supported":                 {"iso_a4_210x297mm", "na_letter_8.5x11in"},
		"printer-resolution-default":      {"300x300dpi"},
		"sides-supported":                 {"one-sided"},
		"print-quality-supported":         {"4"},
		"printer-state":                   {"3"},
		"printer-state-reasons":           {"none"},
		"printer-is-accepting-jobs":       {"true"},
		"color-supported":                 {"true"},
		"ipp-versions-supported":          {"1.1", "2.0"},
		"orientation-requested-supported": {"3", "4", "5", "6"},
	}
	for name, values := range want {
		if got := attrValues(t, attrs, name); !slices.Equal(got, values) {
			t.Errorf("%s = %v, want %v", name, got, values)
		}
	}
	for _, name := range []string{
		"marker-names", "marker-levels", "printer-supply", "pages-per-minute", "pages-per-minute-color",
	} {
		// An attribute without data is absent or carries an unknown value.
		for _, value := range attrValues(t, attrs, name) {
			if value != "" {
				t.Errorf("%s = %v, want no data", name, value)
			}
		}
	}
	if ops := attrValues(t, attrs, "operations-supported"); slices.Contains(ops, "Identify-Printer") || slices.Contains(ops, "60") {
		t.Errorf("the folder destination offers Identify-Printer: %v", ops)
	}
	if id := attrValues(t, attrs, "printer-device-id"); len(id) != 1 || !strings.Contains(id[0], "MDL:Print to Folder;") {
		t.Errorf("printer-device-id = %v", id)
	}
	requireNoWindowsCalls(t, spool)
}

func TestFolderJobIsSavedOnceRegardlessOfCopies(t *testing.T) {
	printer := folderTestPrinter(t)
	spool := &tripwireSpooler{}
	shared := NewSharedState([]core.PrinterInfo{printer}, nil)
	shared.SetSpooler(spool)
	req := goipp.NewRequest(ipp11, goipp.OpPrintJob, 1)
	req.Job.Add(goipp.MakeAttr("copies", goipp.TagInteger, goipp.Integer(5)))
	rsp := serveIPP(req, []byte("%PDF-1.4"), printer, shared, "pc", 631)
	if goipp.Status(rsp.Code) != goipp.StatusOk {
		t.Fatalf("Print-Job: %s", goipp.Status(rsp.Code))
	}
	waitJobLog(t, shared, 1)
	spool.mu.Lock()
	defer spool.mu.Unlock()
	if len(spool.folder) != 1 || len(spool.names) != 0 {
		t.Fatalf("folder jobs = %v, backend calls = %v", spool.folder, spool.names)
	}
}

// iOS and macOS speak IPP/2.0. A folder destination that is only served as
// IPP/1.1 answers every one of their requests with
// server-error-version-not-supported and nothing can be printed to it.
func TestFolderDestinationIsServedAsIPP20(t *testing.T) {
	printer := folderTestPrinter(t)
	shared := NewSharedState([]core.PrinterInfo{printer}, nil)
	shared.SetSpooler(&tripwireSpooler{})

	rsp := serveIPP(goipp.NewRequest(ipp20, goipp.OpGetPrinterAttributes, 1), nil, printer, shared, "pc", 631)
	if status := goipp.Status(rsp.Code); status != goipp.StatusOk {
		t.Fatalf("Get-Printer-Attributes at IPP/2.0: %s", status)
	}
	if got := attrValues(t, rsp.Printer, "ipp-versions-supported"); !slices.Contains(got, "2.0") {
		t.Fatalf("ipp-versions-supported = %v, want 2.0 included", got)
	}

	validate := goipp.NewRequest(ipp20, goipp.OpValidateJob, 2)
	validate.Operation.Add(goipp.MakeAttr("document-format", goipp.TagMimeType, goipp.String("application/pdf")))
	if status := goipp.Status(serveIPP(validate, nil, printer, shared, "pc", 631).Code); status != goipp.StatusOk {
		t.Fatalf("Validate-Job at IPP/2.0: %s", status)
	}
}
