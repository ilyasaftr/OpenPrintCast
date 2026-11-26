package winspool

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// Timing of the supply cache. A Get-Printer-Attributes answer never waits
// for the network: it reads the cache, and a refresh runs in the background.
const (
	// supplyCacheTTL is how long a read stays fresh before a background
	// refresh starts; the stale value is served meanwhile.
	supplyCacheTTL = time.Minute
	// supplyMaxStale is how long a read is still reported when refreshing
	// keeps failing. Older levels are dropped rather than presented as
	// current.
	supplyMaxStale = 10 * time.Minute
	// supplyReadTimeout bounds one read of one printer, all requests
	// included. The USB device probe is a handshake of several round
	// trips — drain, mode switch, negotiation, one exchange per fact —
	// which needs seconds where SNMP needs milliseconds.
	supplyReadTimeout = 15 * time.Second
	// supplyBackoffBase is the wait after the first failed read; it doubles
	// with every further failure up to supplyBackoffMax.
	supplyBackoffBase = 30 * time.Second
	supplyBackoffMax  = 5 * time.Minute
)

// supplyReading is what one read of a printer found: its supplies, and the
// printer's own address (for its web page), which is known even when the
// printer reports no supplies. A read straight from the device also knows
// the serial, the firmware version and the device's own error states.
type supplyReading struct {
	Supplies     []core.Supply
	Host         string
	Serial       string
	Firmware     string
	StateReasons []string
	// TotalPages is the sheets-fed count the device itself keeps, known
	// only when the device reports one (PagesKnown) — zero is a real
	// count, not a stand-in for unknown.
	TotalPages int
	PagesKnown bool
}

// supplyFetcher reads one printer. It returns errNoSupplySource for a
// printer that cannot be read this way. It must stop when ctx is done.
type supplyFetcher func(ctx context.Context, printer string) (supplyReading, error)

// supplyEntry is the cache state of one printer.
type supplyEntry struct {
	reading    supplyReading
	hasReading bool
	hadReading bool      // a read has succeeded at least once
	fetchedAt  time.Time // of the last successful read
	retryAt    time.Time // no refresh before this after a failure
	failures   int       // consecutive failed reads
	refreshing bool      // a refresh is in flight (single-flight)
}

// supplyStore caches supply readings per printer with stale-while-
// revalidate: at most one background refresh per printer, failures backed
// off exponentially. It does nothing, and sends nothing, while disabled.
type supplyStore struct {
	fetch supplyFetcher
	now   func() time.Time
	// start runs a refresh; production runs it on a goroutine.
	start func(func())

	mu      sync.Mutex
	enabled bool
	// generation changes whenever the store is disabled, so a refresh that
	// was in flight then neither stores its result nor logs.
	generation uint64
	ctx        context.Context
	cancel     context.CancelFunc
	entries    map[string]*supplyEntry
}

// newSupplyStore builds a disabled store reading printers with fetch.
func newSupplyStore(fetch supplyFetcher) *supplyStore {
	return &supplyStore{
		fetch:   fetch,
		now:     time.Now,
		start:   func(refresh func()) { go refresh() },
		entries: make(map[string]*supplyEntry),
	}
}

// SetEnabled switches reading on or off. Turning it off cancels reads in
// flight and forgets what was read, so nothing stale returns on turning it
// on again.
func (s *supplyStore) SetEnabled(on bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if on == s.enabled {
		return
	}
	s.enabled = on
	if on {
		s.ctx, s.cancel = context.WithCancel(context.Background())
		return
	}
	s.cancel()
	s.generation++
	s.entries = make(map[string]*supplyEntry)
}

// Get returns the cached reading of printer without waiting for the
// network, and starts a background refresh when it is due.
func (s *supplyStore) Get(printer string) supplyReading {
	reading, refresh := s.lookup(printer)
	if refresh != nil {
		s.start(refresh)
	}
	return reading
}

// lookup answers from the cache and returns the refresh to start, if any.
func (s *supplyStore) lookup(printer string) (supplyReading, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.enabled {
		return supplyReading{}, nil
	}
	entry := s.entries[printer]
	if entry == nil {
		entry = &supplyEntry{}
		s.entries[printer] = entry
	}
	now := s.now()
	var refresh func()
	if entry.due(now) {
		entry.refreshing = true
		refresh = s.refresher(printer, s.generation, s.ctx)
	}
	if !entry.hasReading || now.Sub(entry.fetchedAt) > supplyMaxStale {
		return supplyReading{}, refresh
	}
	return entry.reading, refresh
}

// due reports whether a refresh should start now.
func (e *supplyEntry) due(now time.Time) bool {
	if e.refreshing || now.Before(e.retryAt) {
		return false
	}
	return !e.hasReading || now.Sub(e.fetchedAt) >= supplyCacheTTL
}

func (s *supplyStore) refresher(printer string, generation uint64, ctx context.Context) func() {
	return func() {
		ctx, cancel := context.WithTimeout(ctx, supplyReadTimeout)
		defer cancel()
		reading, err := s.fetch(ctx, printer)
		s.finish(printer, generation, reading, err)
	}
}

// finish records the outcome of a refresh.
func (s *supplyStore) finish(printer string, generation uint64, reading supplyReading, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.entries[printer]
	if generation != s.generation || entry == nil {
		return
	}
	entry.refreshing = false
	if err != nil && !errors.Is(err, errNoSupplySource) {
		entry.failures++
		entry.retryAt = s.now().Add(supplyBackoff(entry.failures))
		logSupplyFailure(printer, entry, err)
		return
	}
	if entry.failures > 0 {
		componentLogger(componentSupplies).Debug("supply levels readable again", "printer", printer)
	}
	if errors.Is(err, errNoSupplySource) {
		reading = supplyReading{}
		componentLogger(componentSupplies).Debug("printer cannot be read for supply levels", "printer", printer)
	} else if !entry.hadReading {
		componentLogger(componentSupplies).Info("supply levels read", "printer", printer, "supplies", len(reading.Supplies))
	}
	entry.reading, entry.hasReading = reading, true
	entry.hadReading = true
	entry.fetchedAt, entry.retryAt, entry.failures = s.now(), time.Time{}, 0
}

// supplyBackoff is the wait after the failures-th consecutive failure:
// 30 s, 1 min, 2 min, 4 min, then 5 min.
func supplyBackoff(failures int) time.Duration {
	delay := supplyBackoffBase
	for i := 1; i < failures && delay < supplyBackoffMax; i++ {
		delay *= 2
	}
	return min(delay, supplyBackoffMax)
}

// logSupplyFailure warns about the first failure of a streak and notes the
// repeats at debug level, so an unreachable printer does not fill the log.
// The error carries no community string.
func logSupplyFailure(printer string, entry *supplyEntry, err error) {
	log := componentLogger(componentSupplies)
	if entry.failures == 1 {
		log.Warn("could not read supply levels", "printer", printer, "err", err, "retry_in", supplyBackoff(1))
		return
	}
	log.Debug("could not read supply levels", "printer", printer, "err", err, "failures", entry.failures)
}

// fetchSupplies is the production supplyFetcher. A printer on a network
// port is read over SNMP through the port's own settings; a printer on a
// USB port is probed straight from the device. Nothing a print client
// sends takes part in choosing either path.
func fetchSupplies(ctx context.Context, printer string) (supplyReading, error) {
	target, err := platformSupplyTarget(printer)
	if err == nil {
		supplies, err := readSupplies(ctx, newGosnmpWalker(target), target.DeviceIndex)
		if err != nil {
			return supplyReading{}, fmt.Errorf("%s: %w", target.Host, err)
		}
		return supplyReading{Supplies: supplies, Host: target.Host}, nil
	}
	return probeUSB(ctx, printer)
}
