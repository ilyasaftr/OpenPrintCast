package logfile

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func newClock() *fakeClock {
	return &fakeClock{t: time.Date(2024, time.March, 9, 10, 0, 0, 0, time.Local)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

type errorLog struct {
	mu   sync.Mutex
	errs []error
}

func (l *errorLog) add(err error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errs = append(l.errs, err)
}

func (l *errorLog) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.errs)
}

func openTest(t *testing.T, opts Options) *Writer {
	t.Helper()
	if opts.Dir == "" {
		opts.Dir = t.TempDir()
	}
	if opts.Prefix == "" {
		opts.Prefix = "app"
	}
	w, err := Open(opts)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { w.Close() })
	return w
}

func mustWrite(t *testing.T, w *Writer, s string) {
	t.Helper()
	n, err := w.Write([]byte(s))
	if err != nil || n != len(s) {
		t.Fatalf("Write(%q) = %d, %v", s, n, err)
	}
}

func dirNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func assertNames(t *testing.T, dir string, want ...string) {
	t.Helper()
	sort.Strings(want)
	if got := dirNames(t, dir); !slices.Equal(got, want) {
		t.Fatalf("files = %v, want %v", got, want)
	}
}

func TestOpenCreatesNestedDirAndFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b")
	clock := newClock()
	w := openTest(t, Options{Dir: dir, Now: clock.Now})
	want := filepath.Join(dir, "app-20240309.log")
	if w.CurrentPath() != want {
		t.Fatalf("CurrentPath = %q, want %q", w.CurrentPath(), want)
	}
	assertNames(t, dir, "app-20240309.log")
}

func TestOpenRequiresDir(t *testing.T) {
	if _, err := Open(Options{}); err == nil {
		t.Fatal("expected error for empty Dir")
	}
}

func TestOpenFailsWhenNoFileCanBeOpened(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(Options{Dir: filepath.Join(blocker, "sub")}); err == nil {
		t.Fatal("expected error when Dir cannot be created")
	}
}

func TestDefaultsApplied(t *testing.T) {
	o := Options{Dir: "x"}.withDefaults()
	if o.Prefix != DefaultPrefix || o.MaxBytes != DefaultMaxBytes || o.Keep != DefaultKeep || o.Now == nil {
		t.Fatalf("defaults not applied: %+v", o)
	}
	o.OnError(errors.New("ignored"))
}

func TestRolloverBySize(t *testing.T) {
	dir := t.TempDir()
	w := openTest(t, Options{Dir: dir, MaxBytes: 10, Now: newClock().Now})
	for _, s := range []string{"aaa\n", "bbb\n", "ccc\n", "ddd\n", "eee\n"} {
		mustWrite(t, w, s)
	}
	assertNames(t, dir, "app-20240309.log", "app-20240309-2.log", "app-20240309-3.log")
	if got := readFile(t, filepath.Join(dir, "app-20240309.log")); got != "aaa\nbbb\n" {
		t.Fatalf("first file = %q", got)
	}
	if got := readFile(t, w.CurrentPath()); got != "eee\n" {
		t.Fatalf("current file = %q", got)
	}
}

func TestWriteFittingExactlyDoesNotRoll(t *testing.T) {
	dir := t.TempDir()
	w := openTest(t, Options{Dir: dir, MaxBytes: 8, Now: newClock().Now})
	mustWrite(t, w, "aaa\n")
	mustWrite(t, w, "bbb\n")
	assertNames(t, dir, "app-20240309.log")
}

func TestRolloverByDay(t *testing.T) {
	dir := t.TempDir()
	clock := newClock()
	w := openTest(t, Options{Dir: dir, Now: clock.Now})
	mustWrite(t, w, "day one\n")
	clock.Advance(24 * time.Hour)
	mustWrite(t, w, "day two\n")
	assertNames(t, dir, "app-20240309.log", "app-20240310.log")
	if got := readFile(t, w.CurrentPath()); got != "day two\n" {
		t.Fatalf("current = %q", got)
	}
}

func TestDayRolloverRestartsSequence(t *testing.T) {
	dir := t.TempDir()
	clock := newClock()
	w := openTest(t, Options{Dir: dir, MaxBytes: 4, Now: clock.Now})
	mustWrite(t, w, "aaa\n")
	mustWrite(t, w, "bbb\n")
	clock.Advance(24 * time.Hour)
	mustWrite(t, w, "ccc\n")
	assertNames(t, dir, "app-20240309.log", "app-20240309-2.log", "app-20240310.log")
}

func TestSequenceNumberingPastNineKeepsNumericOrder(t *testing.T) {
	dir := t.TempDir()
	w := openTest(t, Options{Dir: dir, MaxBytes: 4, Keep: 3, Now: newClock().Now})
	for i := 1; i <= 12; i++ {
		mustWrite(t, w, fmt.Sprintf("%03d\n", i))
	}
	assertNames(t, dir, "app-20240309-10.log", "app-20240309-11.log", "app-20240309-12.log")
}

func TestPruneKeepsNewestFiles(t *testing.T) {
	dir := t.TempDir()
	clock := newClock()
	w := openTest(t, Options{Dir: dir, Keep: 2, Now: clock.Now})
	for i := 0; i < 5; i++ {
		mustWrite(t, w, "x\n")
		clock.Advance(24 * time.Hour)
	}
	mustWrite(t, w, "x\n")
	assertNames(t, dir, "app-20240313.log", "app-20240314.log")
}

func TestPruneIgnoresForeignFiles(t *testing.T) {
	dir := t.TempDir()
	foreign := []string{"notes.txt", "other-20200101.log", "app-20200101.log.bak", "app-20200101-1.log", "app.log"}
	for _, name := range foreign {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("keep"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "app-20200101-7.log"), 0o755); err != nil {
		t.Fatal(err)
	}
	w := openTest(t, Options{Dir: dir, Keep: 1, MaxBytes: 4, Now: newClock().Now})
	mustWrite(t, w, "aaa\n")
	mustWrite(t, w, "bbb\n")
	want := append(foreign, "app-20200101-7.log", "app-20240309-2.log")
	assertNames(t, dir, want...)
}

func TestPruneRemovesOldFilesAtOpen(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"app-20240101.log", "app-20240102.log", "app-20240103.log"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	openTest(t, Options{Dir: dir, Keep: 2, Now: newClock().Now})
	assertNames(t, dir, "app-20240103.log", "app-20240309.log")
}

func TestPruneNeverDeletesCurrentFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app-20250101.log"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	w := openTest(t, Options{Dir: dir, Keep: 1, Now: newClock().Now})
	assertNames(t, dir, filepath.Base(w.CurrentPath()))
}

func TestFailedPruneIsReportedAndRetried(t *testing.T) {
	dir := t.TempDir()
	var errs errorLog
	w := openTest(t, Options{Dir: dir, MaxBytes: 4, Keep: 1, Now: newClock().Now, OnError: errs.add})
	failing := true
	w.remove = func(path string) error {
		if failing {
			return errors.New("locked")
		}
		return os.Remove(path)
	}
	mustWrite(t, w, "aaa\n")
	mustWrite(t, w, "bbb\n")
	if errs.count() != 1 {
		t.Fatalf("errors = %d, want 1", errs.count())
	}
	assertNames(t, dir, "app-20240309.log", "app-20240309-2.log")
	failing = false
	mustWrite(t, w, "ccc\n")
	assertNames(t, dir, "app-20240309-3.log")
}

func TestAppendAfterReopen(t *testing.T) {
	dir := t.TempDir()
	clock := newClock()
	opts := Options{Dir: dir, Now: clock.Now}
	w := openTest(t, opts)
	mustWrite(t, w, "first\n")
	w.Close()
	w2 := openTest(t, opts)
	mustWrite(t, w2, "second\n")
	assertNames(t, dir, "app-20240309.log")
	if got := readFile(t, w2.CurrentPath()); got != "first\nsecond\n" {
		t.Fatalf("content = %q", got)
	}
}

func TestReopenContinuesNewestSequence(t *testing.T) {
	dir := t.TempDir()
	opts := Options{Dir: dir, MaxBytes: 10, Now: newClock().Now}
	w := openTest(t, opts)
	for _, s := range []string{"aaa\n", "bbb\n", "ccc\n"} {
		mustWrite(t, w, s)
	}
	w.Close()
	w2 := openTest(t, opts)
	if filepath.Base(w2.CurrentPath()) != "app-20240309-2.log" {
		t.Fatalf("reopened %q", w2.CurrentPath())
	}
	mustWrite(t, w2, "ddd\n")
	if got := readFile(t, w2.CurrentPath()); got != "ccc\nddd\n" {
		t.Fatalf("content = %q", got)
	}
}

func TestReopenMovesOnWhenCurrentFileExceedsCap(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "app-20240309.log"), []byte("0123456789ABCDEF"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := openTest(t, Options{Dir: dir, MaxBytes: 10, Now: newClock().Now})
	if filepath.Base(w.CurrentPath()) != "app-20240309-2.log" {
		t.Fatalf("CurrentPath = %q", w.CurrentPath())
	}
	if got := readFile(t, filepath.Join(dir, "app-20240309.log")); got != "0123456789ABCDEF" {
		t.Fatalf("old file modified: %q", got)
	}
}

func TestOversizedWrite(t *testing.T) {
	dir := t.TempDir()
	w := openTest(t, Options{Dir: dir, MaxBytes: 5, Now: newClock().Now})
	big := strings.Repeat("x", 20) + "\n"
	mustWrite(t, w, big)
	assertNames(t, dir, "app-20240309.log")
	mustWrite(t, w, "hi\n")
	mustWrite(t, w, big)
	assertNames(t, dir, "app-20240309.log", "app-20240309-2.log", "app-20240309-3.log")
	if got := readFile(t, w.CurrentPath()); got != big {
		t.Fatalf("oversized write not intact: %q", got)
	}
}

func TestOpenFailureFallsBackAndRetriesOncePerMinute(t *testing.T) {
	dir := t.TempDir()
	clock := newClock()
	var errs errorLog
	w := openTest(t, Options{Dir: dir, MaxBytes: 4, Keep: 10, Now: clock.Now, OnError: errs.add})
	mustWrite(t, w, "aaa\n")
	blocker := filepath.Join(dir, "app-20240309-2.log")
	if err := os.Mkdir(blocker, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		mustWrite(t, w, "bbb\n")
	}
	if errs.count() != 1 {
		t.Fatalf("errors after first minute = %d, want 1", errs.count())
	}
	clock.Advance(retryInterval - time.Second)
	mustWrite(t, w, "bbb\n")
	if errs.count() != 1 {
		t.Fatalf("retried too early: %d errors", errs.count())
	}
	clock.Advance(2 * time.Second)
	mustWrite(t, w, "bbb\n")
	if errs.count() != 2 {
		t.Fatalf("errors after retry = %d, want 2", errs.count())
	}
	if filepath.Base(w.CurrentPath()) != "app-20240309.log" {
		t.Fatalf("fallback lost: %q", w.CurrentPath())
	}
	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	clock.Advance(retryInterval)
	mustWrite(t, w, "ccc\n")
	if filepath.Base(w.CurrentPath()) != "app-20240309-2.log" || errs.count() != 2 {
		t.Fatalf("recovery failed: %q, %d errors", w.CurrentPath(), errs.count())
	}
}

func TestCloseIsIdempotentAndBlocksWrites(t *testing.T) {
	w := openTest(t, Options{Now: newClock().Now})
	mustWrite(t, w, "x\n")
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if _, err := w.Write([]byte("y")); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("Write after Close = %v, want os.ErrClosed", err)
	}
	if w.CurrentPath() == "" {
		t.Fatal("CurrentPath should stay available after Close")
	}
}

func TestConcurrentWritersLoseNothing(t *testing.T) {
	const writers, perWriter = 8, 200
	dir := t.TempDir()
	w := openTest(t, Options{Dir: dir, MaxBytes: 512, Keep: 1000, Now: newClock().Now})
	var wg sync.WaitGroup
	for g := 0; g < writers; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				line := fmt.Sprintf("writer=%d line=%d %s\n", g, i, strings.Repeat("z", 20))
				if _, err := w.Write([]byte(line)); err != nil {
					t.Error(err)
					return
				}
			}
		}()
		_ = w.CurrentPath()
	}
	wg.Wait()
	w.Close()
	assertEveryLineOnce(t, dir, writers, perWriter)
}

func assertEveryLineOnce(t *testing.T, dir string, writers, perWriter int) {
	t.Helper()
	seen := map[string]int{}
	for _, name := range dirNames(t, dir) {
		data := readFile(t, filepath.Join(dir, name))
		if len(data) > 0 && !strings.HasSuffix(data, "\n") {
			t.Fatalf("%s ends with a torn line", name)
		}
		for _, line := range bytes.Split([]byte(data), []byte("\n")) {
			if len(line) > 0 {
				seen[string(line)]++
			}
		}
	}
	if len(seen) != writers*perWriter {
		t.Fatalf("distinct lines = %d, want %d", len(seen), writers*perWriter)
	}
	for line, n := range seen {
		if n != 1 || !strings.HasPrefix(line, "writer=") || !strings.HasSuffix(line, strings.Repeat("z", 20)) {
			t.Fatalf("line %q seen %d times", line, n)
		}
	}
}

func TestDirectoryContainsOnlyExpectedNames(t *testing.T) {
	dir := t.TempDir()
	clock := newClock()
	w := openTest(t, Options{Dir: dir, MaxBytes: 8, Keep: 4, Now: clock.Now})
	for i := 0; i < 40; i++ {
		mustWrite(t, w, "abc\n")
		if i%7 == 0 {
			clock.Advance(24 * time.Hour)
		}
	}
	names := dirNames(t, dir)
	if len(names) != 4 {
		t.Fatalf("files = %v, want 4", names)
	}
	for _, name := range names {
		if _, ok := parseName("app", name); !ok {
			t.Fatalf("unexpected (renamed?) file %q", name)
		}
	}
}
