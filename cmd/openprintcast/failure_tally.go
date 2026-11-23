package main

import (
	"sync"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// unseenFailures is what the Jobs tab's badge needs: how many jobs failed
// since the user last looked at the Jobs page, and a way to say they now
// have. *failureTally implements it.
type unseenFailures interface {
	Unseen() int
	MarkSeen()
}

// failureTally counts jobs that failed while the Jobs page was not on
// screen. OnJob is a JobLog observer, so it runs on the server goroutine
// while the UI thread reads and resets the count: the tally is safe for
// concurrent use. Jobs restored from an earlier run never reach it, as the
// log does not notify for restored history. The zero value is ready to use.
type failureTally struct {
	// changed, when set before the tally is shared, runs after each new
	// failure, for the window to redraw its badge. It may be called from
	// any goroutine.
	changed func()

	mu    sync.Mutex
	count int
}

// OnJob is the JobLog observer: it counts failed jobs, not the other
// finished ones.
func (t *failureTally) OnJob(job core.JobEntry) {
	if job.Status != core.JobFailed {
		return
	}
	t.mu.Lock()
	t.count++
	t.mu.Unlock()
	if t.changed != nil {
		t.changed()
	}
}

// Unseen is the number of failures since the last MarkSeen.
func (t *failureTally) Unseen() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.count
}

// MarkSeen forgets the failures counted so far.
func (t *failureTally) MarkSeen() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.count = 0
}
