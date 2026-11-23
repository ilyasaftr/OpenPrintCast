package core

import (
	"reflect"
	"strconv"
	"sync"
	"testing"
)

func finishedEntry(status JobStatus) JobEntry {
	entry := NewJobEntry("p1", "u", "f.pdf", 1)
	entry.SetStatus(status)
	return entry
}

func TestJobLogUpdate(t *testing.T) {
	var log JobLog
	entry := finishedEntry(JobPrinting)
	log.Push(entry)
	if !log.Update(entry.ID, JobFailed, "spooler down") {
		t.Fatal("Update of an in-flight entry reported false")
	}
	got := log.List()[0]
	if got.Status != JobFailed || got.Reason != "spooler down" {
		t.Fatalf("entry = %+v", got)
	}
}

func TestJobLogUpdateKeepsFinishedEntries(t *testing.T) {
	var log JobLog
	entry := finishedEntry(JobCompleted)
	log.Push(entry)
	if log.Update(entry.ID, JobFailed, "late") {
		t.Fatal("Update changed a finished entry")
	}
	if got := log.List()[0]; got.Status != JobCompleted || got.Reason != "" {
		t.Fatalf("finished entry changed: %+v", got)
	}
}

func TestJobLogUpdateUnknownAndEvicted(t *testing.T) {
	var log JobLog
	if log.Update("missing", JobFailed, "") {
		t.Fatal("Update of an unknown id reported true")
	}
	first := finishedEntry(JobPrinting)
	log.Push(first)
	for i := 0; i < MaxLogEntries; i++ {
		log.Push(NewJobEntry("p1", "u", "f.pdf", 1))
	}
	if log.Update(first.ID, JobCompleted, "") {
		t.Fatal("Update of an evicted entry reported true")
	}
}

func TestJobLogUpdateToInFlightStatus(t *testing.T) {
	var log JobLog
	entry := NewJobEntry("p1", "u", "f.pdf", 1)
	log.Push(entry)
	var calls int
	log.Observe(func(JobEntry) { calls++ })
	if !log.Update(entry.ID, JobPrinting, "") || calls != 0 {
		t.Fatalf("pending -> printing: calls = %d", calls)
	}
	if !log.Update(entry.ID, JobCompleted, "") || calls != 1 {
		t.Fatalf("printing -> completed: calls = %d", calls)
	}
}

func TestJobLogObserveFiresForFinishedPushAndUpdate(t *testing.T) {
	var log JobLog
	var seen []JobEntry
	log.Observe(func(e JobEntry) { seen = append(seen, e) })
	log.Push(finishedEntry(JobFailed))
	inflight := finishedEntry(JobPrinting)
	log.Push(inflight)
	if len(seen) != 1 || seen[0].Status != JobFailed {
		t.Fatalf("observed %+v after pushes, want only the failed entry", seen)
	}
	log.Update(inflight.ID, JobCancelled, "Cancelled")
	if len(seen) != 2 || seen[1].ID != inflight.ID || seen[1].Reason != "Cancelled" {
		t.Fatalf("observed %+v after update", seen)
	}
}

func TestJobLogObserveIgnoresRejectedUpdates(t *testing.T) {
	var log JobLog
	entry := finishedEntry(JobCompleted)
	log.Push(entry)
	var calls int
	log.Observe(func(JobEntry) { calls++ })
	log.Update(entry.ID, JobFailed, "late")
	log.Update("missing", JobFailed, "")
	if calls != 0 {
		t.Fatalf("observer called %d times for rejected updates", calls)
	}
}

func TestJobLogObserveRunsAfterUnlock(t *testing.T) {
	var log JobLog
	inflight := finishedEntry(JobPrinting)
	log.Push(inflight)
	var listed int
	log.Observe(func(JobEntry) { listed = len(log.List()) })
	log.Update(inflight.ID, JobCompleted, "")
	log.Push(finishedEntry(JobFailed))
	if listed != 2 {
		t.Fatalf("observer saw %d entries; it must run with the lock released", listed)
	}
}

func TestJobLogObserveGetsCopies(t *testing.T) {
	var log JobLog
	inflight := finishedEntry(JobPrinting)
	log.Push(inflight)
	log.Observe(func(e JobEntry) { e.Reason = "mutated by observer" })
	log.Update(inflight.ID, JobFailed, "real")
	if got := log.List()[0].Reason; got != "real" {
		t.Fatalf("observer mutated the log: reason = %q", got)
	}
}

func TestJobLogObserversRunInRegistrationOrder(t *testing.T) {
	var log JobLog
	var order []int
	for i := 1; i <= 3; i++ {
		log.Observe(func(JobEntry) { order = append(order, i) })
	}
	log.Push(finishedEntry(JobCompleted))
	if len(order) != 3 || order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Fatalf("observer order = %v", order)
	}
}

func TestJobLogConcurrentPushUpdateObserve(t *testing.T) {
	var log JobLog
	var mu sync.Mutex
	var finished int
	var wg sync.WaitGroup
	const workers, perWorker = 8, 20
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			log.Observe(func(JobEntry) {})
			for i := 0; i < perWorker; i++ {
				entry := finishedEntry(JobPrinting)
				log.Push(entry)
				log.Update(entry.ID, JobCompleted, "")
				log.List()
			}
		}()
	}
	log.Observe(func(JobEntry) { mu.Lock(); finished++; mu.Unlock() })
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if finished == 0 || finished > workers*perWorker {
		t.Fatalf("finished notifications = %d", finished)
	}
}

func numberedEntries(from, to int, status JobStatus) []JobEntry {
	var out []JobEntry
	for n := from; n <= to; n++ {
		out = append(out, JobEntry{ID: "job-" + strconv.Itoa(n), Status: status})
	}
	return out
}

func entryIDs(entries []JobEntry) []string {
	var out []string
	for _, e := range entries {
		out = append(out, e.ID)
	}
	return out
}

func TestJobLogRestoreDoesNotNotifyObservers(t *testing.T) {
	var log JobLog
	calls := 0
	log.Observe(func(JobEntry) { calls++ })
	log.Restore(numberedEntries(1, 3, JobFailed))
	if calls != 0 {
		t.Fatalf("observer called %d times by Restore", calls)
	}
	if got := entryIDs(log.List()); !reflect.DeepEqual(got, []string{"job-1", "job-2", "job-3"}) {
		t.Fatalf("List = %v", got)
	}
}

func TestJobLogRestoreCapAndOrder(t *testing.T) {
	for _, tc := range []struct {
		name            string
		restored        int
		live            int
		wantFirst, want string
		wantLen         int
	}{
		{"empty restore", 0, 0, "", "", 0},
		{"under cap", 3, 0, "job-1", "job-3", 3},
		{"restore over cap keeps newest", MaxLogEntries + 5, 0, "job-6", "job-505", MaxLogEntries},
		{"restored sit before live entries", 2, 2, "job-1", "live-2", 4},
		{"live entries win the cap", MaxLogEntries, 2, "job-3", "live-2", MaxLogEntries},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var log JobLog
			for n := 1; n <= tc.live; n++ {
				log.Push(JobEntry{ID: "live-" + strconv.Itoa(n), Status: JobPrinting})
			}
			log.Restore(numberedEntries(1, tc.restored, JobCompleted))
			got := log.List()
			if len(got) != tc.wantLen {
				t.Fatalf("len = %d, want %d", len(got), tc.wantLen)
			}
			if tc.wantLen > 0 && (got[0].ID != tc.wantFirst || got[len(got)-1].ID != tc.want) {
				t.Fatalf("first/last = %s/%s, want %s/%s", got[0].ID, got[len(got)-1].ID, tc.wantFirst, tc.want)
			}
		})
	}
}

func TestJobLogRestoreThenPushAndUpdate(t *testing.T) {
	var log JobLog
	var seen []string
	log.Observe(func(e JobEntry) { seen = append(seen, e.ID) })
	log.Restore(numberedEntries(1, 2, JobCompleted))

	live := JobEntry{ID: "live", Status: JobPrinting}
	log.Push(live)
	if !log.Update("live", JobFailed, "boom") {
		t.Fatal("Update of a live entry after Restore failed")
	}
	if log.Update("job-1", JobFailed, "again") {
		t.Fatal("a restored finished entry must stay unchanged")
	}
	if got := entryIDs(log.List()); !reflect.DeepEqual(got, []string{"job-1", "job-2", "live"}) {
		t.Fatalf("List = %v", got)
	}
	if !reflect.DeepEqual(seen, []string{"live"}) {
		t.Fatalf("observer saw %v, want only the live job", seen)
	}
	if removed := log.ClearFinished(); removed != 3 || len(log.List()) != 0 {
		t.Fatalf("ClearFinished removed %d, left %v", removed, log.List())
	}
}

func TestJobLogRestoreDoesNotAliasInput(t *testing.T) {
	var log JobLog
	in := numberedEntries(1, 2, JobCompleted)
	log.Restore(in)
	in[0].ID = "mutated"
	if log.List()[0].ID != "job-1" {
		t.Fatal("Restore kept the caller's slice")
	}
}

// interleavedLog holds f1 p1 f2 f3 p2 f4: finished jobs around two live ones.
func interleavedLog() *JobLog {
	log := &JobLog{}
	for _, e := range []JobEntry{
		{ID: "f1", Status: JobCompleted}, {ID: "p1", Status: JobPending}, {ID: "f2", Status: JobFailed},
		{ID: "f3", Status: JobCancelled}, {ID: "p2", Status: JobPrinting}, {ID: "f4", Status: JobCompleted},
	} {
		log.Push(e)
	}
	return log
}

func TestJobLogTakeFinishedHandsBackWhatItRemoved(t *testing.T) {
	log := interleavedLog()
	cleared := log.TakeFinished()
	if got := entryIDs(cleared.Entries()); !reflect.DeepEqual(got, []string{"f1", "f2", "f3", "f4"}) || cleared.Len() != 4 {
		t.Fatalf("removed = %v", got)
	}
	if got := entryIDs(log.List()); !reflect.DeepEqual(got, []string{"p1", "p2"}) {
		t.Fatalf("left = %v, want only the live jobs", got)
	}
	if empty := (&JobLog{}).TakeFinished(); empty.Len() != 0 || len(empty.Entries()) != 0 {
		t.Fatal("an empty log removed something")
	}
}

func TestJobLogReinstatePutsJobsBackInTheirOriginalOrder(t *testing.T) {
	log := interleavedLog()
	cleared := log.TakeFinished()
	var seen []string
	log.Observe(func(e JobEntry) { seen = append(seen, e.ID) })

	if added := log.Reinstate(cleared); added != 4 {
		t.Fatalf("added %d, want 4", added)
	}
	want := []string{"f1", "p1", "f2", "f3", "p2", "f4"}
	if got := entryIDs(log.List()); !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	if len(seen) != 0 {
		t.Fatalf("observers saw %v; a reinstated job is history, not news", seen)
	}
}

func TestJobLogReinstateIsIdempotent(t *testing.T) {
	log := interleavedLog()
	cleared := log.TakeFinished()
	log.Reinstate(cleared)
	if added := log.Reinstate(cleared); added != 0 {
		t.Fatalf("second Reinstate added %d, want 0", added)
	}
	if got := entryIDs(log.List()); len(got) != 6 {
		t.Fatalf("order = %v", got)
	}
}

func TestJobLogReinstateKeepsJobsPushedMeanwhileAndTheCap(t *testing.T) {
	log := interleavedLog()
	cleared := log.TakeFinished()
	log.Push(JobEntry{ID: "new", Status: JobPending})
	log.Reinstate(cleared)
	want := []string{"f1", "p1", "f2", "f3", "p2", "f4", "new"}
	if got := entryIDs(log.List()); !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}

	full := &JobLog{}
	full.Restore(numberedEntries(1, MaxLogEntries, JobCompleted))
	cleared = full.TakeFinished()
	for n := 0; n < 3; n++ {
		full.Push(JobEntry{ID: "live-" + strconv.Itoa(n), Status: JobPending})
	}
	full.Reinstate(cleared)
	got := full.List()
	if len(got) != MaxLogEntries || got[len(got)-1].ID != "live-2" || got[0].ID != "job-4" {
		t.Fatalf("after the cap: %d entries, first %q, last %q", len(got), got[0].ID, got[len(got)-1].ID)
	}
}

func TestJobLogClearFinishedStillCounts(t *testing.T) {
	log := interleavedLog()
	if removed := log.ClearFinished(); removed != 4 || len(log.List()) != 2 {
		t.Fatalf("ClearFinished removed %d, left %v", removed, log.List())
	}
}

func TestJobLogSetPagesOnInFlightEntry(t *testing.T) {
	var log JobLog
	entry := NewJobEntry("p1", "u", "f.pdf", 0)
	entry.SetStatus(JobPrinting)
	log.Push(entry)
	var calls int
	log.Observe(func(JobEntry) { calls++ })
	if !log.SetPages(entry.ID, 12) {
		t.Fatal("SetPages of an in-flight entry reported false")
	}
	got := log.List()[0]
	if got.Pages != 12 || got.Status != JobPrinting || calls != 0 {
		t.Fatalf("entry = %+v, observer calls = %d", got, calls)
	}
}

func TestJobLogSetPagesOnFinishedEntry(t *testing.T) {
	var log JobLog
	entry := NewJobEntry("p1", "u", "f.pdf", 0)
	entry.SetStatus(JobCompleted)
	log.Push(entry)
	var calls int
	log.Observe(func(JobEntry) { calls++ })
	if !log.SetPages(entry.ID, 4) || log.List()[0].Pages != 4 || calls != 0 {
		t.Fatalf("finished entry = %+v, observer calls = %d", log.List()[0], calls)
	}
}

func TestJobLogSetPagesIgnoresNonPositiveCounts(t *testing.T) {
	var log JobLog
	entry := NewJobEntry("p1", "u", "f.pdf", 5)
	log.Push(entry)
	for _, pages := range []int{0, -3} {
		if !log.SetPages(entry.ID, pages) {
			t.Fatalf("SetPages(%d) of a known entry reported false", pages)
		}
	}
	if got := log.List()[0].Pages; got != 5 {
		t.Fatalf("Pages = %d, want the earlier 5", got)
	}
}

func TestJobLogSetPagesUnknownAndEvicted(t *testing.T) {
	var log JobLog
	if log.SetPages("missing", 3) || log.SetPages("", 3) {
		t.Fatal("SetPages of an unknown id reported true")
	}
	first := NewJobEntry("p1", "u", "f.pdf", 0)
	log.Push(first)
	for i := 0; i < MaxLogEntries; i++ {
		log.Push(NewJobEntry("p1", "u", "f.pdf", 1))
	}
	if log.SetPages(first.ID, 3) {
		t.Fatal("SetPages of an evicted entry reported true")
	}
}

func TestJobLogConcurrentSetPages(t *testing.T) {
	var log JobLog
	entry := NewJobEntry("p1", "u", "f.pdf", 0)
	entry.SetStatus(JobPrinting)
	log.Push(entry)
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				log.SetPages(entry.ID, 9)
				log.List()
			}
		}()
	}
	wg.Wait()
	if got := log.List()[0].Pages; got != 9 {
		t.Fatalf("Pages = %d, want 9", got)
	}
}
