package main

import (
	"context"
	"errors"
	"time"

	"github.com/ilyasaftr/OpenPrintCast/internal/updatecheck"
)

// updateTick is what one frame of the page owes the update state: a
// sentence for screen readers (empty if none) and when to draw again (0 if
// never).
type updateTick struct {
	Announcement string
	Wake         time.Duration
}

// updateService is what the window, the tray and the app lifecycle need of
// update checking. All methods run on the UI thread.
type updateService interface {
	// Status is what the Software update row shows.
	Status() updateStatus
	// Tick moves the state on to now, as the page draws a frame.
	Tick(now time.Time) updateTick
	// CheckNow starts a manual check, which ignores the schedule; it
	// reports false when none was started (one is running, or the build
	// cannot check).
	CheckNow(now time.Time) bool
	// Skip stops offering the version on offer.
	Skip()
	// AutoCheckChanged and IncludeBetaChanged apply the two switches,
	// which the caller has already changed in the settings.
	AutoCheckChanged()
	IncludeBetaChanged()
	// Start begins the background checks; ctx ends them (and any fetch
	// under way) when the app quits. Stop ends them at once.
	Start(ctx context.Context)
	Stop()
}

// noUpdates is the service of fixtures and builds without one: update
// checking is not set up and nothing happens.
type noUpdates struct{}

func (noUpdates) Status() updateStatus      { return updateStatus{Phase: updateNotSetUp} }
func (noUpdates) Tick(time.Time) updateTick { return updateTick{} }
func (noUpdates) CheckNow(time.Time) bool   { return false }
func (noUpdates) Skip()                     {}
func (noUpdates) AutoCheckChanged()         {}
func (noUpdates) IncludeBetaChanged()       {}
func (noUpdates) Start(context.Context)     {}
func (noUpdates) Stop()                     {}

// releaseFeed lists the releases of the feed; *updatecheck.Feed does.
type releaseFeed interface {
	Fetch(ctx context.Context) ([]updatecheck.Release, error)
}

// updateSettings is the part of the settings the update feature reads and
// writes. The app's config backs it; tests use a fake.
type updateSettings interface {
	AutoCheck() bool
	IncludeBeta() bool
	Skipped() string
	SetSkipped(tag string)
	// LastChecked is when a check last finished, or the zero time.
	LastChecked() time.Time
	SetLastChecked(at time.Time)
}

// updateDeps are everything an updateController needs from outside.
type updateDeps struct {
	feed     releaseFeed
	current  string // the version releases are compared with
	phase    updatePhase
	settings updateSettings
	notifier notifier
	run      scanRunner // off the UI thread and back, as the scan does
	now      func() time.Time
	after    afterFunc
	jitter   func(limit time.Duration) time.Duration
	redraw   func()
}

// updateController is the production updateService: it runs checks off the
// UI thread, feeds the state machine, keeps the schedule and tells the user
// about new versions. It knows nothing of the toolkit.
type updateController struct {
	updateDeps
	state     *updateState
	sched     *updateScheduler
	announcer updateAnnouncer
	ctx       context.Context

	// releases is the last feed answer, kept so a changed beta switch can
	// be applied without asking the feed again.
	releases    []updatecheck.Release
	hasReleases bool
}

// newUpdateController wires the state machine and the scheduler to d.
func newUpdateController(d updateDeps) *updateController {
	c := &updateController{
		updateDeps: d,
		state:      newUpdateState(d.phase, d.settings.LastChecked()),
		announcer:  updateAnnouncer{notifier: d.notifier},
		ctx:        context.Background(),
	}
	c.sched = &updateScheduler{
		now: d.now, after: d.after, jitter: d.jitter, lastDone: d.settings.LastChecked,
		check: func() { c.start(c.now(), false) },
	}
	return c
}

// Status is the state, marked with whether the user skipped the version.
func (c *updateController) Status() updateStatus {
	status := c.state.Status()
	status.Skipped = status.HasUpdate() && status.Release.Tag == c.settings.Skipped()
	return status
}

// Tick advances the state to now.
func (c *updateController) Tick(now time.Time) updateTick {
	c.state.Advance(now)
	return updateTick{Announcement: c.state.TakeAnnouncement(), Wake: c.state.NextWake(now)}
}

// CheckNow starts a manual check at now, the time of the UI frame.
func (c *updateController) CheckNow(now time.Time) bool { return c.start(now, true) }

// start begins a check at now and sends the fetch off, unless the state
// refuses it.
func (c *updateController) start(now time.Time, manual bool) bool {
	if !c.state.Begin(now, manual) {
		return false
	}
	c.fetch(manual)
	return true
}

// fetch asks the feed off the UI thread and applies the answer on it.
func (c *updateController) fetch(manual bool) {
	var (
		releases []updatecheck.Release
		err      error
	)
	ctx, feed := c.ctx, c.feed
	c.run(func() { releases, err = safeFetch(ctx, feed) }, func() { c.finish(manual, releases, err) })
}

// safeFetch asks the feed and turns a panic into a failure, so a fault in
// the exchange cannot take the app down from its goroutine.
func safeFetch(ctx context.Context, feed releaseFeed) (releases []updatecheck.Release, err error) {
	defer func() {
		if r := recover(); r != nil {
			releases, err = nil, errors.New("update check panicked")
		}
	}()
	return feed.Fetch(ctx)
}

// finish applies a check's answer on the UI thread: the state, the log, the
// time of the check, the notice and the next appointment. An answer that
// only reports the app quitting is dropped.
func (c *updateController) finish(manual bool, releases []updatecheck.Release, err error) {
	if errors.Is(err, context.Canceled) {
		return
	}
	res := updateResult{err: err, at: c.now()}
	if err == nil {
		c.releases, c.hasReleases = releases, true
		res.release, res.found = c.newest()
	}
	if c.state.Finish(res) {
		componentLogger(componentUpdate).Warn("update check failed", "err", err)
	}
	c.settings.SetLastChecked(res.at)
	if res.found {
		c.offered(res.release, manual)
	}
	c.sched.Completed(err != nil, retryAfterOf(err))
	c.redraw()
}

// newest is the version to offer from the last feed answer.
func (c *updateController) newest() (updatecheck.Release, bool) {
	return updatecheck.Newest(c.releases, c.current, c.settings.IncludeBeta())
}

// offered handles a version found by a check: a newer one than the skipped
// version lifts the skip, and a background check announces it.
func (c *updateController) offered(release updatecheck.Release, manual bool) {
	if skipped := c.settings.Skipped(); skipped != "" && skipped != release.Tag && updatecheck.Newer(release.Tag, skipped) {
		c.settings.SetSkipped("")
	}
	if !manual {
		c.announcer.Announce(release, c.settings.Skipped())
	}
}

// retryAfterOf is the wait a rate-limited answer asked for, or 0.
func retryAfterOf(err error) time.Duration {
	var limit *updatecheck.RateLimitError
	if errors.As(err, &limit) {
		return limit.RetryAfter
	}
	return 0
}

// Skip remembers the version on offer as skipped.
func (c *updateController) Skip() {
	if status := c.state.Status(); status.HasUpdate() {
		c.settings.SetSkipped(status.Release.Tag)
		c.redraw()
	}
}

// AutoCheckChanged applies the automatic-checks switch to the schedule.
func (c *updateController) AutoCheckChanged() { c.sched.SetEnabled(c.settings.AutoCheck()) }

// IncludeBetaChanged re-reads the last feed answer under the new beta
// setting, so the page follows the switch without another request.
func (c *updateController) IncludeBetaChanged() {
	if c.hasReleases {
		c.state.Reclassify(c.newest())
	}
}

// Start begins the schedule, in a build that can check.
func (c *updateController) Start(ctx context.Context) {
	c.ctx = ctx
	if c.state.Status().Unavailable() != "" {
		return
	}
	c.sched.SetEnabled(c.settings.AutoCheck())
	c.sched.Start()
}

// Stop ends the schedule.
func (c *updateController) Stop() { c.sched.Stop() }
