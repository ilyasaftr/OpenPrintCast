package net

import "sync"

// activeJobs maps the job-log id of every in-flight job to the handle that
// cancels it. The zero value is ready to use.
type activeJobs struct {
	mu      sync.Mutex
	cancels map[string]func() bool
}

// register stores cancel for id and returns an idempotent release func.
// cancel reports whether it actually stopped the job. A nil cancel registers
// nothing: the job cannot be cancelled.
func (a *activeJobs) register(id string, cancel func() bool) (release func()) {
	if cancel == nil {
		return func() {}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancels == nil {
		a.cancels = make(map[string]func() bool)
	}
	a.cancels[id] = cancel
	return func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		delete(a.cancels, id)
	}
}

// cancel invokes the handle of id, if any, and reports whether the job was
// stopped. The handle runs without the lock held.
func (a *activeJobs) cancel(id string) bool {
	a.mu.Lock()
	cancel := a.cancels[id]
	a.mu.Unlock()
	return cancel != nil && cancel()
}
