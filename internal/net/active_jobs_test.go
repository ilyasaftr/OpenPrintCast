package net

import (
	"sync"
	"testing"
)

func TestActiveJobsCancelInvokesRegisteredHandle(t *testing.T) {
	var active activeJobs
	calls := 0
	active.register("job-1", func() bool { calls++; return true })
	if !active.cancel("job-1") || calls != 1 {
		t.Fatalf("cancel = false or calls = %d", calls)
	}
}

func TestActiveJobsCancelReportsHandleOutcome(t *testing.T) {
	var active activeJobs
	active.register("job-1", func() bool { return false })
	if active.cancel("job-1") {
		t.Fatal("cancel reported true although the handle did not stop the job")
	}
}

func TestActiveJobsUnknownAndNilHandle(t *testing.T) {
	var active activeJobs
	if active.cancel("missing") {
		t.Fatal("unknown id cancelled")
	}
	release := active.register("job-1", nil)
	release()
	if active.cancel("job-1") {
		t.Fatal("nil handle must register nothing")
	}
}

func TestActiveJobsReleaseIsIdempotent(t *testing.T) {
	var active activeJobs
	release := active.register("job-1", func() bool { return true })
	release()
	release()
	if active.cancel("job-1") {
		t.Fatal("released handle still cancels")
	}
}

func TestActiveJobsHandleMayCallBack(t *testing.T) {
	var active activeJobs
	var release func()
	release = active.register("job-1", func() bool { release(); return true })
	if !active.cancel("job-1") {
		t.Fatal("handle re-entering the registry deadlocked or failed")
	}
}

func TestActiveJobsConcurrentUse(t *testing.T) {
	var active activeJobs
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			release := active.register("job", func() bool { return true })
			active.cancel("job")
			release()
		}()
	}
	wg.Wait()
}
