package core

import (
	"encoding/csv"
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// ErrJobNotFound is returned when no job matches an id.
var ErrJobNotFound = errors.New("job not found")

// MaxLogEntries caps the in-memory job history.
const MaxLogEntries = 500

// JobLog is a bounded, thread-safe print-job history.
type JobLog struct {
	mu      sync.Mutex
	entries []JobEntry
	finishedObservers
}

// Push appends entry, evicting the oldest entries past the cap. Observers
// are notified when entry is already finished.
func (l *JobLog) Push(entry JobEntry) {
	l.mu.Lock()
	l.entries = append(l.entries, entry)
	for len(l.entries) > MaxLogEntries {
		l.entries = l.entries[1:]
	}
	l.mu.Unlock()
	if entry.Status.Finished() {
		l.notifyFinished(entry)
	}
}

// Restore seeds the log with entries from an earlier run, given oldest
// first. They are placed before any entry already in the log (history is
// older than what is live), and at most MaxLogEntries of the newest are
// kept. Observers are NOT notified: restored jobs are history, not news.
// It is meant for startup, before observers are registered and the server
// accepts jobs; entries are stored as given, so pass only finished ones.
func (l *JobLog) Restore(entries []JobEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	merged := make([]JobEntry, 0, len(entries)+len(l.entries))
	merged = append(merged, entries...)
	merged = append(merged, l.entries...)
	if len(merged) > MaxLogEntries {
		merged = merged[len(merged)-MaxLogEntries:]
	}
	l.entries = merged
}

// Update sets the status and reason of the entry with id. It reports false
// when the entry is unknown (e.g. evicted) or already finished, since a
// finished job never changes again. Observers are notified when status is
// finished.
func (l *JobLog) Update(id string, status JobStatus, reason string) bool {
	entry, ok := l.applyUpdate(id, status, reason)
	if ok && status.Finished() {
		l.notifyFinished(entry)
	}
	return ok
}

// applyUpdate mutates the entry under the lock and returns a copy of it.
func (l *JobLog) applyUpdate(id string, status JobStatus, reason string) (JobEntry, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range l.entries {
		entry := &l.entries[i]
		if entry.ID != id {
			continue
		}
		if entry.Status.Finished() {
			return JobEntry{}, false
		}
		entry.Status, entry.Reason = status, reason
		return *entry, true
	}
	return JobEntry{}, false
}

// SetPages records the page count of the entry with id and reports whether
// the entry was found. A count that is not positive is ignored (unknown stays
// unknown). It works for in-flight entries, so a job shows its count while
// it is still printing, and a finished entry may still receive it. Observers
// are not notified: the count is a detail of the entry, not a status change.
func (l *JobLog) SetPages(id string, pages int) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	i := l.indexOf(id)
	if i < 0 {
		return false
	}
	if pages > 0 {
		l.entries[i].Pages = pages
	}
	return true
}

// List returns a snapshot copy, newest last.
func (l *JobLog) List() []JobEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]JobEntry, len(l.entries))
	copy(out, l.entries)
	return out
}

// Cancel marks a pending/printing job cancelled.
func (l *JobLog) Cancel(id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range l.entries {
		if l.entries[i].ID != id {
			continue
		}
		switch l.entries[i].Status {
		case JobPending, JobPrinting:
			l.entries[i].Status = JobCancelled
			return nil
		default:
			return errors.New("job already finished: " + id)
		}
	}
	return ErrJobNotFound
}

// ClearedJobs is what TakeFinished removed from a JobLog: the finished
// entries and where each sat, enough for Reinstate to put them back.
type ClearedJobs struct {
	items []clearedJob
}

// clearedJob is one removed entry and the id of the entry that preceded it
// in the log ("" when it was the oldest).
type clearedJob struct {
	entry JobEntry
	after string
}

// Len is how many jobs were removed.
func (c ClearedJobs) Len() int { return len(c.items) }

// Entries returns the removed jobs, oldest first.
func (c ClearedJobs) Entries() []JobEntry {
	out := make([]JobEntry, len(c.items))
	for i, it := range c.items {
		out[i] = it.entry
	}
	return out
}

// TakeFinished drops every finished job and hands them back, so the caller
// can offer to undo with Reinstate. Unfinished jobs stay in place.
func (l *JobLog) TakeFinished() ClearedJobs {
	l.mu.Lock()
	defer l.mu.Unlock()
	var cleared ClearedJobs
	kept := l.entries[:0]
	prev := ""
	for _, e := range l.entries {
		if e.Status.Finished() {
			cleared.items = append(cleared.items, clearedJob{entry: e, after: prev})
		} else {
			kept = append(kept, e)
		}
		prev = e.ID
	}
	clear(l.entries[len(kept):]) // let the dropped entries go
	l.entries = kept
	return cleared
}

// ClearFinished drops every finished job, returning how many were removed.
func (l *JobLog) ClearFinished() int { return l.TakeFinished().Len() }

// Reinstate puts jobs removed by TakeFinished back where they were: each
// right after the entry that preceded it (older jobs first when that entry
// is gone, such as evicted by the cap), then the newest MaxLogEntries are
// kept. It is idempotent: a job whose id is already in the log is skipped,
// so a second call changes nothing. Observers are NOT notified, as with
// Restore: these are history, not news. It returns how many were added.
func (l *JobLog) Reinstate(cleared ClearedJobs) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	added := 0
	for _, it := range cleared.items {
		if l.indexOf(it.entry.ID) >= 0 {
			continue
		}
		at := l.indexOf(it.after) + 1 // an unknown or empty predecessor gives 0
		l.entries = slices.Insert(l.entries, at, it.entry)
		added++
	}
	if len(l.entries) > MaxLogEntries {
		l.entries = slices.Clone(l.entries[len(l.entries)-MaxLogEntries:])
	}
	return added
}

// indexOf is the position of the entry with id, or -1; the caller holds
// the lock.
func (l *JobLog) indexOf(id string) int {
	if id == "" {
		return -1
	}
	return slices.IndexFunc(l.entries, func(e JobEntry) bool { return e.ID == id })
}

// ToCSV exports the full history, one row per job, for accounting.
func (l *JobLog) ToCSV() (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	var sb strings.Builder
	w := csv.NewWriter(&sb)
	if err := w.Write([]string{"id", "printer", "user", "file", "pages", "status"}); err != nil {
		return "", err
	}
	for _, e := range l.entries {
		if err := w.Write([]string{
			e.ID, e.PrinterID, e.User, e.Filename,
			strconv.Itoa(e.Pages), string(e.Status),
		}); err != nil {
			return "", err
		}
	}
	w.Flush()
	return sb.String(), w.Error()
}
