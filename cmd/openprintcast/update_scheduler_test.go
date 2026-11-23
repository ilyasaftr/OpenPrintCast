package main

import (
	"testing"
	"time"
)

// schedulerRig is an updateScheduler over a fake clock and fake timers.
type schedulerRig struct {
	*updateScheduler
	clock   *fakeClock
	timers  *fakeTimers
	checks  int
	last    time.Time
	jitters []time.Duration
}

func newSchedulerRig(enabled bool) *schedulerRig {
	r := &schedulerRig{clock: &fakeClock{t: updateT0}, timers: &fakeTimers{}}
	r.updateScheduler = &updateScheduler{
		now: r.clock.now, after: r.timers.after, enabled: enabled,
		jitter:   func(limit time.Duration) time.Duration { r.jitters = append(r.jitters, limit); return limit },
		lastDone: func() time.Time { return r.last },
		check:    func() { r.checks++ },
	}
	return r
}

func TestSchedulerFirstCheckComesTenSecondsAfterStart(t *testing.T) {
	r := newSchedulerRig(true)
	r.Start()
	if live := r.timers.live(); live == nil || live.delay != 10*time.Second {
		t.Fatalf("first timer = %+v", live)
	}
	r.timers.live().fire()
	if r.checks != 1 || r.timers.live() != nil {
		t.Fatalf("checks = %d, live timer = %+v; the next one waits for Completed", r.checks, r.timers.live())
	}
}

func TestSchedulerNeverChecksSoonerThanSixHoursAfterTheLastCheck(t *testing.T) {
	for _, tc := range []struct {
		name string
		ago  time.Duration
		want time.Duration
	}{
		{"never checked", 0, updateFirstDelay},
		{"an hour ago", time.Hour, 5 * time.Hour},
		{"just now", time.Second, 6*time.Hour - time.Second},
		{"exactly six hours ago", 6 * time.Hour, updateFirstDelay},
		{"a week ago", 7 * 24 * time.Hour, updateFirstDelay},
		{"in the future (a clock set back)", -time.Hour, 6 * time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newSchedulerRig(true)
			if tc.ago != 0 {
				r.last = updateT0.Add(-tc.ago)
			}
			r.Start()
			if live := r.timers.live(); live == nil || live.delay != tc.want {
				t.Fatalf("delay = %+v, want %v", live, tc.want)
			}
		})
	}
}

func TestSchedulerNextCheckIsADayLaterWithUpToTenPercentJitter(t *testing.T) {
	r := newSchedulerRig(true)
	r.Start()
	r.timers.live().fire()
	r.Completed(false, 0)
	if want := 24*time.Hour + 144*time.Minute; r.timers.live().delay != want {
		t.Fatalf("delay = %v, want %v (a day plus the whole jitter, as the fake returns the limit)", r.timers.live().delay, want)
	}
	if len(r.jitters) != 1 || r.jitters[0] != 144*time.Minute {
		t.Fatalf("jitter limits = %v, want 10%% of a day", r.jitters)
	}
}

func TestSchedulerRetriesAFailureAfterSixHoursOrWhatTheFeedAsked(t *testing.T) {
	for _, tc := range []struct {
		name       string
		retryAfter time.Duration
		base       time.Duration
	}{
		{"no request", 0, 6 * time.Hour},
		{"a short request", time.Minute, 6 * time.Hour},
		{"a long request", 12 * time.Hour, 12 * time.Hour},
		{"an absurd request", 90 * time.Hour, 24 * time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newSchedulerRig(true)
			r.Start()
			r.timers.live().fire()
			r.Completed(true, tc.retryAfter)
			if want := tc.base + tc.base/100*updateJitterPercent; r.timers.live().delay != want {
				t.Fatalf("delay = %v, want %v", r.timers.live().delay, want)
			}
		})
	}
}

func TestSchedulerFollowsTheSwitchWhileRunning(t *testing.T) {
	r := newSchedulerRig(false)
	r.Start()
	if r.timers.live() != nil {
		t.Fatal("a timer ran with automatic checks off")
	}
	r.SetEnabled(true)
	first := r.timers.live()
	if first == nil || first.delay != updateFirstDelay {
		t.Fatalf("switching on armed %+v", first)
	}
	r.SetEnabled(true) // no change: no second timer
	if len(r.timers.all) != 1 {
		t.Fatalf("a repeated switch armed %d timers", len(r.timers.all))
	}
	r.SetEnabled(false)
	if !first.canceled || r.timers.live() != nil {
		t.Fatal("switching off left the timer running")
	}
	first.fire() // a callback that was already on its way
	if r.checks != 0 {
		t.Fatal("a canceled timer started a check")
	}
	r.Completed(false, 0) // a check that was running finishes after the switch
	if r.timers.live() != nil {
		t.Fatal("a finished check re-armed a switched-off schedule")
	}
}

func TestSchedulerSwitchingOnHonorsTheThrottle(t *testing.T) {
	r := newSchedulerRig(false)
	r.Start()
	r.last = updateT0.Add(-time.Hour)
	r.SetEnabled(true)
	if live := r.timers.live(); live == nil || live.delay != 5*time.Hour {
		t.Fatalf("delay = %+v, want 5h", live)
	}
}

func TestSchedulerReplacesItsTimerAndIgnoresStaleOnes(t *testing.T) {
	r := newSchedulerRig(true)
	r.Start()
	stale := r.timers.live()
	r.Completed(false, 0) // a manual check finished meanwhile
	if !stale.canceled || r.timers.live() == stale {
		t.Fatal("the old timer was kept")
	}
	stale.fire()
	if r.checks != 0 {
		t.Fatal("a stale timer started a check")
	}
	r.timers.live().fire()
	if r.checks != 1 {
		t.Fatalf("checks = %d", r.checks)
	}
}

func TestSchedulerStopCancelsForGood(t *testing.T) {
	r := newSchedulerRig(true)
	r.Start()
	timer := r.timers.live()
	r.Stop()
	if !timer.canceled {
		t.Fatal("Stop left the timer running")
	}
	r.Completed(false, 0)
	r.SetEnabled(false)
	r.SetEnabled(true)
	if r.timers.live() != nil {
		t.Fatal("a stopped scheduler armed a timer")
	}
	timer.fire()
	if r.checks != 0 {
		t.Fatal("a stopped scheduler checked")
	}
}

func TestRandomJitterStaysBelowItsLimit(t *testing.T) {
	if randomJitter(0) != 0 || randomJitter(-time.Hour) != 0 {
		t.Fatal("jitter without a limit")
	}
	limit := 144 * time.Minute
	var distinct = map[time.Duration]bool{}
	for i := 0; i < 500; i++ {
		got := randomJitter(limit)
		if got < 0 || got >= limit {
			t.Fatalf("jitter %v outside [0, %v)", got, limit)
		}
		distinct[got] = true
	}
	if len(distinct) < 100 {
		t.Fatalf("only %d distinct values: not random", len(distinct))
	}
}
