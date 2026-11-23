package net

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/OpenPrinting/goipp"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

func testPrinter() core.PrinterInfo {
	color := true
	return core.PrinterInfo{
		ID:           "hp",
		WindowsName:  "HP LaserJet",
		AirPrintName: "HP Air",
		Location:     "Office",
		Enabled:      true,
		ColorCapable: &color,
	}
}

// fakeSpooler records spool calls without touching any OS queue.
type fakeSpooler struct {
	mu              sync.Mutex
	calls           int
	last            string
	lastOptions     core.PrintOptions
	err             error
	identified      int
	reportedState   int
	reportedReasons []string
}

type mediaSpooler struct{ *fakeSpooler }

// duplexSpooler is a driver that reports and accepts duplex printing.
type duplexSpooler struct{ *mediaSpooler }

func (*duplexSpooler) DuplexSupported(string) bool { return true }

type unknownDefaultMediaSpooler struct{ *mediaSpooler }

func (f *unknownDefaultMediaSpooler) DefaultMedia(string) (int, int, bool) {
	return 0, 0, false
}

func (f *unknownDefaultMediaSpooler) DefaultMediaDetails(string) (core.MediaDetails, bool) {
	return core.MediaDetails{}, false
}

func (f *mediaSpooler) SupportedMedia(string) []core.MediaSize {
	return []core.MediaSize{
		{Width: 21000, Height: 29700, PaperID: 9},
		{Width: 21590, Height: 27940, PaperID: 1, Left: 200, Right: 200, Top: 250, Bottom: 250, MarginsKnown: true},
	}
}

func (f *fakeSpooler) SpoolJob(printer string, data []byte, mime string, options core.PrintOptions) error {
	f.mu.Lock()
	f.lastOptions = options
	f.calls++
	f.last = printer + "|" + mime + "|" + string(rune('0'+len(data)%10))
	err := f.err
	f.mu.Unlock()
	return err
}

func (f *fakeSpooler) SpoolBuffer(printer string, data []byte, mime string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.last = printer + "|" + mime + "|" + string(rune('0'+len(data)%10))
	return f.err
}

func (f *fakeSpooler) DefaultMedia(string) (int, int, bool) { return 21000, 29700, true }

func (f *fakeSpooler) DefaultMediaDetails(string) (core.MediaDetails, bool) {
	return core.MediaDetails{
		Width: 21000, Height: 29700, Left: 300, Right: 300, Top: 400, Bottom: 400,
		DPIX: 600, DPIY: 600, MarginsKnown: true,
	}, true
}

func (f *fakeSpooler) MediaReady(string) bool { return true }

func (f *fakeSpooler) OrientationSupported(string) bool { return true }

func (f *fakeSpooler) PagesPerMinute(string) (int, bool) { return 18, true }

func (f *fakeSpooler) ColorPagesPerMinute(string) (int, bool) { return 12, true }

func (f *fakeSpooler) IPPStatus(string) (int, []string, bool) {
	return f.reportedState, f.reportedReasons, f.reportedState != 0
}

func (f *fakeSpooler) IdentifyPrinter(string) error {
	f.identified++
	return f.err
}

func testShared() *SharedState {
	s := NewSharedState([]core.PrinterInfo{testPrinter()}, &core.JobLog{})
	s.SetSpooler(&fakeSpooler{})
	return s
}

func waitIPPJob(t *testing.T, shared *SharedState, id uint32) ippJob {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if job, ok := shared.getIPPJob("hp", id); ok && job.state >= 7 {
			return job
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("job %d did not finish", id)
	return ippJob{}
}

func responseJobID(t *testing.T, rsp *goipp.Message) uint32 {
	t.Helper()
	values := attrValues(t, rsp.Job, "job-id")
	if len(values) != 1 {
		t.Fatalf("missing job-id in response: %v", rsp)
	}
	id, err := strconv.ParseUint(values[0], 10, 32)
	if err != nil {
		t.Fatal(err)
	}
	return uint32(id)
}

// waitJobLog waits until the log holds at least count entries and every one
// of them is finished (in-flight jobs are logged as printing).
func waitJobLog(t *testing.T, shared *SharedState, count int) []core.JobEntry {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		entries := shared.Jobs.List()
		if len(entries) >= count && allFinished(entries) {
			return entries
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("expected %d finished job log entries, have %+v", count, shared.Jobs.List())
	return nil
}

func allFinished(entries []core.JobEntry) bool {
	for _, e := range entries {
		if !e.Status.Finished() {
			return false
		}
	}
	return true
}

// attrValues returns every value string of a named attribute.
func attrValues(t *testing.T, attrs goipp.Attributes, name string) []string {
	t.Helper()
	var out []string
	for _, a := range attrs {
		if a.Name == name {
			for _, v := range a.Values {
				out = append(out, v.V.String())
			}
		}
	}
	return out
}
