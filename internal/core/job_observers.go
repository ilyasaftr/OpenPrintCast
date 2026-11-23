package core

import "sync"

// finishedObservers is the registry of callbacks told about jobs that
// reached a finished status. The zero value is ready to use.
type finishedObservers struct {
	obsMu     sync.RWMutex
	observers []func(JobEntry)
}

// Observe registers fn to be called with a copy of every entry that reaches
// a finished status: from Push of an already finished entry and from Update.
// Observers run after the log's lock is released, in the goroutine that made
// the change and in registration order, so they must be cheap. Panics are
// not recovered.
func (o *finishedObservers) Observe(fn func(JobEntry)) {
	o.obsMu.Lock()
	defer o.obsMu.Unlock()
	o.observers = append(o.observers, fn)
}

// notifyFinished calls every observer registered so far with entry.
func (o *finishedObservers) notifyFinished(entry JobEntry) {
	o.obsMu.RLock()
	observers := o.observers
	o.obsMu.RUnlock()
	for _, fn := range observers {
		fn(entry)
	}
}
