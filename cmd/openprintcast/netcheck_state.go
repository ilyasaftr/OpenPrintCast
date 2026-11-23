package main

import (
	"time"

	"github.com/ilyasaftr/OpenPrintCast/internal/reach"
)

// Timing of the reachability check. The first check follows the server's
// report of listening (or failing) by netCheckStartDelay, so the network
// and the firewall have settled; a window shown again after
// netCheckStaleAfter checks anew; while a problem is on screen the check
// repeats every netCheckPollEvery, so the banner goes away by itself once
// the user has fixed it. A check the user asked for stays busy for at
// least netCheckMinBusy, long enough to be seen.
const (
	netCheckStartDelay = 3 * time.Second
	netCheckStaleAfter = 5 * time.Minute
	netCheckPollEvery  = 60 * time.Second
	netCheckMinBusy    = 700 * time.Millisecond
)

// checkPhase is where the check stands: nothing running, running (or its
// answer held back for the minimum busy time) and showing an answer.
type checkPhase int

const (
	checkIdle checkPhase = iota
	checkChecking
	checkResult
)

// tickOutcome tells the caller what a tick did.
type tickOutcome struct {
	// Start means a check began and the caller must run it.
	Start bool
	// Revealed means a new answer is now on show.
	Revealed bool
}

// netCheckState is the check's state machine. Like scanState it owns no
// goroutine and reads no clock: the UI thread passes the time, so tests
// need no sleeping. Triggers that arrive from other places (the server
// reporting, the window being shown) only set a flag, which the next Tick
// turns into a time.
type netCheckState struct {
	phase   checkPhase
	visible bool

	serverEvent bool
	shown       bool
	dueAt       time.Time

	manual    bool
	startedAt time.Time
	finished  bool
	outcome   reach.Assessment

	hasResult bool
	result    reach.Assessment
	checkedAt time.Time
}

// newNetCheckState builds an idle state for a visible window.
func newNetCheckState() *netCheckState { return &netCheckState{visible: true} }

// Busy reports whether a check is in progress.
func (s *netCheckState) Busy() bool { return s.phase == checkChecking }

// HasResult reports whether any check has answered yet.
func (s *netCheckState) HasResult() bool { return s.hasResult }

// Result is the latest answer; it is the zero (unknown) Assessment before
// the first one.
func (s *netCheckState) Result() reach.Assessment { return s.result }

// Problem reports whether the latest answer is one the user must act on.
func (s *netCheckState) Problem() bool { return s.hasResult && s.result.Problem() }

// NoteServerEvent records that the IPP server started or failed to listen.
func (s *netCheckState) NoteServerEvent() { s.serverEvent = true }

// SetVisible records whether the window is shown. Showing it again after a
// long while asks for a fresh check.
func (s *netCheckState) SetVisible(visible bool) {
	if visible && !s.visible {
		s.shown = true
	}
	s.visible = visible
}

// Request starts a check at now for the Scan Again and Check Again
// buttons. manual keeps the busy state up for the minimum time. It reports
// false, changing nothing, while a check is already running.
func (s *netCheckState) Request(now time.Time, manual bool) bool {
	if s.phase == checkChecking {
		return false
	}
	s.phase, s.manual, s.finished = checkChecking, manual, false
	s.startedAt, s.dueAt = now, time.Time{}
	return true
}

// Finish records the answer of the running check; it is ignored when none
// is running.
func (s *netCheckState) Finish(answer reach.Assessment) {
	if s.phase == checkChecking && !s.finished {
		s.finished, s.outcome = true, answer
	}
}

// Tick moves the state on to now: it takes up the pending triggers, shows
// a finished answer once the minimum busy time has passed, and starts a
// check that is due. Nothing starts while the window is hidden.
func (s *netCheckState) Tick(now time.Time) tickOutcome {
	var out tickOutcome
	s.takeEvents(now)
	if s.phase == checkChecking && s.finished && (!s.manual || now.Sub(s.startedAt) >= netCheckMinBusy) {
		s.phase, s.hasResult, s.result, s.checkedAt = checkResult, true, s.outcome, now
		out.Revealed = true
	}
	if s.dueNow(now) && s.Request(now, false) {
		out.Start = true
	}
	return out
}

// takeEvents turns the flags raised from outside into due times.
func (s *netCheckState) takeEvents(now time.Time) {
	if s.serverEvent {
		s.serverEvent = false
		s.dueAt = now.Add(netCheckStartDelay)
	}
	if s.shown {
		s.shown = false
		if !s.hasResult || now.Sub(s.checkedAt) >= netCheckStaleAfter {
			s.dueAt = now
		}
	}
}

// dueNow reports whether a check should start: the window is shown, none
// is running, and either a trigger came due or a problem has been on show
// for the polling interval.
func (s *netCheckState) dueNow(now time.Time) bool {
	if !s.visible || s.phase == checkChecking {
		return false
	}
	if !s.dueAt.IsZero() && !now.Before(s.dueAt) {
		return true
	}
	return s.Problem() && !now.Before(s.checkedAt.Add(netCheckPollEvery))
}

// NextWake is how long from now until Tick has something to do, or 0 when
// only an outside event can change the state.
func (s *netCheckState) NextWake(now time.Time) time.Duration {
	var at time.Time
	switch {
	case s.phase == checkChecking && s.finished && s.manual:
		at = s.startedAt.Add(netCheckMinBusy)
	case s.phase == checkChecking || !s.visible:
		return 0
	case !s.dueAt.IsZero():
		at = s.dueAt
	case s.Problem():
		at = s.checkedAt.Add(netCheckPollEvery)
	}
	if wait := at.Sub(now); !at.IsZero() && wait > 0 {
		return wait
	}
	return 0
}
