package jobhistory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

func entry(n int, status core.JobStatus) core.JobEntry {
	return core.JobEntry{
		ID: fmt.Sprintf("job-%d", n), PrinterID: "p1", User: "u", Filename: fmt.Sprintf("f%d.pdf", n),
		Pages: n, Status: status, CreatedAt: int64(1000 + n),
	}
}

func ids(entries []core.JobEntry) []string {
	var out []string
	for _, e := range entries {
		out = append(out, e.ID)
	}
	return out
}

func idRange(from, to int) []string {
	var out []string
	for n := from; n <= to; n++ {
		out = append(out, fmt.Sprintf("job-%d", n))
	}
	return out
}

func newStore(t *testing.T, limit int) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nested", FileName)
	return Open(path, limit), path
}

func mustAppend(t *testing.T, s *Store, entries ...core.JobEntry) {
	t.Helper()
	for _, e := range entries {
		if err := s.Append(e); err != nil {
			t.Fatalf("Append(%s): %v", e.ID, err)
		}
	}
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func TestOpenDefaultsLimit(t *testing.T) {
	for _, limit := range []int{0, -3} {
		if got := Open("x", limit).max; got != core.MaxLogEntries {
			t.Errorf("Open(limit=%d).max = %d, want %d", limit, got, core.MaxLogEntries)
		}
	}
	if got := Open("x", 7).max; got != 7 {
		t.Errorf("max = %d, want 7", got)
	}
}

func TestAppendLoadRoundTripKeepsOrderAndFields(t *testing.T) {
	s, path := newStore(t, 10)
	want := []core.JobEntry{entry(1, core.JobCompleted), entry(2, core.JobFailed), entry(3, core.JobCancelled)}
	want[1].Reason = "Printer offline"
	mustAppend(t, s, want...)

	got, skipped, err := Open(path, 10).Load()
	if err != nil || skipped != 0 {
		t.Fatalf("Load = %v, skipped %d", err, skipped)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Load = %+v, want %+v", got, want)
	}
}

func TestPageCountReportedWhilePrintingIsRestoredFromHistory(t *testing.T) {
	s, path := newStore(t, 10)
	var log core.JobLog
	log.Observe(s.Observe)
	e := core.NewJobEntry("p1", "u", "a.pdf", 0)
	e.SetStatus(core.JobPrinting)
	log.Push(e)
	log.SetPages(e.ID, 12)
	log.Update(e.ID, core.JobCompleted, "")
	got, _, err := Open(path, 10).Load()
	if err != nil || len(got) != 1 || got[0].Pages != 12 {
		t.Fatalf("Load = %+v, %v, want the 12 pages kept", got, err)
	}
}

func TestEntriesWithoutAPageCountLoadAsUnknown(t *testing.T) {
	s, path := newStore(t, 10)
	mustAppend(t, s, entry(1, core.JobCompleted))
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintln(f, `{"id":"job-old","printer_id":"p1","status":"completed"}`)
	f.Close()
	got, skipped, err := Open(path, 10).Load()
	if err != nil || skipped != 0 || len(got) != 2 || got[1].ID != "job-old" || got[1].Pages != 0 {
		t.Fatalf("Load = %+v, skipped %d, err %v, want the old entry with unknown pages", got, skipped, err)
	}
}

func TestOnlyFinishedEntriesAreStored(t *testing.T) {
	for _, status := range []core.JobStatus{core.JobPending, core.JobPrinting, "bogus", ""} {
		t.Run(string(status), func(t *testing.T) {
			s, path := newStore(t, 10)
			mustAppend(t, s, entry(1, status))
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unfinished entry created the file: %v", err)
			}
		})
	}
}

func TestLoadSkipsUnfinishedLinesAndCountsThem(t *testing.T) {
	s, path := newStore(t, 10)
	mustAppend(t, s, entry(1, core.JobCompleted))
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	fmt.Fprintln(f, `{"id":"job-9","status":"printing"}`)
	f.Close()
	got, skipped, err := Open(path, 10).Load()
	if err != nil || skipped != 1 || !reflect.DeepEqual(ids(got), []string{"job-1"}) {
		t.Fatalf("Load = %v, skipped %d, err %v", ids(got), skipped, err)
	}
}

func TestLoadKeepsNewestMax(t *testing.T) {
	s, path := newStore(t, 100)
	for n := 1; n <= 7; n++ {
		mustAppend(t, s, entry(n, core.JobCompleted))
	}
	got, _, err := Open(path, 3).Load()
	if err != nil || !reflect.DeepEqual(ids(got), idRange(5, 7)) {
		t.Fatalf("Load = %v, %v; want %v", ids(got), err, idRange(5, 7))
	}
}

func TestLoadToleratesDamagedLines(t *testing.T) {
	good := func(n int) string {
		return fmt.Sprintf(`{"id":"job-%d","printer_id":"p","user":"u","filename":"f","pages":1,"status":"completed","created_at":1}`, n)
	}
	for _, tc := range []struct {
		name        string
		content     string
		wantIDs     []string
		wantSkipped int
	}{
		{"empty file", "", nil, 0},
		{"blank lines only", "\n  \n\r\n", nil, 0},
		{"torn last line", good(1) + "\n" + good(2) + "\n" + good(3)[:20], []string{"job-1", "job-2"}, 1},
		{"corrupt middle line", good(1) + "\nnot json\n" + good(3) + "\n", []string{"job-1", "job-3"}, 1},
		{"blank lines and CRLF", good(1) + "\r\n\r\n" + good(2) + "\r\n", []string{"job-1", "job-2"}, 0},
		{"json of wrong shape", "[1,2]\n\"x\"\n" + good(1) + "\n", []string{"job-1"}, 2},
		{"unterminated valid last line", good(1) + "\n" + good(2), []string{"job-1", "job-2"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, path := newStore(t, 10)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			got, skipped, err := s.Load()
			if err != nil || skipped != tc.wantSkipped || !reflect.DeepEqual(ids(got), tc.wantIDs) {
				t.Fatalf("Load = %v, skipped %d, err %v; want %v, %d", ids(got), skipped, err, tc.wantIDs, tc.wantSkipped)
			}
		})
	}
}

func TestAppendAfterTornTailKeepsNewEntryReadable(t *testing.T) {
	for _, loadFirst := range []bool{true, false} {
		t.Run(fmt.Sprintf("loadFirst=%t", loadFirst), func(t *testing.T) {
			s, path := newStore(t, 10)
			mustAppend(t, s, entry(1, core.JobCompleted))
			f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
			f.WriteString(`{"id":"job-2","stat`)
			f.Close()

			s = Open(path, 10)
			if loadFirst {
				if _, skipped, _ := s.Load(); skipped != 1 {
					t.Fatalf("skipped = %d, want 1", skipped)
				}
			}
			mustAppend(t, s, entry(3, core.JobFailed))
			got, skipped, err := Open(path, 10).Load()
			if err != nil || skipped != 1 || !reflect.DeepEqual(ids(got), []string{"job-1", "job-3"}) {
				t.Fatalf("Load = %v, skipped %d, err %v", ids(got), skipped, err)
			}
		})
	}
}

func TestLoadMissingFileIsEmptyNotError(t *testing.T) {
	s, _ := newStore(t, 10)
	got, skipped, err := s.Load()
	if err != nil || skipped != 0 || len(got) != 0 {
		t.Fatalf("Load = %v, %d, %v", got, skipped, err)
	}
}

func TestLoadReportsUnreadablePath(t *testing.T) {
	dir := t.TempDir() // a directory where the file should be
	if _, _, err := Open(dir, 10).Load(); err == nil {
		t.Fatal("Load of a directory succeeded")
	}
}

func TestCompactionKeepsNewestMaxAndLeavesValidFile(t *testing.T) {
	const limit = 3
	s, path := newStore(t, limit)
	total := 2*limit + 1
	for n := 1; n < total; n++ {
		mustAppend(t, s, entry(n, core.JobCompleted))
	}
	if got := len(readLines(t, path)); got != 2*limit {
		t.Fatalf("lines before threshold = %d, want %d (no compaction yet)", got, 2*limit)
	}
	mustAppend(t, s, entry(total, core.JobCompleted))

	if got := readLines(t, path); len(got) != limit {
		t.Fatalf("lines after compaction = %d, want %d", len(got), limit)
	}
	got, skipped, err := Open(path, limit).Load()
	if err != nil || skipped != 0 || !reflect.DeepEqual(ids(got), idRange(total-limit+1, total)) {
		t.Fatalf("Load = %v, skipped %d, err %v", ids(got), skipped, err)
	}
	files, _ := os.ReadDir(filepath.Dir(path))
	if len(files) != 1 {
		t.Fatalf("directory holds %d files after compaction, want only the history", len(files))
	}
	mustAppend(t, s, entry(total+1, core.JobFailed))
	if got := readLines(t, path); len(got) != limit+1 {
		t.Fatalf("lines after appending to a compacted file = %d, want %d", len(got), limit+1)
	}
}

func TestCompactionCountsLinesOfAnUnloadedFile(t *testing.T) {
	const limit = 2
	s, path := newStore(t, limit)
	for n := 1; n <= 2*limit; n++ {
		mustAppend(t, s, entry(n, core.JobCompleted))
	}
	fresh := Open(path, limit) // Load never called: lines are counted lazily
	mustAppend(t, fresh, entry(5, core.JobCompleted))
	if got := readLines(t, path); len(got) != limit {
		t.Fatalf("lines = %d, want %d", len(got), limit)
	}
}

func TestClear(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		s, _ := newStore(t, 10)
		if err := s.Clear(); err != nil {
			t.Fatalf("Clear = %v", err)
		}
	})
	t.Run("existing file", func(t *testing.T) {
		s, path := newStore(t, 10)
		mustAppend(t, s, entry(1, core.JobCompleted), entry(2, core.JobFailed))
		if err := s.Clear(); err != nil {
			t.Fatalf("Clear = %v", err)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("file survived Clear: %v", err)
		}
		if got, _, _ := s.Load(); len(got) != 0 {
			t.Fatalf("Load after Clear = %v", ids(got))
		}
		mustAppend(t, s, entry(3, core.JobCompleted))
		if got, _, _ := s.Load(); !reflect.DeepEqual(ids(got), []string{"job-3"}) {
			t.Fatalf("Load after Clear+Append = %v", ids(got))
		}
	})
}

func TestClearResetsCompactionCount(t *testing.T) {
	const limit = 2
	s, path := newStore(t, limit)
	for n := 1; n <= 2*limit; n++ {
		mustAppend(t, s, entry(n, core.JobCompleted))
	}
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	for n := 1; n <= 2*limit; n++ {
		mustAppend(t, s, entry(n, core.JobCompleted))
	}
	if got := len(readLines(t, path)); got != 2*limit {
		t.Fatalf("lines = %d, want %d: the count survived Clear", got, 2*limit)
	}
}

func TestConcurrentAppendLeavesValidLines(t *testing.T) {
	const writers, perWriter, limit = 8, 40, 50
	s, path := newStore(t, limit)
	var wg sync.WaitGroup
	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				if err := s.Append(entry(w*1000+i, core.JobCompleted)); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	got, skipped, err := Open(path, 1000).Load()
	if err != nil || skipped != 0 {
		t.Fatalf("Load = %d entries, skipped %d, err %v", len(got), skipped, err)
	}
	if len(got) < limit || len(got) > 2*limit {
		t.Fatalf("stored %d entries, want between %d and %d", len(got), limit, 2*limit)
	}
}

func TestObserveReportsErrorsToOnError(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s := Open(filepath.Join(blocker, "sub", FileName), 10) // parent is a file
	s.Observe(entry(1, core.JobCompleted))                 // default OnError ignores

	var errs []error
	s.OnError = func(err error) { errs = append(errs, err) }
	s.Observe(entry(2, core.JobFailed))
	s.Observe(entry(3, core.JobPending)) // unfinished: no write, no error
	if len(errs) != 1 {
		t.Fatalf("OnError calls = %d (%v), want 1", len(errs), errs)
	}
}

func TestObserveAppendsAsJobLogObserver(t *testing.T) {
	s, path := newStore(t, 10)
	var log core.JobLog
	log.Observe(s.Observe)
	e := core.NewJobEntry("p1", "u", "a.pdf", 1)
	e.SetStatus(core.JobPrinting)
	log.Push(e)
	log.Update(e.ID, core.JobFailed, "boom")
	got, _, err := Open(path, 10).Load()
	if err != nil || len(got) != 1 || got[0].Status != core.JobFailed || got[0].Reason != "boom" {
		t.Fatalf("Load = %+v, %v", got, err)
	}
}

func TestRestoreAfterClearWritesTheEntriesBackInOrder(t *testing.T) {
	s, path := newStore(t, 10)
	cleared := []core.JobEntry{entry(1, core.JobCompleted), entry(2, core.JobFailed), entry(3, core.JobCancelled)}
	mustAppend(t, s, cleared...)
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	if err := s.Restore(cleared); err != nil {
		t.Fatalf("Restore = %v", err)
	}
	got, skipped, err := s.Load()
	if err != nil || skipped != 0 || !reflect.DeepEqual(got, cleared) {
		t.Fatalf("Load = %v, skipped %d, err %v; want the cleared entries in order", ids(got), skipped, err)
	}
	if lines := readLines(t, path); len(lines) != 3 {
		t.Fatalf("file has %d lines, want 3", len(lines))
	}
}

func TestRestorePlacesEntriesBeforeWhatWasRecordedSince(t *testing.T) {
	s, _ := newStore(t, 10)
	mustAppend(t, s, entry(5, core.JobCompleted))
	if err := s.Restore([]core.JobEntry{entry(1, core.JobCompleted), entry(2, core.JobFailed)}); err != nil {
		t.Fatal(err)
	}
	got, _, _ := s.Load()
	if want := []string{"job-1", "job-2", "job-5"}; !reflect.DeepEqual(ids(got), want) {
		t.Fatalf("Load = %v, want %v", ids(got), want)
	}
	mustAppend(t, s, entry(6, core.JobCompleted)) // appending still works, in order
	if got, _, _ := s.Load(); !reflect.DeepEqual(ids(got), []string{"job-1", "job-2", "job-5", "job-6"}) {
		t.Fatalf("Load after Append = %v", ids(got))
	}
}

func TestRestoreIsIdempotentAndSkipsUnfinishedEntries(t *testing.T) {
	s, _ := newStore(t, 10)
	batch := []core.JobEntry{entry(1, core.JobCompleted), entry(2, core.JobPending)}
	for range 2 {
		if err := s.Restore(batch); err != nil {
			t.Fatal(err)
		}
	}
	if got, _, _ := s.Load(); !reflect.DeepEqual(ids(got), []string{"job-1"}) {
		t.Fatalf("Load = %v, want job-1 once", ids(got))
	}
	empty, emptyPath := newStore(t, 10)
	if err := empty.Restore(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(emptyPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("restoring nothing created the file: %v", err)
	}
}

func TestRestoreKeepsOnlyTheNewestMax(t *testing.T) {
	s, _ := newStore(t, 3)
	mustAppend(t, s, entry(5, core.JobCompleted))
	if err := s.Restore([]core.JobEntry{entry(1, core.JobCompleted), entry(2, core.JobCompleted), entry(3, core.JobCompleted)}); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := s.Load(); !reflect.DeepEqual(ids(got), []string{"job-2", "job-3", "job-5"}) {
		t.Fatalf("Load = %v", ids(got))
	}
}

func TestRestoreReportsAnUnwritablePathAndKeepsTheHistory(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := Open(filepath.Join(blocker, FileName), 10) // its directory is a file
	if err := s.Restore([]core.JobEntry{entry(1, core.JobCompleted)}); err == nil {
		t.Fatal("Restore into an unwritable path succeeded")
	}
}
