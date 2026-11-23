package main

import (
	"sync"
	"testing"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
	bridgenet "github.com/ilyasaftr/OpenPrintCast/internal/net"
)

// fakeNotifier records every notice; it is safe for the observer goroutine.
type fakeNotifier struct {
	mu      sync.Mutex
	notices []userNotice
}

func (f *fakeNotifier) Notify(notice userNotice) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notices = append(f.notices, notice)
}

func (f *fakeNotifier) got() []userNotice {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]userNotice(nil), f.notices...)
}

type fakeNamer map[string]core.PrinterInfo

func (f fakeNamer) PrinterByID(id string) (core.PrinterInfo, bool) {
	p, ok := f[id]
	return p, ok
}

type fakePaused bool

func (f fakePaused) IsPaused() bool { return bool(f) }

var epsonNamer = fakeNamer{
	"printer-epson": core.NewPrinterInfo("printer-epson", "EPSON L3210 Series", "Office EPSON", "", true),
}

func job(status core.JobStatus, printerID, filename, reason string) core.JobEntry {
	j := core.NewJobEntry(printerID, "Agung", filename, 1)
	j.Status, j.Reason = status, reason
	return j
}

func TestAnnouncerWordsAndFiltersNotices(t *testing.T) {
	failedBody := func(body string) []userNotice {
		return []userNotice{{ID: "job-failed", Title: "Print job failed", Body: body}}
	}
	tests := []struct {
		name    string
		job     core.JobEntry
		enabled bool
		paused  bool
		want    []userNotice
	}{
		{"failed with everything", job(core.JobFailed, "printer-epson", "invoice.pdf", "Out of paper"), true, false,
			failedBody("invoice.pdf on Office EPSON: Out of paper")},
		{"no filename", job(core.JobFailed, "printer-epson", "", "Out of paper"), true, false,
			failedBody("A document on Office EPSON: Out of paper")},
		{"unknown printer", job(core.JobFailed, "printer-gone", "invoice.pdf", "Out of paper"), true, false,
			failedBody("invoice.pdf on printer-gone: Out of paper")},
		{"no reason", job(core.JobFailed, "printer-epson", "invoice.pdf", ""), true, false,
			failedBody("invoice.pdf on Office EPSON: Something went wrong.")},
		{"completed", job(core.JobCompleted, "printer-epson", "invoice.pdf", ""), true, false, nil},
		{"cancelled", job(core.JobCancelled, "printer-epson", "invoice.pdf", "Cancelled"), true, false, nil},
		{"printing", job(core.JobPrinting, "printer-epson", "invoice.pdf", ""), true, false, nil},
		{"disabled", job(core.JobFailed, "printer-epson", "invoice.pdf", "x"), false, false, nil},
		{"paused", job(core.JobFailed, "printer-epson", "invoice.pdf", "x"), true, true, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := &fakeNotifier{}
			newJobFailureAnnouncer(n, epsonNamer, fakePaused(tc.paused), tc.enabled).OnJob(tc.job)
			got := n.got()
			if len(got) != len(tc.want) || (len(got) == 1 && got[0] != tc.want[0]) {
				t.Fatalf("notices = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestAnnouncerSetEnabledFlipsBehaviour(t *testing.T) {
	n := &fakeNotifier{}
	a := newJobFailureAnnouncer(n, epsonNamer, fakePaused(false), true)
	failed := job(core.JobFailed, "printer-epson", "a.pdf", "x")
	a.SetEnabled(false)
	a.OnJob(failed)
	if len(n.got()) != 0 {
		t.Fatal("notified while disabled")
	}
	a.SetEnabled(true)
	a.OnJob(failed)
	if len(n.got()) != 1 {
		t.Fatal("did not notify after re-enabling")
	}
}

func TestAnnouncerObservesRealJobLog(t *testing.T) {
	log := &core.JobLog{}
	shared := bridgenet.NewSharedState(nil, log)
	n := &fakeNotifier{}
	log.Observe(newJobFailureAnnouncer(n, shared, shared, true).OnJob)

	entry := job(core.JobPrinting, "printer-epson", "a.pdf", "")
	log.Push(entry)
	if len(n.got()) != 0 {
		t.Fatalf("in-flight push notified: %+v", n.got())
	}
	done := make(chan struct{})
	go func() { log.Update(entry.ID, core.JobFailed, "Printer offline"); close(done) }()
	<-done
	got := n.got()
	if len(got) != 1 || got[0].Body != "a.pdf on printer-epson: Printer offline" {
		t.Fatalf("notices after failed update = %+v", got)
	}
}
