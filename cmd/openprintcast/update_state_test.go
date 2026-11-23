package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ilyasaftr/OpenPrintCast/internal/updatecheck"
)

var (
	updateT0   = time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	betaOffer  = updatecheck.Release{Tag: "v0.2.9-beta.15", Prerelease: true, URL: "https://github.com/o/r/releases/tag/v0.2.9-beta.15"}
	errOffline = fmt.Errorf("%w: dial tcp: no route", updatecheck.ErrNetwork)
)

func newIdleUpdateState() *updateState { return newUpdateState(updateIdle, time.Time{}) }

// finishAt finishes the running check with a result at when.
func finishAt(s *updateState, when time.Time, release updatecheck.Release, found bool, err error) bool {
	return s.Finish(updateResult{release: release, found: found, err: err, at: when})
}

func TestUpdateStateStartsIdleOrExplainsWhyItCannotCheck(t *testing.T) {
	for phase, text := range map[updatePhase]string{
		updateIdle: updateIdleText, updateNotSetUp: updateNotSetUpText, updateDevBuild: updateDevBuildText,
	} {
		s := newUpdateState(phase, time.Time{})
		if got := s.Status().Text(); got != text {
			t.Errorf("phase %d: text = %q, want %q", phase, got, text)
		}
		if began := s.Begin(updateT0, true); began != (phase == updateIdle) {
			t.Errorf("phase %d: Begin = %v", phase, began)
		}
	}
	if got := newUpdateState(updateNotSetUp, time.Time{}).Status().Unavailable(); got != updateNotSetUpText {
		t.Errorf("Unavailable = %q", got)
	}
}

func TestManualCheckIsBusyAtOnceAndRefusesASecond(t *testing.T) {
	s := newIdleUpdateState()
	if !s.Begin(updateT0, true) {
		t.Fatal("the first check did not begin")
	}
	if st := s.Status(); !st.Busy() || st.Text() != updateCheckingText || st.Checkable() {
		t.Fatalf("status right after the click = %+v", st)
	}
	if s.Begin(updateT0.Add(time.Millisecond), true) || s.Begin(updateT0.Add(time.Millisecond), false) {
		t.Fatal("a second check began while one was running")
	}
	if got := s.TakeAnnouncement(); got != updateCheckingText || s.TakeAnnouncement() != "" {
		t.Fatalf("announcement = %q, then %q", got, s.TakeAnnouncement())
	}
}

func TestManualResultWaitsForTheMinimumBusyTime(t *testing.T) {
	s := newIdleUpdateState()
	s.Begin(updateT0, true)
	s.TakeAnnouncement()
	finishAt(s, updateT0, updatecheck.Release{}, false, nil) // the feed answers in no time
	s.Advance(updateT0.Add(updateMinBusy - time.Millisecond))
	if !s.Status().Busy() {
		t.Fatal("the busy state ended before the minimum time")
	}
	if wake := s.NextWake(updateT0.Add(100 * time.Millisecond)); wake != updateMinBusy-100*time.Millisecond {
		t.Fatalf("NextWake = %v", wake)
	}
	s.Advance(updateT0.Add(updateMinBusy))
	if st := s.Status(); st.Phase != updateUpToDate || st.Text() != updateUpToDateText || st.CheckedAt != updateT0 {
		t.Fatalf("status after the minimum time = %+v", st)
	}
	if got := s.TakeAnnouncement(); got != updateUpToDateText {
		t.Fatalf("announcement = %q", got)
	}
}

func TestSlowManualResultShowsAtTheNextAdvance(t *testing.T) {
	s := newIdleUpdateState()
	s.Begin(updateT0, true)
	later := updateT0.Add(3 * time.Second)
	finishAt(s, later, betaOffer, true, nil)
	if !s.Status().Busy() {
		t.Fatal("the result showed before Advance")
	}
	s.Advance(later)
	if st := s.Status(); !st.HasUpdate() || st.Release.Tag != betaOffer.Tag || st.Text() != "Version v0.2.9-beta.15 is available." {
		t.Fatalf("status = %+v", st)
	}
	if got := s.TakeAnnouncement(); got != "Version v0.2.9-beta.15 is available." {
		t.Fatalf("announcement = %q", got)
	}
}

func TestManualFailureShowsItsSentence(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{errOffline, "Couldn't check for updates. Check your internet connection."},
		{&updatecheck.RateLimitError{}, "GitHub is limiting requests. Try again later."},
		{updatecheck.ErrFeedNotFound, "The update feed wasn't found."},
		{updatecheck.ErrMalformedFeed, "Couldn't check for updates. Try again later."},
		{&updatecheck.StatusError{Code: 500}, "Couldn't check for updates. Try again later."},
	} {
		s := newIdleUpdateState()
		s.Begin(updateT0, true)
		finishAt(s, updateT0, updatecheck.Release{}, false, tc.err)
		s.Advance(updateT0.Add(updateMinBusy))
		st := s.Status()
		if st.Phase != updateFailed || st.Text() != tc.want {
			t.Errorf("%v: status = %+v, want %q", tc.err, st, tc.want)
		}
		if !st.Checkable() {
			t.Errorf("%v: the user cannot try again", tc.err)
		}
	}
}

func TestBackgroundCheckIsInvisibleAndNeverShowsAnError(t *testing.T) {
	s := newUpdateState(updateIdle, updateT0.Add(-time.Hour))
	if !s.Begin(updateT0, false) {
		t.Fatal("background check refused")
	}
	if st := s.Status(); st.Busy() || st.Phase != updateIdle || s.TakeAnnouncement() != "" {
		t.Fatalf("a background check showed itself: %+v", st)
	}
	finishAt(s, updateT0, updatecheck.Release{}, false, errOffline)
	if st := s.Status(); st.Phase != updateIdle || st.Failure != "" || s.TakeAnnouncement() != "" {
		t.Fatalf("a background failure reached the page: %+v", st)
	}
	s.Begin(updateT0.Add(time.Hour), false)
	finishAt(s, updateT0.Add(time.Hour), betaOffer, true, nil)
	if st := s.Status(); !st.HasUpdate() || s.TakeAnnouncement() != "" {
		t.Fatalf("a background success = %+v", st)
	}
}

func TestBackgroundFailureKeepsWhatThePageSaid(t *testing.T) {
	s := newIdleUpdateState()
	s.Begin(updateT0, true)
	finishAt(s, updateT0, betaOffer, true, nil)
	s.Advance(updateT0.Add(updateMinBusy))
	s.Begin(updateT0.Add(time.Hour), false)
	finishAt(s, updateT0.Add(time.Hour), updatecheck.Release{}, false, errOffline)
	if st := s.Status(); !st.HasUpdate() || st.CheckedAt != updateT0 {
		t.Fatalf("a background failure changed the page: %+v", st)
	}
}

func TestManualClickTakesOverARunningBackgroundCheck(t *testing.T) {
	s := newIdleUpdateState()
	s.Begin(updateT0, false)
	if s.Begin(updateT0.Add(time.Second), true) {
		t.Fatal("a second fetch was asked for")
	}
	if st := s.Status(); !st.Busy() {
		t.Fatalf("the click did not show the running check: %+v", st)
	}
	if s.TakeAnnouncement() != updateCheckingText {
		t.Fatal("the takeover was not announced")
	}
	finishAt(s, updateT0.Add(2*time.Second), updatecheck.Release{}, false, nil)
	s.Advance(updateT0.Add(time.Second + updateMinBusy - time.Millisecond))
	if !s.Status().Busy() {
		t.Fatal("the minimum busy time counts from the click")
	}
	s.Advance(updateT0.Add(time.Second + updateMinBusy))
	if s.Status().Phase != updateUpToDate {
		t.Fatalf("status = %+v", s.Status())
	}
}

func TestFinishWithoutARunningCheckIsIgnored(t *testing.T) {
	s := newIdleUpdateState()
	if finishAt(s, updateT0, betaOffer, true, nil) || s.Status().HasUpdate() {
		t.Fatal("a stray result was applied")
	}
}

func TestFailureStreakAsksForOneLogLineUntilASuccess(t *testing.T) {
	s := newIdleUpdateState()
	var firsts []bool
	check := func(err error) {
		s.Begin(updateT0, false)
		firsts = append(firsts, finishAt(s, updateT0, updatecheck.Release{}, false, err))
	}
	check(errOffline)
	check(errOffline)
	check(updatecheck.ErrFeedNotFound)
	check(nil)
	check(errOffline)
	want := []bool{true, false, false, false, true}
	for i := range want {
		if firsts[i] != want[i] {
			t.Fatalf("first-failure flags = %v, want %v", firsts, want)
		}
	}
}

func TestReclassifyFollowsTheBetaSwitchOnSettledResults(t *testing.T) {
	s := newIdleUpdateState()
	s.Begin(updateT0, true)
	finishAt(s, updateT0, updatecheck.Release{}, false, nil)
	s.Advance(updateT0.Add(updateMinBusy))
	s.Reclassify(betaOffer, true)
	if !s.Status().HasUpdate() {
		t.Fatal("the beta was not offered after the switch")
	}
	s.Reclassify(updatecheck.Release{}, false)
	if s.Status().Phase != updateUpToDate {
		t.Fatal("the beta was still offered after the switch")
	}
	s.Begin(updateT0, true)
	s.Reclassify(betaOffer, true)
	finishAt(s, updateT0, updatecheck.Release{}, false, nil)
	if s.Status().HasUpdate() {
		t.Fatal("Reclassify touched a check that was running")
	}
	failed := newIdleUpdateState()
	failed.Begin(updateT0, true)
	finishAt(failed, updateT0, updatecheck.Release{}, false, errOffline)
	failed.Advance(updateT0.Add(updateMinBusy))
	failed.Reclassify(betaOffer, true)
	if failed.Status().Phase != updateFailed {
		t.Fatal("Reclassify hid a failure")
	}
}

func TestIdleStateRefreshesTheRelativeTimeEveryMinute(t *testing.T) {
	if wake := newIdleUpdateState().NextWake(updateT0); wake != 0 {
		t.Fatalf("an unchecked state wakes after %v", wake)
	}
	s := newUpdateState(updateIdle, updateT0)
	if wake := s.NextWake(updateT0); wake != time.Minute {
		t.Fatalf("NextWake = %v, want a minute", wake)
	}
}

func TestUpdateStatusSentences(t *testing.T) {
	available := updateStatus{Phase: updateAvailable, Release: betaOffer}
	if got := available.Text(); got != "Version v0.2.9-beta.15 is available." {
		t.Errorf("available = %q", got)
	}
	available.Skipped = true
	if got := available.Text(); got != "Version v0.2.9-beta.15 is available (skipped)." {
		t.Errorf("skipped = %q", got)
	}
	if available.Notable() || !available.CanDownload() {
		t.Errorf("a skipped update must not be notable but can be downloaded: %+v", available)
	}
	noLink := updateStatus{Phase: updateAvailable, Release: updatecheck.Release{Tag: "v1.0.0"}}
	if noLink.CanDownload() || !noLink.Notable() {
		t.Errorf("an update without a link: %+v", noLink)
	}
	if got := updateFailureText(context.DeadlineExceeded); got != updateFailedText {
		t.Errorf("unknown failure = %q", got)
	}
}
