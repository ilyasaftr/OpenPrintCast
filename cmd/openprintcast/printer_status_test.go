package main

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ilyasaftr/OpenPrintCast/internal/winspool"
)

// fakeSource answers QueryStatus from a mutable map.
type fakeSource struct {
	mu       sync.Mutex
	statuses map[string]winspool.PrinterStatus
}

func newFakeSource(statuses map[string]winspool.PrinterStatus) *fakeSource {
	return &fakeSource{statuses: statuses}
}

func (f *fakeSource) set(name string, s winspool.PrinterStatus) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statuses[name] = s
}

func (f *fakeSource) QueryStatus(name string) winspool.PrinterStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.statuses[name]; ok {
		return s
	}
	return winspool.PrinterStatusUnknown
}

// fakeReasonSource adds IPP reasons to fakeSource.
type fakeReasonSource struct {
	*fakeSource
	reasons map[string][]string
	ok      bool
}

func (f fakeReasonSource) IPPStatus(name string) (int, []string, bool) {
	return 0, f.reasons[name], f.ok
}

func TestRefreshDetectsChangesOnce(t *testing.T) {
	src := newFakeSource(map[string]winspool.PrinterStatus{"A": winspool.PrinterStatusReady})
	m := newStatusMonitor(src, nil)
	if _, ok := m.Condition("A"); ok {
		t.Fatal("a printer that was never polled must have no condition")
	}
	if !m.refresh([]string{"A"}) {
		t.Fatal("first poll must report a change")
	}
	if c, ok := m.Condition("A"); !ok || c.status != winspool.PrinterStatusReady {
		t.Fatalf("condition = %+v, %t", c, ok)
	}
	if m.refresh([]string{"A"}) {
		t.Fatal("an unchanged poll must not report a change")
	}
	src.set("A", winspool.PrinterStatusOffline)
	if !m.refresh([]string{"A"}) {
		t.Fatal("a new status must report a change")
	}
	if c, _ := m.Condition("A"); c.status != winspool.PrinterStatusOffline {
		t.Fatalf("cache not updated: %+v", c)
	}
}

func TestRefreshUnknownPrinterIsCachedAsUnknown(t *testing.T) {
	m := newStatusMonitor(newFakeSource(nil), nil)
	m.refresh([]string{"Ghost"})
	if c, ok := m.Condition("Ghost"); !ok || c.status != winspool.PrinterStatusUnknown {
		t.Fatalf("condition = %+v, %t", c, ok)
	}
}

func TestRefreshDropsPrintersNoLongerListed(t *testing.T) {
	src := newFakeSource(map[string]winspool.PrinterStatus{"A": winspool.PrinterStatusReady, "B": winspool.PrinterStatusReady})
	m := newStatusMonitor(src, nil)
	m.refresh([]string{"A", "B"})
	if m.refresh([]string{"A"}) {
		t.Fatal("removing a printer is not a visible change")
	}
	if _, ok := m.Condition("B"); ok {
		t.Fatal("removed printer is still cached")
	}
}

func TestRefreshUsesReasonsWhenSourceProvidesThem(t *testing.T) {
	base := newFakeSource(map[string]winspool.PrinterStatus{"A": winspool.PrinterStatusOffline})
	src := fakeReasonSource{fakeSource: base, reasons: map[string][]string{"A": {"media-empty"}}, ok: true}
	m := newStatusMonitor(src, nil)
	m.refresh([]string{"A"})
	if c, _ := m.Condition("A"); len(c.reasons) != 1 || c.reasons[0] != "media-empty" {
		t.Fatalf("reasons not captured: %+v", c)
	}
	src.reasons["A"] = []string{"media-jam"}
	if !m.refresh([]string{"A"}) {
		t.Fatal("changed reasons must report a change")
	}
}

func TestRefreshIgnoresReasonsWhenUnavailable(t *testing.T) {
	base := newFakeSource(map[string]winspool.PrinterStatus{"A": winspool.PrinterStatusOffline})
	for name, src := range map[string]statusSource{
		"not implemented": base,
		"not ok":          fakeReasonSource{fakeSource: base, reasons: map[string][]string{"A": {"media-empty"}}, ok: false},
	} {
		m := newStatusMonitor(src, nil)
		m.refresh([]string{"A"})
		if c, _ := m.Condition("A"); c.reasons != nil {
			t.Errorf("%s: unexpected reasons %v", name, c.reasons)
		}
	}
}

func TestNilMonitorKnowsNothing(t *testing.T) {
	var m *statusMonitor
	if _, ok := m.Condition("A"); ok {
		t.Fatal("nil monitor reported a condition")
	}
}

func TestRunNotifiesOnlyOnChangeAndStopsOnCancel(t *testing.T) {
	src := newFakeSource(map[string]winspool.PrinterStatus{"A": winspool.PrinterStatusReady})
	var changes atomic.Int32
	notified := make(chan struct{}, 8)
	m := newStatusMonitor(src, func() { changes.Add(1); notified <- struct{}{} })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		m.Run(ctx, func() []string { return []string{"A"} }, time.Millisecond)
		close(done)
	}()

	waitFor(t, notified, "the first poll")
	time.Sleep(30 * time.Millisecond) // many unchanged polls
	if got := changes.Load(); got != 1 {
		t.Fatalf("onChange ran %d times for unchanged polls, want 1", got)
	}
	src.set("A", winspool.PrinterStatusPaused)
	waitFor(t, notified, "the status change")
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on cancel")
	}
}

func waitFor(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func TestNameSnapshotIsACopy(t *testing.T) {
	var s nameSnapshot
	names := []string{"A"}
	s.Set(names)
	names[0] = "changed"
	got := s.Get()
	if len(got) != 1 || got[0] != "A" {
		t.Fatalf("snapshot aliases the caller's slice: %v", got)
	}
	got[0] = "changed"
	if s.Get()[0] != "A" {
		t.Fatal("Get exposes the internal slice")
	}
}

func TestPublishingPrintersFeedsThePollerWithoutRacing(t *testing.T) {
	app := newTestApp(t)
	src := newFakeSource(map[string]winspool.PrinterStatus{"EPSON L3210 Series": winspool.PrinterStatusReady})
	app.printerStatus = newStatusMonitor(src, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		app.printerStatus.Run(ctx, app.printerNames.Get, time.Millisecond)
		close(done)
	}()
	for i := 0; i < 50; i++ {
		app.printers[0].Enabled = !app.printers[0].Enabled // UI-thread mutation
		app.publishPrinters()
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	if _, ok := app.printerStatus.Condition("EPSON L3210 Series"); !ok {
		t.Fatal("the poller never saw the published printer names")
	}
}
