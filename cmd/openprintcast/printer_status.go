package main

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/ilyasaftr/OpenPrintCast/internal/winspool"
)

// statusPollInterval is how often the spooler is asked about each printer.
const statusPollInterval = 5 * time.Second

// printerCondition is what the spooler last reported about one printer:
// its coarse state and, when the backend offers them, IPP reason keywords.
type printerCondition struct {
	status  winspool.PrinterStatus
	reasons []string
}

// equal reports whether two conditions would look the same on screen.
func (c printerCondition) equal(o printerCondition) bool {
	return c.status == o.status && slices.Equal(c.reasons, o.reasons)
}

// statusSource is the part of a spooler backend the monitor needs.
type statusSource interface {
	QueryStatus(printer string) winspool.PrinterStatus
}

// reasonSource is implemented by backends that can explain a state with
// IPP reason keywords; other backends simply yield none.
type reasonSource interface {
	IPPStatus(printer string) (state int, reasons []string, ok bool)
}

// nameSnapshot is a goroutine-safe copy of the printer names to poll. The
// UI thread owns the printer list and publishes its names here, so the
// polling goroutine never reads the UI's slice.
type nameSnapshot struct {
	mu    sync.Mutex
	names []string
}

// Set replaces the snapshot with a copy of names.
func (s *nameSnapshot) Set(names []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.names = slices.Clone(names)
}

// Get returns a copy of the current names.
func (s *nameSnapshot) Get() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.names)
}

// statusMonitor polls printer conditions off the UI thread and caches them
// by Windows printer name, so the view only ever reads the cache.
type statusMonitor struct {
	src      statusSource
	reasons  reasonSource
	onChange func()
	// wake asks Run to poll now instead of at the next tick; it holds at
	// most one pending request.
	wake chan struct{}

	mu    sync.Mutex
	cache map[string]printerCondition
}

// newStatusMonitor builds a monitor over src. onChange (may be nil) runs
// after a poll that changed something; the reason keywords are used when
// src also implements reasonSource.
func newStatusMonitor(src statusSource, onChange func()) *statusMonitor {
	m := &statusMonitor{src: src, onChange: onChange, wake: make(chan struct{}, 1), cache: map[string]printerCondition{}}
	m.reasons, _ = src.(reasonSource)
	return m
}

// Condition returns the cached condition of the named printer; ok is false
// until it has been polled. A nil monitor knows nothing.
func (m *statusMonitor) Condition(name string) (printerCondition, bool) {
	if m == nil {
		return printerCondition{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.cache[name]
	return c, ok
}

// query asks the backend for one printer's condition.
func (m *statusMonitor) query(name string) printerCondition {
	c := printerCondition{status: m.src.QueryStatus(name)}
	if m.reasons != nil {
		if _, reasons, ok := m.reasons.IPPStatus(name); ok {
			c.reasons = reasons
		}
	}
	return c
}

// refresh polls the named printers and replaces the cache with the result,
// dropping printers no longer listed. It reports whether any listed
// printer's condition is new or differs from the cached one.
func (m *statusMonitor) refresh(names []string) bool {
	next := make(map[string]printerCondition, len(names))
	for _, name := range names {
		next[name] = m.query(name)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	changed := false
	for name, c := range next {
		if old, ok := m.cache[name]; !ok || !old.equal(c) {
			changed = true
		}
	}
	m.cache = next
	return changed
}

// Nudge asks a running monitor to poll right away, as after the printer
// list changed. It never blocks, requests made while one is pending merge,
// and a nil monitor ignores it.
func (m *statusMonitor) Nudge() {
	if m == nil {
		return
	}
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

// Run polls immediately and then every interval, or sooner when nudged,
// until ctx is cancelled, calling onChange only after a poll that changed
// something.
func (m *statusMonitor) Run(ctx context.Context, names func() []string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if m.refresh(names()) && m.onChange != nil {
			m.onChange()
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-m.wake:
		}
	}
}
