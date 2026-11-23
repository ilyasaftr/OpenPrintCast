package main

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// fakeCompat answers per printer ID from a mutable map and counts calls.
type fakeCompat struct {
	mu      sync.Mutex
	answers map[string]compatibilityResult
	calls   map[string]int
	panics  bool
}

func newFakeCompat() *fakeCompat {
	return &fakeCompat{answers: map[string]compatibilityResult{}, calls: map[string]int{}}
}

func (f *fakeCompat) set(id string, gaps []string, known bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answers[id] = compatibilityResult{gaps: gaps, known: known}
}

func (f *fakeCompat) callCount(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[id]
}

func (f *fakeCompat) IPP20Gaps(p core.PrinterInfo) ([]string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[p.ID]++
	if f.panics {
		panic("driver fault")
	}
	a := f.answers[p.ID]
	return a.gaps, a.known
}

func sharedPrinter(id string, enabled bool) core.PrinterInfo {
	return core.PrinterInfo{ID: id, WindowsName: "Windows " + id, Enabled: enabled}
}

// stepClock returns a clock that advances one minute per reading.
func stepClock() func() time.Time {
	var mu sync.Mutex
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		now = now.Add(time.Minute)
		return now
	}
}

func TestCompatibilityMonitorChecksOnlySharedPrinters(t *testing.T) {
	src := newFakeCompat()
	src.set("a", []string{"orientation"}, true)
	src.set("b", nil, true)
	m := newCompatibilityMonitor(src, stepClock(), nil)
	folder := core.PrinterInfo{ID: "f", WindowsName: core.FolderPrinterName, Enabled: true}
	changed := m.refresh([]core.PrinterInfo{sharedPrinter("a", true), sharedPrinter("b", false), folder})
	if !changed {
		t.Fatal("first check must report a change")
	}
	if r, ok := m.Result("a"); !ok || !r.Problem() || r.at.IsZero() {
		t.Fatalf("a = %+v, %t", r, ok)
	}
	if _, ok := m.Result("b"); ok || src.callCount("b") != 0 {
		t.Fatal("a printer that is not shared must not be checked")
	}
	if src.callCount("f") != 0 {
		t.Fatal("the folder destination must not be checked")
	}
}

func TestCompatibilityMonitorStampsTimeAndDetectsChanges(t *testing.T) {
	src := newFakeCompat()
	src.set("a", []string{"resolution"}, true)
	m := newCompatibilityMonitor(src, stepClock(), nil)
	list := []core.PrinterInfo{sharedPrinter("a", true)}
	m.refresh(list)
	first, _ := m.Result("a")
	if m.refresh(list) {
		t.Fatal("an unchanged verdict must not report a change")
	}
	second, _ := m.Result("a")
	if !second.at.After(first.at) {
		t.Fatalf("time not renewed: %v then %v", first.at, second.at)
	}
	src.set("a", nil, true)
	if !m.refresh(list) {
		t.Fatal("a recovered printer must report a change")
	}
}

func TestCompatibilityMonitorDropsDisappearedPrinters(t *testing.T) {
	src := newFakeCompat()
	src.set("a", []string{"orientation"}, true)
	m := newCompatibilityMonitor(src, nil, nil)
	m.refresh([]core.PrinterInfo{sharedPrinter("a", true)})
	m.refresh(nil)
	if _, ok := m.Result("a"); ok {
		t.Fatal("a printer that disappeared must leave the cache")
	}
}

func TestCompatibilityMonitorKeepsResultWhenSharingTurnsOff(t *testing.T) {
	src := newFakeCompat()
	src.set("a", []string{"orientation"}, true)
	m := newCompatibilityMonitor(src, nil, nil)
	m.refresh([]core.PrinterInfo{sharedPrinter("a", true)})
	m.refresh([]core.PrinterInfo{sharedPrinter("a", false)})
	if r, ok := m.Result("a"); !ok || !r.Problem() {
		t.Fatalf("result lost: %+v, %t", r, ok)
	}
}

func TestCompatibilityMonitorUnknownNeverReplacesKnown(t *testing.T) {
	src := newFakeCompat()
	src.set("a", []string{"orientation"}, true)
	m := newCompatibilityMonitor(src, nil, nil)
	list := []core.PrinterInfo{sharedPrinter("a", true)}
	m.refresh(list)
	src.set("a", nil, false)
	if m.refresh(list) {
		t.Fatal("an undetermined check is not a change")
	}
	if r, _ := m.Result("a"); !r.Problem() {
		t.Fatalf("known result replaced by unknown: %+v", r)
	}
	src2 := newFakeCompat()
	m2 := newCompatibilityMonitor(src2, nil, nil)
	m2.refresh(list)
	if r, ok := m2.Result("a"); !ok || r.known {
		t.Fatalf("an undetermined first check must be cached as unknown: %+v, %t", r, ok)
	}
}

func TestCompatibilityMonitorSurvivesDriverFault(t *testing.T) {
	src := newFakeCompat()
	src.panics = true
	m := newCompatibilityMonitor(src, nil, nil)
	m.refresh([]core.PrinterInfo{sharedPrinter("a", true)})
	if r, ok := m.Result("a"); !ok || r.known {
		t.Fatalf("a fault must read as undetermined: %+v, %t", r, ok)
	}
}

func TestCompatibilityMonitorNilIsSafe(t *testing.T) {
	var m *compatibilityMonitor
	if _, ok := m.Result("a"); ok {
		t.Fatal("a nil monitor knows nothing")
	}
	m.Nudge()
}

func TestCompatibilityMonitorLogsChangesOnce(t *testing.T) {
	out := captureLogs(t)
	src := newFakeCompat()
	src.set("a", []string{"paper-sizes", "resolution"}, true)
	m := newCompatibilityMonitor(src, nil, nil)
	list := []core.PrinterInfo{sharedPrinter("a", true)}
	for range 3 {
		m.refresh(list)
	}
	log := out.String()
	if n := strings.Count(log, "printer cannot be served as IPP/2.0"); n != 1 {
		t.Fatalf("warned %d times, want once:\n%s", n, log)
	}
	for _, want := range []string{"level=WARN", "printer=a", "gaps=paper-sizes,resolution", "component=app"} {
		if !strings.Contains(log, want) {
			t.Errorf("log lacks %q:\n%s", want, log)
		}
	}
	src.set("a", nil, true)
	m.refresh(list)
	m.refresh(list)
	log = out.String()
	if n := strings.Count(log, "can be served as IPP/2.0 again"); n != 1 || !strings.Contains(log, "level=INFO") {
		t.Fatalf("recovery logged %d times:\n%s", n, log)
	}
}

func TestCompatibilityMonitorDoesNotLogHealthyOrUnknown(t *testing.T) {
	out := captureLogs(t)
	src := newFakeCompat()
	src.set("a", nil, true)
	m := newCompatibilityMonitor(src, nil, nil)
	m.refresh([]core.PrinterInfo{sharedPrinter("a", true), sharedPrinter("b", true)})
	if strings.Contains(out.String(), "IPP/2.0") {
		t.Fatalf("unexpected log:\n%s", out.String())
	}
}

// waitUntil polls cond until it holds or the test times out.
func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestCompatibilityMonitorRunChecksAtStartOnTickAndNudge(t *testing.T) {
	src := newFakeCompat()
	src.set("a", nil, true)
	m := newCompatibilityMonitor(src, nil, nil)
	var list printerSnapshot
	list.Set([]core.PrinterInfo{sharedPrinter("a", true)})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.Run(ctx, list.Get, time.Hour); close(done) }()

	waitUntil(t, "the start check", func() bool { return src.callCount("a") == 1 })
	m.Nudge()
	waitUntil(t, "the nudged check", func() bool { return src.callCount("a") == 2 })

	list.Set([]core.PrinterInfo{sharedPrinter("a", true), sharedPrinter("b", true)})
	m.Nudge()
	waitUntil(t, "the new printer", func() bool { return src.callCount("b") == 1 })
	cancel()
	<-done
}

func TestCompatibilityMonitorRunChecksOnInterval(t *testing.T) {
	src := newFakeCompat()
	m := newCompatibilityMonitor(src, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		m.Run(ctx, func() []core.PrinterInfo { return []core.PrinterInfo{sharedPrinter("a", true)} }, time.Millisecond)
		close(done)
	}()
	waitUntil(t, "repeated checks", func() bool { return src.callCount("a") >= 3 })
	cancel()
	<-done
}

func TestCompatibilityMonitorCallsOnChangeOnlyOnChange(t *testing.T) {
	src := newFakeCompat()
	src.set("a", []string{"orientation"}, true)
	var mu sync.Mutex
	calls := 0
	m := newCompatibilityMonitor(src, nil, func() { mu.Lock(); calls++; mu.Unlock() })
	var list printerSnapshot
	list.Set([]core.PrinterInfo{sharedPrinter("a", true)})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.Run(ctx, list.Get, time.Hour); close(done) }()
	count := func() int { mu.Lock(); defer mu.Unlock(); return calls }
	waitUntil(t, "the first change", func() bool { return count() == 1 })
	m.Nudge()
	waitUntil(t, "the second check", func() bool { return src.callCount("a") == 2 })
	cancel()
	<-done
	if count() != 1 {
		t.Fatalf("onChange ran %d times, want 1", count())
	}
}

func TestPrinterSnapshotIsACopy(t *testing.T) {
	var s printerSnapshot
	in := []core.PrinterInfo{sharedPrinter("a", true)}
	s.Set(in)
	in[0].ID = "changed"
	out := s.Get()
	out[0].ID = "other"
	if got := s.Get(); !slices.EqualFunc(got, []core.PrinterInfo{sharedPrinter("a", true)}, func(x, y core.PrinterInfo) bool { return x.ID == y.ID }) {
		t.Fatalf("snapshot = %+v", got)
	}
}
