package winspool

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// storeHarness drives a supplyStore with a manual clock and refreshes that
// run only when the test says so: no sleeping, no goroutines, no network.
type storeHarness struct {
	store   *supplyStore
	clock   time.Time
	pending []func()
	fetches int
	result  supplyReading
	err     error
	lastCtx context.Context
}

const testPrinterName = "Office Laser"

var cyanToner = core.Supply{Name: "Cyan", Color: "cyan", Kind: core.SupplyToner, Level: 40}

func newStoreHarness() *storeHarness {
	h := &storeHarness{clock: time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)}
	h.result = supplyReading{Supplies: []core.Supply{cyanToner}, Host: "192.168.1.50"}
	h.store = newSupplyStore(func(ctx context.Context, printer string) (supplyReading, error) {
		h.fetches++
		h.lastCtx = ctx
		return h.result, h.err
	})
	h.store.now = func() time.Time { return h.clock }
	h.store.start = func(refresh func()) { h.pending = append(h.pending, refresh) }
	h.store.SetEnabled(true)
	return h
}

// runPending runs the queued refreshes.
func (h *storeHarness) runPending() {
	queued := h.pending
	h.pending = nil
	for _, refresh := range queued {
		refresh()
	}
}

func (h *storeHarness) advance(d time.Duration) { h.clock = h.clock.Add(d) }

func (h *storeHarness) get() supplyReading { return h.store.Get(testPrinterName) }

func TestSupplyStoreNeverWaitsForTheFirstRead(t *testing.T) {
	h := newStoreHarness()
	if got := h.get(); got.Supplies != nil {
		t.Fatalf("first answer has data before any read: %+v", got)
	}
	if h.fetches != 0 || len(h.pending) != 1 {
		t.Fatalf("fetches=%d pending=%d: the read must be queued, not run", h.fetches, len(h.pending))
	}
	h.runPending()
	if got := h.get(); len(got.Supplies) != 1 || got.Host != "192.168.1.50" {
		t.Fatalf("after the read: %+v", got)
	}
}

func TestSupplyStoreSingleFlight(t *testing.T) {
	h := newStoreHarness()
	for range 5 {
		h.get()
	}
	if len(h.pending) != 1 {
		t.Fatalf("%d refreshes queued for one printer, want 1", len(h.pending))
	}
	h.runPending()
	h.advance(supplyCacheTTL)
	for range 5 {
		h.get()
	}
	if len(h.pending) != 1 {
		t.Fatalf("%d refreshes queued after the TTL, want 1", len(h.pending))
	}
	if other := h.store.Get("Other Printer"); other.Supplies != nil || len(h.pending) != 2 {
		t.Fatalf("another printer needs its own refresh: %+v, %d pending", other, len(h.pending))
	}
}

func TestSupplyStoreServesStaleWhileRevalidating(t *testing.T) {
	h := newStoreHarness()
	h.get()
	h.runPending()

	h.advance(supplyCacheTTL - time.Second)
	h.get()
	if len(h.pending) != 0 || h.fetches != 1 {
		t.Fatalf("fresh value refreshed early: pending=%d fetches=%d", len(h.pending), h.fetches)
	}

	h.advance(time.Second)
	h.result = supplyReading{Supplies: []core.Supply{{Name: "Cyan", Color: "cyan", Kind: core.SupplyToner, Level: 35}}}
	stale := h.get()
	if len(stale.Supplies) != 1 || stale.Supplies[0].Level != 40 {
		t.Fatalf("stale value not served during the refresh: %+v", stale)
	}
	h.runPending()
	if fresh := h.get(); fresh.Supplies[0].Level != 35 {
		t.Fatalf("refresh not applied: %+v", fresh)
	}
}

func TestSupplyStoreBacksOffExponentiallyAndRecovers(t *testing.T) {
	h := newStoreHarness()
	h.err = errors.New("request timeout")
	h.get()
	h.runPending()
	for _, wait := range []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 5 * time.Minute, 5 * time.Minute} {
		h.advance(wait - time.Second)
		h.get()
		if len(h.pending) != 0 {
			t.Fatalf("retried %v after the failure, before the backoff of %v", wait-time.Second, wait)
		}
		h.advance(time.Second)
		h.get()
		if len(h.pending) != 1 {
			t.Fatalf("no retry after the backoff of %v", wait)
		}
		h.runPending()
	}
	h.err = nil
	h.advance(supplyBackoffMax)
	h.get()
	h.runPending()
	if got := h.get(); len(got.Supplies) != 1 {
		t.Fatalf("no recovery: %+v", got)
	}
	// A success resets the streak: the next failure waits the base again.
	h.err = errors.New("timeout")
	h.advance(supplyCacheTTL)
	h.get()
	h.runPending()
	h.advance(supplyBackoffBase)
	h.get()
	if len(h.pending) != 1 {
		t.Fatal("backoff did not restart from the base after a success")
	}
}

func TestSupplyBackoffSequence(t *testing.T) {
	want := []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 5 * time.Minute, 5 * time.Minute}
	for i, w := range want {
		if got := supplyBackoff(i + 1); got != w {
			t.Errorf("failure %d: backoff %v, want %v", i+1, got, w)
		}
	}
	if got := supplyBackoff(1000); got != supplyBackoffMax {
		t.Errorf("huge streak: %v", got)
	}
}

func TestSupplyStoreDropsLevelsThatGrowTooOld(t *testing.T) {
	h := newStoreHarness()
	h.get()
	h.runPending()
	h.err = errors.New("printer offline")
	for elapsed := time.Duration(0); elapsed <= supplyMaxStale; {
		h.advance(supplyCacheTTL)
		elapsed += supplyCacheTTL
		got := h.get()
		h.runPending()
		if elapsed <= supplyMaxStale && len(got.Supplies) != 1 {
			t.Fatalf("level dropped after %v, before the limit of %v", elapsed, supplyMaxStale)
		}
	}
	h.advance(supplyCacheTTL)
	if got := h.get(); got.Supplies != nil {
		t.Fatalf("level older than %v still reported: %+v", supplyMaxStale, got)
	}
}

func TestSupplyStoreNoSourceIsNotAFailure(t *testing.T) {
	logs := captureLogs(t)
	h := newStoreHarness()
	h.err = errNoSupplySource
	h.get()
	h.runPending()
	if got := h.get(); got.Supplies != nil {
		t.Fatalf("got %+v", got)
	}
	h.advance(supplyCacheTTL)
	h.get()
	if len(h.pending) != 1 {
		t.Fatal("a printer without a source must be looked at again after the TTL, without a backoff")
	}
	if strings.Contains(logs.String(), "WARN") {
		t.Fatalf("no source logged as a warning: %s", logs.String())
	}
}

func TestSupplyStoreLogsFailuresOnceAsWarnings(t *testing.T) {
	logs := captureLogs(t)
	h := newStoreHarness()
	h.err = errors.New("192.168.1.50: request timeout")
	for range 4 {
		h.get()
		h.runPending()
		h.advance(supplyBackoffMax)
	}
	out := logs.String()
	if got := strings.Count(out, "level=WARN"); got != 1 {
		t.Fatalf("%d warnings for one outage, want 1:\n%s", got, out)
	}
	if got := strings.Count(out, "level=DEBUG"); got != 3 {
		t.Fatalf("%d debug lines for the repeats, want 3:\n%s", got, out)
	}
	if !strings.Contains(out, "component=supplies") || !strings.Contains(out, testPrinterName) {
		t.Fatalf("log lacks component or printer:\n%s", out)
	}
}

func TestSupplyStoreDisabledDoesNothing(t *testing.T) {
	h := newStoreHarness()
	h.store.SetEnabled(false)
	for range 3 {
		if got := h.get(); got.Supplies != nil || got.Host != "" {
			t.Fatalf("disabled store answered %+v", got)
		}
	}
	h.runPending()
	if h.fetches != 0 || len(h.pending) != 0 {
		t.Fatalf("disabled store read the network: fetches=%d pending=%d", h.fetches, len(h.pending))
	}
	if newSupplyStore(nil).Get(testPrinterName).Supplies != nil {
		t.Fatal("a store nobody enabled answered")
	}
}

func TestSupplyStoreDisablingCancelsAndForgets(t *testing.T) {
	h := newStoreHarness()
	h.get()
	h.runPending()
	if len(h.get().Supplies) != 1 {
		t.Fatal("no data to forget")
	}
	h.advance(supplyCacheTTL)
	h.get() // a refresh is now in flight
	inFlight := h.pending
	h.pending = nil
	h.store.SetEnabled(false)
	if h.get().Supplies != nil {
		t.Fatal("data served while off")
	}
	h.result = supplyReading{Supplies: []core.Supply{{Name: "Late", Kind: core.SupplyInk, Level: 1}}}
	for _, refresh := range inFlight {
		refresh()
	}
	if h.lastCtx.Err() == nil {
		t.Fatal("the read in flight was not cancelled when reading was turned off")
	}
	h.store.SetEnabled(true)
	if got := h.get(); got.Supplies != nil {
		t.Fatalf("a result from before the switch-off reappeared: %+v", got)
	}
}

func TestSupplyStoreBoundsEachRead(t *testing.T) {
	h := newStoreHarness()
	h.get()
	h.runPending()
	deadline, ok := h.lastCtx.Deadline()
	if !ok || time.Until(deadline) > supplyReadTimeout {
		t.Fatalf("read has no deadline within %v: %v %v", supplyReadTimeout, deadline, ok)
	}
}
