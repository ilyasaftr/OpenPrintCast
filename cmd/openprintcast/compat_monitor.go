package main

import (
	"context"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
	bridgenet "github.com/ilyasaftr/OpenPrintCast/internal/net"
)

// compatibilityInterval is how often shared printers are checked again:
// drivers rarely change what they report, so the check is not frequent.
const compatibilityInterval = 5 * time.Minute

// compatibilitySource answers whether a printer's driver reports what
// IPP/2.0 needs. gaps are the stable keys of the missing items; known is
// false when the answer could not be determined.
type compatibilitySource interface {
	IPP20Gaps(printer core.PrinterInfo) (gaps []string, known bool)
}

// sharedCompatibility is the production compatibilitySource: the same
// check the IPP server makes before it serves a printer as IPP/2.0.
type sharedCompatibility struct{ shared *bridgenet.SharedState }

// IPP20Gaps asks the shared state's capability providers.
func (s sharedCompatibility) IPP20Gaps(printer core.PrinterInfo) ([]string, bool) {
	return bridgenet.IPP20Gaps(s.shared, printer)
}

// compatibilityResult is the outcome of one check and when it was made.
type compatibilityResult struct {
	gaps  []string
	known bool
	at    time.Time
}

// Problem reports whether the check found something missing.
func (r compatibilityResult) Problem() bool { return r.known && len(r.gaps) > 0 }

// sameVerdict reports whether two results would read the same to the user.
func (r compatibilityResult) sameVerdict(o compatibilityResult) bool {
	return r.known == o.known && slices.Equal(r.gaps, o.gaps)
}

// printerSnapshot is a goroutine-safe copy of the printer list. The UI
// thread owns the list and publishes copies here for the monitor.
type printerSnapshot struct {
	mu       sync.Mutex
	printers []core.PrinterInfo
}

// Set replaces the snapshot with a copy of printers.
func (s *printerSnapshot) Set(printers []core.PrinterInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.printers = slices.Clone(printers)
}

// Get returns a copy of the current printers.
func (s *printerSnapshot) Get() []core.PrinterInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.printers)
}

// compatibilityMonitor checks shared printers off the UI thread and caches
// the results by printer ID, so the view only ever reads the cache.
type compatibilityMonitor struct {
	src      compatibilitySource
	now      func() time.Time
	onChange func()
	// wake asks Run to check now instead of at the next tick; it holds at
	// most one pending request.
	wake chan struct{}

	mu    sync.Mutex
	cache map[string]compatibilityResult
}

// newCompatibilityMonitor builds a monitor over src, stamping results with
// now (nil: the system clock). onChange (may be nil) runs after a check that
// changed what the user would see.
func newCompatibilityMonitor(src compatibilitySource, now func() time.Time, onChange func()) *compatibilityMonitor {
	if now == nil {
		now = time.Now
	}
	return &compatibilityMonitor{
		src: src, now: now, onChange: onChange,
		wake: make(chan struct{}, 1), cache: map[string]compatibilityResult{},
	}
}

// Result returns the cached check of the printer with id; ok is false until
// it has been checked. A nil monitor knows nothing.
func (m *compatibilityMonitor) Result(id string) (compatibilityResult, bool) {
	if m == nil {
		return compatibilityResult{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.cache[id]
	return r, ok
}

// Nudge asks a running monitor to check right away, as after a scan or an
// edit to the printer list. It never blocks, requests made while one is
// pending merge, and a nil monitor ignores it.
func (m *compatibilityMonitor) Nudge() {
	if m == nil {
		return
	}
	select {
	case m.wake <- struct{}{}:
	default:
	}
}

// check asks the source about one printer. A fault in the driver query
// counts as "could not determine" instead of taking the app down.
func (m *compatibilityMonitor) check(printer core.PrinterInfo) (result compatibilityResult) {
	defer func() {
		if r := recover(); r != nil {
			result = compatibilityResult{at: m.now()}
		}
	}()
	gaps, known := m.src.IPP20Gaps(printer)
	return compatibilityResult{gaps: slices.Clone(gaps), known: known, at: m.now()}
}

// refresh checks the shared printers without holding the lock, then
// replaces the cache entries, drops printers no longer listed and logs
// each verdict that changed. It reports whether any verdict changed.
func (m *compatibilityMonitor) refresh(printers []core.PrinterInfo) bool {
	fresh := make(map[string]compatibilityResult)
	for _, p := range printers {
		if p.Enabled && !p.IsFolderDestination() {
			fresh[p.ID] = m.check(p)
		}
	}
	m.mu.Lock()
	changes := m.store(printers, fresh)
	m.mu.Unlock()
	for _, c := range changes {
		logVerdict(c.id, c.before, c.after)
	}
	return len(changes) > 0
}

// verdictChange is one printer whose verdict differs from the cached one.
type verdictChange struct {
	id            string
	before, after compatibilityResult
}

// store merges fresh results into the cache and returns the verdict
// changes. A result that could not be determined never replaces a known
// one. The caller holds the lock.
func (m *compatibilityMonitor) store(printers []core.PrinterInfo, fresh map[string]compatibilityResult) []verdictChange {
	next := make(map[string]compatibilityResult, len(printers))
	var changes []verdictChange
	for _, p := range printers {
		old, had := m.cache[p.ID]
		r, checked := fresh[p.ID]
		switch {
		case !checked && had:
			next[p.ID] = old
		case checked && !r.known && old.known:
			next[p.ID] = old
		case checked:
			next[p.ID] = r
			if !had || !old.sameVerdict(r) {
				changes = append(changes, verdictChange{p.ID, old, r})
			}
		}
	}
	m.cache = next
	return changes
}

// logVerdict logs a change of verdict once: Warn when a printer cannot be
// served as IPP/2.0, Info when it can again. Other changes are not news.
func logVerdict(id string, before, after compatibilityResult) {
	log := componentLogger(componentApp)
	switch {
	case after.Problem():
		log.Warn("printer cannot be served as IPP/2.0", "printer", id, "gaps", strings.Join(after.gaps, ","))
	case before.Problem() && after.known:
		log.Info("printer can be served as IPP/2.0 again", "printer", id)
	}
}

// Run checks immediately and then every interval, or sooner when nudged,
// until ctx is cancelled, calling onChange only after a check that changed
// something.
func (m *compatibilityMonitor) Run(ctx context.Context, printers func() []core.PrinterInfo, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if m.refresh(printers()) && m.onChange != nil {
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
