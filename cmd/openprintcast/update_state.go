package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/ilyasaftr/OpenPrintCast/internal/updatecheck"
)

// Update feedback timing. A manual check that ends in a few milliseconds
// would flash by unseen, so its busy state lasts at least updateMinBusy; a
// check in the background has no busy state at all. The "Last checked"
// sentence reads in minutes, so the page redraws once a minute.
const (
	updateMinBusy         = 700 * time.Millisecond
	updateRelativeRefresh = time.Minute
)

// Update status wording.
const (
	updateIdleText        = "Not checked yet."
	updateCheckingText    = "Checking for updates…"
	updateUpToDateText    = "You're up to date."
	updateAvailableFormat = "Version %s is available."
	updateSkippedFormat   = "Version %s is available (skipped)."
	updateOfflineText     = "Couldn't check for updates. Check your internet connection."
	updateLimitedText     = "GitHub is limiting requests. Try again later."
	updateNotFoundText    = "The update feed wasn't found."
	updateFailedText      = "Couldn't check for updates. Try again later."
	updateNotSetUpText    = "Update checking isn't set up in this build."
	updateDevBuildText    = "This is a development build."
)

// updatePhase is what the Software update row says about updates.
type updatePhase int

const (
	// updateIdle: no check has finished in this run.
	updateIdle updatePhase = iota
	updateChecking
	updateUpToDate
	updateAvailable
	updateFailed
	// updateNotSetUp: the build has no (valid) feed.
	updateNotSetUp
	// updateDevBuild: the version is not a release, so there is nothing to
	// compare with.
	updateDevBuild
)

// updateResult is the outcome of one finished check: the newest version
// worth offering if there is one, or the error that stopped the check, and
// when it finished.
type updateResult struct {
	release updatecheck.Release
	found   bool
	err     error
	at      time.Time
}

// updateFailureText is the sentence for a check that failed.
func updateFailureText(err error) string {
	switch {
	case errors.Is(err, updatecheck.ErrRateLimited):
		return updateLimitedText
	case errors.Is(err, updatecheck.ErrFeedNotFound):
		return updateNotFoundText
	case errors.Is(err, updatecheck.ErrNetwork):
		return updateOfflineText
	}
	return updateFailedText
}

// availableText is the sentence for a version on offer.
func availableText(tag string, skipped bool) string {
	if skipped {
		return fmt.Sprintf(updateSkippedFormat, tag)
	}
	return fmt.Sprintf(updateAvailableFormat, tag)
}

// updateStatus is what the page shows about updates right now.
type updateStatus struct {
	Phase     updatePhase
	Release   updatecheck.Release // the offered version, when Phase is updateAvailable
	CheckedAt time.Time           // when the last check finished; zero if none did
	Failure   string              // the sentence, when Phase is updateFailed
	Skipped   bool                // the user skipped Release
}

// Busy reports whether a manual check is shown as running.
func (s updateStatus) Busy() bool { return s.Phase == updateChecking }

// Checkable reports whether Check Now can start a check.
func (s updateStatus) Checkable() bool {
	return s.Phase != updateNotSetUp && s.Phase != updateDevBuild && s.Phase != updateChecking
}

// Unavailable is the reason Check Now cannot work at all in this build, or
// "" when it can.
func (s updateStatus) Unavailable() string {
	switch s.Phase {
	case updateNotSetUp:
		return updateNotSetUpText
	case updateDevBuild:
		return updateDevBuildText
	}
	return ""
}

// HasUpdate reports whether a newer version is on offer.
func (s updateStatus) HasUpdate() bool { return s.Phase == updateAvailable }

// Notable reports whether the update deserves attention: on offer and not
// skipped.
func (s updateStatus) Notable() bool { return s.HasUpdate() && !s.Skipped }

// CanDownload reports whether there is a safe page to open for the update.
func (s updateStatus) CanDownload() bool { return s.HasUpdate() && s.Release.URL != "" }

// Text is the status sentence.
func (s updateStatus) Text() string {
	switch s.Phase {
	case updateChecking:
		return updateCheckingText
	case updateUpToDate:
		return updateUpToDateText
	case updateAvailable:
		return availableText(s.Release.Tag, s.Skipped)
	case updateFailed:
		return s.Failure
	case updateNotSetUp:
		return updateNotSetUpText
	case updateDevBuild:
		return updateDevBuildText
	}
	return updateIdleText
}

// updateState is the update check's state machine. A manual check shows
// as busy, and its result is held back until the busy state has lasted
// long enough to be seen; a background check shows nothing while it runs,
// leaves the page alone when it fails and only updates the result when it
// succeeds. It owns no goroutine and reads no clock (the caller passes the
// time), so the UI thread alone drives it and tests need no sleeping.
type updateState struct {
	minBusy time.Duration

	phase     updatePhase
	release   updatecheck.Release
	checkedAt time.Time
	failure   string

	running   bool
	manual    bool
	startedAt time.Time
	finished  bool
	result    updateResult

	failures     int
	announcement string
}

// newUpdateState builds a state in phase, which is updateIdle for a build
// that can check, else the reason it cannot. lastChecked is when the last
// check finished, or zero.
func newUpdateState(phase updatePhase, lastChecked time.Time) *updateState {
	return &updateState{minBusy: updateMinBusy, phase: phase, checkedAt: lastChecked}
}

// Begin starts a check at now and reports whether the caller must fetch.
// It reports false, and starts nothing, in a build that cannot check and
// while a check is running; a manual request during a background check
// takes that check over, so it shows as busy without a second fetch.
func (s *updateState) Begin(now time.Time, manual bool) bool {
	switch {
	case s.phase == updateNotSetUp || s.phase == updateDevBuild:
		return false
	case s.running:
		if manual && !s.manual {
			s.manual, s.startedAt, s.announcement = true, now, updateCheckingText
		}
		return false
	}
	s.running, s.finished, s.manual, s.startedAt = true, false, manual, now
	if manual {
		s.announcement = updateCheckingText
	}
	return true
}

// Finish records the outcome of the running check. It is ignored when no
// check is running. It reports whether the failure is the first of a run
// of failures, which is the one worth a log line. A background check takes
// effect at once; a manual one waits for Advance.
func (s *updateState) Finish(res updateResult) (firstFailure bool) {
	if !s.running || s.finished {
		return false
	}
	s.finished, s.result = true, res
	firstFailure = s.noteFailure(res.err)
	if !s.manual {
		s.settle(res.at)
	}
	return firstFailure
}

// noteFailure counts consecutive failures and reports the first.
func (s *updateState) noteFailure(err error) bool {
	if err == nil {
		s.failures = 0
		return false
	}
	s.failures++
	return s.failures == 1
}

// Advance moves the state on to now: it reveals a finished manual check
// once the minimum busy time has passed.
func (s *updateState) Advance(now time.Time) {
	if s.running && s.finished && s.manual && now.Sub(s.startedAt) >= s.minBusy {
		s.settle(now)
	}
}

// settle makes the finished check's result the state. A failed background
// check changes nothing the user sees.
func (s *updateState) settle(now time.Time) {
	res := s.result
	s.running = false
	switch {
	case res.err != nil && !s.manual:
		return
	case res.err != nil:
		s.phase, s.failure = updateFailed, updateFailureText(res.err)
	default:
		s.checkedAt = res.at
		s.setOutcome(res.release, res.found)
	}
	if s.manual {
		s.announcement = s.Status().Text()
	}
}

// setOutcome shows the newest version on offer, or that there is none.
func (s *updateState) setOutcome(release updatecheck.Release, found bool) {
	if found {
		s.phase, s.release = updateAvailable, release
		return
	}
	s.phase, s.release = updateUpToDate, updatecheck.Release{}
}

// Reclassify replaces a settled success with a new reading of the same
// feed answer (the beta switch changed). It leaves a running check, a
// failure and an unchecked state alone.
func (s *updateState) Reclassify(release updatecheck.Release, found bool) {
	if !s.running && (s.phase == updateUpToDate || s.phase == updateAvailable) {
		s.setOutcome(release, found)
	}
}

// Status is what to show; call Advance first.
func (s *updateState) Status() updateStatus {
	status := updateStatus{Phase: s.phase, Release: s.release, CheckedAt: s.checkedAt, Failure: s.failure}
	if s.running && s.manual {
		status.Phase = updateChecking
	}
	return status
}

// NextWake is how long from now until the state has something to do: the
// end of the minimum busy time, or the next redraw of the relative time. It
// is 0 when nothing is due.
func (s *updateState) NextWake(now time.Time) time.Duration {
	switch {
	case s.running && s.finished && s.manual:
		return s.startedAt.Add(s.minBusy).Sub(now)
	case !s.running && !s.checkedAt.IsZero():
		return updateRelativeRefresh
	}
	return 0
}

// TakeAnnouncement returns the sentence screen readers should hear about a
// change of state, once; it is empty when nothing new happened.
func (s *updateState) TakeAnnouncement() string {
	text := s.announcement
	s.announcement = ""
	return text
}
