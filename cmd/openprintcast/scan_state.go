package main

import (
	"fmt"
	"time"
)

// Scan feedback timing. A scan that ends in a few milliseconds would flash
// by unseen, so the busy state lasts at least scanMinBusy; a result that
// carries good news fades from the page after scanResultVisible, while a
// failure stays until the next scan.
const (
	scanMinBusy       = 700 * time.Millisecond
	scanResultVisible = 10 * time.Second
)

// Scan wording.
const (
	scanBusyText     = "Scanning for printers…"
	scanFailedText   = "Couldn't read the printer list"
	scanNoChangeText = "No new printers found"
	scanNoneText     = "No printers found"
)

// scanOutcome is what one scan found, counted in printers the Printers page
// lists: how many appeared, how many are gone and how many there are now.
// failed means the printer list could not be read and nothing changed.
type scanOutcome struct {
	added, removed, total int
	failed                bool
}

// message is the result sentence of the outcome.
func (o scanOutcome) message() string {
	switch {
	case o.failed:
		return scanFailedText
	case o.added > 0 && o.removed > 0:
		return fmt.Sprintf("%s added, %d removed", plural(o.added, "new printer"), o.removed)
	case o.added > 0:
		return plural(o.added, "new printer") + " added"
	case o.removed > 0:
		return plural(o.removed, "printer") + " removed"
	case o.total == 0:
		return scanNoneText
	}
	return scanNoChangeText
}

// scanStatus is what the Printers page shows about the scan right now.
// An empty Text means there is nothing to say.
type scanStatus struct {
	Busy   bool
	Failed bool
	Text   string
}

// scanState is the scan's state machine: idle, busy (a scan is running or
// its result is being held back until the busy state has lasted long
// enough to be seen) and showing a result. It owns no goroutine and reads
// no clock, the caller passes the time, so it is driven by the UI thread
// alone and tests need no sleeping.
type scanState struct {
	minBusy, resultVisible time.Duration

	busy      bool
	startedAt time.Time
	finished  bool
	outcome   scanOutcome

	hasResult bool
	result    scanOutcome
	shownAt   time.Time

	announcement string
}

// newScanState builds an idle scan state with the standard timing.
func newScanState() *scanState {
	return &scanState{minBusy: scanMinBusy, resultVisible: scanResultVisible}
}

// Busy reports whether a scan is in progress.
func (s *scanState) Busy() bool { return s.busy }

// Begin starts a scan at now. It reports false, and changes nothing, while
// a scan is already in progress, so repeated clicks do not stack scans.
func (s *scanState) Begin(now time.Time) bool {
	if s.busy {
		return false
	}
	s.busy, s.finished, s.hasResult = true, false, false
	s.startedAt = now
	s.announcement = scanBusyText
	return true
}

// Finish records the outcome of the running scan. It is ignored when no
// scan is running. The result stays hidden until Advance finds the busy
// state has lasted long enough.
func (s *scanState) Finish(outcome scanOutcome) {
	if s.busy && !s.finished {
		s.finished, s.outcome = true, outcome
	}
}

// Advance moves the state on to now: it reveals a finished scan's result
// once the minimum busy time has passed and drops an old result.
func (s *scanState) Advance(now time.Time) {
	if s.busy && s.finished && now.Sub(s.startedAt) >= s.minBusy {
		s.busy = false
		s.hasResult, s.result, s.shownAt = true, s.outcome, now
		s.announcement = s.result.message()
	}
	if s.hasResult && !s.result.failed && now.Sub(s.shownAt) >= s.resultVisible {
		s.hasResult = false
	}
}

// Status is what to show at now; call Advance first.
func (s *scanState) Status() scanStatus {
	switch {
	case s.busy:
		return scanStatus{Busy: true, Text: scanBusyText}
	case s.hasResult:
		return scanStatus{Failed: s.result.failed, Text: s.result.message()}
	}
	return scanStatus{}
}

// NextWake is how long from now until Advance has something to do, or 0
// when only an outside event (the scan finishing) can change the state.
func (s *scanState) NextWake(now time.Time) time.Duration {
	switch {
	case s.busy && s.finished:
		return s.startedAt.Add(s.minBusy).Sub(now)
	case s.hasResult && !s.result.failed:
		return s.shownAt.Add(s.resultVisible).Sub(now)
	}
	return 0
}

// TakeAnnouncement returns the sentence screen readers should hear about a
// change of state, once; it is empty when nothing new happened.
func (s *scanState) TakeAnnouncement() string {
	text := s.announcement
	s.announcement = ""
	return text
}
