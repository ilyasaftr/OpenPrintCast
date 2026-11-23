package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ilyasaftr/OpenPrintCast/internal/updatecheck"
)

// runBackgroundCheck fires the armed timer and completes the fetch.
func (r *updateRig) runBackgroundCheck(t *testing.T) {
	t.Helper()
	timer := r.timers.live()
	if timer == nil {
		t.Fatal("no timer is armed")
	}
	timer.fire()
	if !r.held.pending() {
		t.Fatal("the timer did not start a fetch")
	}
	r.held.finish()
}

func TestBackgroundCheckAnnouncesAnUpdateOncePerVersion(t *testing.T) {
	r := newUpdateRig(t)
	r.Start(context.Background())
	if live := r.timers.live(); live == nil || live.delay != updateFirstDelay {
		t.Fatalf("armed %+v", live)
	}
	r.runBackgroundCheck(t)
	want := userNotice{ID: "update-available", Title: "Update available", Body: "OpenPrintCast v0.2.9-beta.15 is ready to download."}
	if got := r.notices.got(); len(got) != 1 || got[0] != want {
		t.Fatalf("notices = %+v, want %+v", got, want)
	}
	if st := r.Status(); !st.HasUpdate() || !st.Notable() || st.Release.Tag != betaOffer.Tag {
		t.Fatalf("status = %+v", st)
	}
	r.clock.advance(updateInterval)
	r.runBackgroundCheck(t)
	if len(r.notices.got()) != 1 {
		t.Fatalf("the same version was announced again: %+v", r.notices.got())
	}
	r.feed.releases = append(r.feed.releases, updatecheck.Release{Tag: "v0.2.9-beta.16", Prerelease: true})
	r.clock.advance(updateInterval)
	r.runBackgroundCheck(t)
	if got := r.notices.got(); len(got) != 2 || !strings.Contains(got[1].Body, "v0.2.9-beta.16") || got[1].ID != got[0].ID {
		t.Fatalf("the newer version was not announced under the same notice ID: %+v", got)
	}
}

func TestBackgroundCheckDoesNotAnnounceASkippedVersion(t *testing.T) {
	r := newUpdateRig(t)
	r.settings.skipped = betaOffer.Tag
	r.Start(context.Background())
	r.runBackgroundCheck(t)
	if len(r.notices.got()) != 0 {
		t.Fatalf("a skipped version was announced: %+v", r.notices.got())
	}
	if st := r.Status(); !st.HasUpdate() || !st.Skipped || st.Notable() || !st.CanDownload() {
		t.Fatalf("the skipped version must stay listed and downloadable: %+v", st)
	}
	if got := r.Status().Text(); got != "Version v0.2.9-beta.15 is available (skipped)." {
		t.Fatalf("text = %q", got)
	}
}

func TestNewerVersionLiftsTheSkip(t *testing.T) {
	r := newUpdateRig(t)
	r.settings.skipped = "v0.2.9-beta.14"
	r.feed.releases = []updatecheck.Release{betaOffer}
	r.Start(context.Background())
	r.runBackgroundCheck(t)
	if r.settings.skipped != "" || len(r.notices.got()) != 1 {
		t.Fatalf("skipped = %q, notices = %d", r.settings.skipped, len(r.notices.got()))
	}
}

func TestSkipThenLowerVersionKeepsTheSkip(t *testing.T) {
	r := newUpdateRig(t)
	r.settings.skipped = "v0.2.9-beta.20"
	r.Start(context.Background())
	r.runBackgroundCheck(t)
	if r.settings.skipped != "v0.2.9-beta.20" {
		t.Fatalf("an older version lifted the skip: %q", r.settings.skipped)
	}
}

func TestSkipRemembersTheVersionOnOffer(t *testing.T) {
	r := newUpdateRig(t)
	r.Skip() // nothing on offer: nothing to skip
	if r.settings.skipped != "" {
		t.Fatalf("skipped %q with nothing on offer", r.settings.skipped)
	}
	r.CheckNow(updateT0)
	r.settle()
	r.Skip()
	if r.settings.skipped != betaOffer.Tag || !r.Status().Skipped {
		t.Fatalf("skipped = %q, status = %+v", r.settings.skipped, r.Status())
	}
}

func TestManualCheckBypassesTheThrottleAndNeverNotifies(t *testing.T) {
	r := newUpdateRig(t)
	r.settings.last = updateT0.Add(-time.Minute) // a background check ran a minute ago
	r.Start(context.Background())
	if live := r.timers.live(); live == nil || live.delay != 6*time.Hour-time.Minute {
		t.Fatalf("the schedule ignored the last check: %+v", live)
	}
	if !r.CheckNow(updateT0) || !r.held.pending() {
		t.Fatal("a manual check was throttled")
	}
	if r.CheckNow(updateT0) {
		t.Fatal("a second manual check began")
	}
	r.settle()
	if r.feed.calls != 1 || len(r.notices.got()) != 0 || !r.Status().HasUpdate() {
		t.Fatalf("fetches = %d, notices = %+v, status = %+v", r.feed.calls, r.notices.got(), r.Status())
	}
	if live := r.timers.live(); live == nil || live.delay != 24*time.Hour+72*time.Minute {
		t.Fatalf("a manual check must restart the day: %+v", live)
	}
}

func TestEveryCompletedCheckIsRecordedForTheThrottle(t *testing.T) {
	r := newUpdateRig(t)
	r.Start(context.Background())
	r.clock.advance(time.Minute)
	r.runBackgroundCheck(t)
	if !r.settings.last.Equal(r.clock.now()) {
		t.Fatalf("a successful check was not recorded: %v", r.settings.last)
	}
	r.feed.err = errOffline
	r.clock.advance(updateInterval)
	r.runBackgroundCheck(t)
	if !r.settings.last.Equal(r.clock.now()) {
		t.Fatalf("a failed check was not recorded: %v", r.settings.last)
	}
}

func TestFailureStreakIsLoggedOnceAndNeverShown(t *testing.T) {
	logs := captureLogs(t)
	r := newUpdateRig(t)
	r.feed.err = errOffline
	r.Start(context.Background())
	for i := 0; i < 3; i++ {
		r.runBackgroundCheck(t)
		if live := r.timers.live(); live == nil || live.delay < updateMinInterval || live.delay >= updateInterval {
			t.Fatalf("after a failure the retry is %+v, want about six hours", live)
		}
	}
	if n := strings.Count(logs.String(), "update check failed"); n != 1 {
		t.Fatalf("logged %d times, want once per streak:\n%s", n, logs)
	}
	if st := r.Status(); st.Phase != updateIdle || st.Failure != "" || len(r.notices.got()) != 0 {
		t.Fatalf("a background failure reached the user: %+v, %+v", st, r.notices.got())
	}
	r.feed.err = nil
	r.runBackgroundCheck(t)
	r.feed.err = errOffline
	r.runBackgroundCheck(t)
	if n := strings.Count(logs.String(), "update check failed"); n != 2 {
		t.Fatalf("a new streak after a success must log again; logged %d times", n)
	}
}

func TestRateLimitedFeedIsNotAskedAgainBeforeItAllows(t *testing.T) {
	r := newUpdateRig(t)
	r.feed.err = &updatecheck.RateLimitError{RetryAfter: 10 * time.Hour}
	r.Start(context.Background())
	r.runBackgroundCheck(t)
	if live := r.timers.live(); live == nil || live.delay < 10*time.Hour {
		t.Fatalf("retry = %+v, want at least the 10 hours the feed asked for", live)
	}
}

func TestManualFailureShowsOnThePageWithoutANotice(t *testing.T) {
	r := newUpdateRig(t)
	r.feed.err = errOffline
	r.CheckNow(updateT0)
	r.settle()
	if st := r.Status(); st.Phase != updateFailed || st.Text() != updateOfflineText {
		t.Fatalf("status = %+v", st)
	}
	if len(r.notices.got()) != 0 {
		t.Fatalf("a failed manual check posted a notice: %+v", r.notices.got())
	}
}

func TestSwitchingAutomaticChecksOnAndOffIsLive(t *testing.T) {
	r := newUpdateRig(t)
	r.Start(context.Background())
	armed := r.timers.live()
	r.settings.auto = false
	r.AutoCheckChanged()
	if !armed.canceled || r.timers.live() != nil {
		t.Fatal("switching off left a timer")
	}
	r.settings.auto = true
	r.AutoCheckChanged()
	if r.timers.live() == nil {
		t.Fatal("switching on did not arm a timer")
	}
	if !r.CheckNow(updateT0) || !r.held.pending() {
		t.Fatal("a manual check did not start")
	}
}

func TestManualChecksWorkWithAutomaticChecksOff(t *testing.T) {
	r := newUpdateRig(t)
	r.settings.auto = false
	r.Start(context.Background())
	if r.timers.live() != nil {
		t.Fatal("a timer ran with automatic checks off")
	}
	r.CheckNow(updateT0)
	r.settle()
	if !r.Status().HasUpdate() || r.timers.live() != nil || len(r.notices.got()) != 0 {
		t.Fatalf("status = %+v, timer = %+v", r.Status(), r.timers.live())
	}
}

func TestBetaSwitchRereadsTheLastAnswerWithoutAskingAgain(t *testing.T) {
	r := newUpdateRig(t)
	r.settings.beta = false
	r.CheckNow(updateT0)
	r.settle()
	if r.Status().Phase != updateUpToDate {
		t.Fatalf("betas excluded, status = %+v", r.Status())
	}
	r.settings.beta = true
	r.IncludeBetaChanged()
	if !r.Status().HasUpdate() || r.feed.calls != 1 {
		t.Fatalf("status = %+v after %d fetches", r.Status(), r.feed.calls)
	}
	r.settings.beta = false
	r.IncludeBetaChanged()
	if r.Status().Phase != updateUpToDate {
		t.Fatalf("status = %+v", r.Status())
	}
	fresh := newUpdateRig(t)
	fresh.IncludeBetaChanged() // no answer yet: nothing to re-read
	if fresh.Status().Phase != updateIdle {
		t.Fatalf("status = %+v", fresh.Status())
	}
}

func TestBuildsThatCannotCheckNeverFetchOrArm(t *testing.T) {
	for _, phase := range []updatePhase{updateNotSetUp, updateDevBuild} {
		r := newUpdateRig(t)
		r.state = newUpdateState(phase, time.Time{})
		r.Start(context.Background())
		if r.timers.live() != nil || r.CheckNow(updateT0) || r.feed.calls != 0 || r.held.pending() {
			t.Fatalf("phase %d checked or armed a timer", phase)
		}
		if r.Status().Unavailable() == "" {
			t.Fatalf("phase %d gave no reason", phase)
		}
	}
}

func TestQuitCancelsTheFetchAndTheTimer(t *testing.T) {
	r := newUpdateRig(t)
	ctx, cancel := context.WithCancel(context.Background())
	r.Start(ctx)
	r.timers.live().fire()
	cancel() // the app quits while the fetch is under way
	r.Stop()
	r.held.finish()
	if r.feed.lastCtx != ctx {
		t.Fatal("the fetch was not given the app's context")
	}
	if len(r.notices.got()) != 0 || r.settings.saves != 0 || r.timers.live() != nil || r.Status().HasUpdate() {
		t.Fatalf("an answer that arrived after quit was applied: saves=%d status=%+v", r.settings.saves, r.Status())
	}
}

func TestAFaultInTheFeedBecomesAFailure(t *testing.T) {
	r := newUpdateRig(t)
	r.feed.panics = true
	r.CheckNow(updateT0)
	r.settle()
	if st := r.Status(); st.Phase != updateFailed {
		t.Fatalf("status = %+v", st)
	}
}

func TestTickAnnouncesAndAsksForTheNextFrame(t *testing.T) {
	r := newUpdateRig(t)
	r.CheckNow(updateT0)
	if tick := r.Tick(updateT0); tick.Announcement != updateCheckingText {
		t.Fatalf("tick = %+v", tick)
	}
	r.held.finish()
	if tick := r.Tick(updateT0); tick.Wake != updateMinBusy {
		t.Fatalf("tick = %+v, want a wake at the end of the busy time", tick)
	}
	if tick := r.Tick(updateT0.Add(updateMinBusy)); tick.Announcement == "" || tick.Wake != updateRelativeRefresh {
		t.Fatalf("tick = %+v", tick)
	}
}

func TestAppWithoutAnUpdateServiceIsNotSetUp(t *testing.T) {
	app := &bridgeApp{}
	if st := app.updates().Status(); st.Phase != updateNotSetUp || st.Checkable() || st.Notable() {
		t.Fatalf("default status = %+v", st)
	}
	app.updates().Start(context.Background())
	app.updates().Skip()
	app.updates().AutoCheckChanged()
	app.updates().IncludeBetaChanged()
	app.updates().Stop()
	if app.updates().CheckNow(updateT0) || app.updates().Tick(updateT0) != (updateTick{}) {
		t.Fatal("the default service did something")
	}
}
