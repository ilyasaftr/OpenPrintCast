package main

import (
	"math/rand/v2"
	"time"

	"github.com/egoist/mygo"
)

// Schedule of the background check. The first one comes shortly after the
// app starts, then one every updateInterval plus up to updateJitterPercent
// of it, so a fleet of PCs does not call the feed in the same minute. No
// two checks, across restarts too, are closer than updateMinInterval, and
// a failed check is tried again after updateMinInterval, not a day later.
const (
	updateFirstDelay    = 10 * time.Second
	updateInterval      = 24 * time.Hour
	updateMinInterval   = 6 * time.Hour
	updateJitterPercent = 10
)

// afterFunc runs f once after d, on the UI thread, and returns the function
// that cancels it. A cancel that comes too late may still let f run, so
// callers must tolerate that.
type afterFunc func(d time.Duration, f func()) (cancel func())

// mainThreadAfter is the production afterFunc: a timer whose callback
// hops to the main thread, where the scheduler's state lives.
func mainThreadAfter(d time.Duration, f func()) func() {
	timer := time.AfterFunc(d, func() { mygo.RunOnMain(f) })
	return func() { timer.Stop() }
}

// randomJitter is the production jitter: a random wait below limit.
func randomJitter(limit time.Duration) time.Duration {
	if limit <= 0 {
		return 0
	}
	return rand.N(limit)
}

// updateScheduler decides when the next background check runs. It owns no
// goroutine: it asks afterFunc for one timer at a time, so tests drive it
// with a fake clock and a fake timer. Everything runs on the UI thread.
type updateScheduler struct {
	now      func() time.Time
	after    afterFunc
	jitter   func(limit time.Duration) time.Duration
	lastDone func() time.Time // when the last check finished; zero if never
	check    func()           // starts a background check

	enabled bool
	started bool
	cancel  func()
	epoch   int // tells a stale timer callback from the current one
}

// Start arms the first check, unless background checks are off.
func (s *updateScheduler) Start() {
	s.started = true
	s.armFirst()
}

// Stop cancels the timer for good, as the app quits.
func (s *updateScheduler) Stop() {
	s.started = false
	s.disarm()
}

// SetEnabled turns background checks on or off while the app runs.
func (s *updateScheduler) SetEnabled(enabled bool) {
	if s.enabled == enabled {
		return
	}
	s.enabled = enabled
	if enabled {
		s.armFirst()
	} else {
		s.disarm()
	}
}

// Completed tells the scheduler a check finished and arms the next one. A
// manual check counts, and restarts the day. retryAfter is a wait the feed
// asked for after a failure.
func (s *updateScheduler) Completed(failed bool, retryAfter time.Duration) {
	delay := updateInterval
	if failed {
		delay = min(max(updateMinInterval, retryAfter), updateInterval)
	}
	s.arm(delay + s.jitter(delay/100*updateJitterPercent))
}

// armFirst arms the check after start or after switching checks on: soon,
// but never before updateMinInterval has passed since the last check. A
// last check dated in the future (the clock was set back) counts as now.
func (s *updateScheduler) armFirst() {
	delay := updateFirstDelay
	if last := s.lastDone(); !last.IsZero() {
		since := max(s.now().Sub(last), 0)
		delay = max(delay, updateMinInterval-since)
	}
	s.arm(delay)
}

// arm replaces the timer with one that fires after delay.
func (s *updateScheduler) arm(delay time.Duration) {
	s.disarm()
	if !s.started || !s.enabled {
		return
	}
	epoch := s.epoch
	s.cancel = s.after(delay, func() { s.fire(epoch) })
}

// disarm cancels the pending timer, and invalidates a callback that has
// already left the timer.
func (s *updateScheduler) disarm() {
	s.epoch++
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

// fire runs when a timer ends; a stale one does nothing. The check it
// starts reports back through Completed, which arms the next timer.
func (s *updateScheduler) fire(epoch int) {
	if epoch != s.epoch || !s.started || !s.enabled {
		return
	}
	s.cancel = nil
	s.check()
}
