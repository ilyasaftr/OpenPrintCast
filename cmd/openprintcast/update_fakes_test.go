package main

import (
	"context"
	"testing"
	"time"

	"github.com/ilyasaftr/OpenPrintCast/internal/updatecheck"
)

// fakeClock is a settable clock.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

// fakeTimer is one timer the scheduler asked for.
type fakeTimer struct {
	delay    time.Duration
	fn       func()
	canceled bool
	fired    bool
}

// fakeTimers is an afterFunc that never waits: the test fires timers.
type fakeTimers struct{ all []*fakeTimer }

func (f *fakeTimers) after(d time.Duration, fn func()) func() {
	t := &fakeTimer{delay: d, fn: fn}
	f.all = append(f.all, t)
	return func() { t.canceled = true }
}

// live is the timer that is still waiting, or nil.
func (f *fakeTimers) live() *fakeTimer {
	for i := len(f.all) - 1; i >= 0; i-- {
		if t := f.all[i]; !t.canceled && !t.fired {
			return t
		}
	}
	return nil
}

// fire runs the callback of t, even a canceled one, as a timer that had
// already left its queue would.
func (t *fakeTimer) fire() { t.fired = true; t.fn() }

// fakeUpdateSettings are the update settings of a test, in memory.
type fakeUpdateSettings struct {
	auto, beta bool
	skipped    string
	last       time.Time
	saves      int
}

func (f *fakeUpdateSettings) AutoCheck() bool            { return f.auto }
func (f *fakeUpdateSettings) IncludeBeta() bool          { return f.beta }
func (f *fakeUpdateSettings) Skipped() string            { return f.skipped }
func (f *fakeUpdateSettings) SetSkipped(tag string)      { f.skipped = tag; f.saves++ }
func (f *fakeUpdateSettings) LastChecked() time.Time     { return f.last }
func (f *fakeUpdateSettings) SetLastChecked(t time.Time) { f.last = t; f.saves++ }

// fakeReleaseFeed answers every fetch with the same releases or error.
type fakeReleaseFeed struct {
	releases []updatecheck.Release
	err      error
	panics   bool
	calls    int
	lastCtx  context.Context
}

func (f *fakeReleaseFeed) Fetch(ctx context.Context) ([]updatecheck.Release, error) {
	f.calls++
	f.lastCtx = ctx
	if f.panics {
		panic("feed exploded")
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return f.releases, f.err
}

// updateRig is an updateController with everything around it faked.
type updateRig struct {
	*updateController
	clock    *fakeClock
	timers   *fakeTimers
	settings *fakeUpdateSettings
	feed     *fakeReleaseFeed
	notices  *fakeNotifier
	held     *heldScan
	redraws  int
	jitters  []time.Duration
}

// newUpdateRig builds a controller for current v0.2.9-beta.14 whose feed
// offers v0.2.9-beta.15, with automatic checks and betas on.
func newUpdateRig(t *testing.T) *updateRig {
	t.Helper()
	r := &updateRig{
		clock:    &fakeClock{t: updateT0},
		timers:   &fakeTimers{},
		settings: &fakeUpdateSettings{auto: true, beta: true},
		feed:     &fakeReleaseFeed{releases: []updatecheck.Release{betaOffer, {Tag: "v0.2.9-beta.14", Prerelease: true}}},
		notices:  &fakeNotifier{},
		held:     &heldScan{},
	}
	r.updateController = newUpdateController(updateDeps{
		feed: r.feed, current: "v0.2.9-beta.14", phase: updateIdle, settings: r.settings,
		notifier: r.notices, run: r.held.run, now: r.clock.now, after: r.timers.after,
		jitter: func(limit time.Duration) time.Duration { r.jitters = append(r.jitters, limit); return limit / 2 },
		redraw: func() { r.redraws++ },
	})
	return r
}

// settle finishes the held fetch and lets the busy time of a manual check
// pass, as the frames of the window would.
func (r *updateRig) settle() {
	r.held.finish()
	r.Tick(r.clock.now().Add(updateMinBusy))
}
