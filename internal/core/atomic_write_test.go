package core

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

var errInjected = errors.New("injected failure")

// failingTempFile wraps a real temp file and fails the step named by failAt.
type failingTempFile struct {
	*os.File
	failAt string
}

func (f failingTempFile) Write(p []byte) (int, error) {
	if f.failAt == "write" {
		return 0, errInjected
	}
	return f.File.Write(p)
}

func (f failingTempFile) Chmod(mode os.FileMode) error {
	if f.failAt == "chmod" {
		return errInjected
	}
	return f.File.Chmod(mode)
}

func (f failingTempFile) Sync() error {
	if f.failAt == "sync" {
		return errInjected
	}
	return f.File.Sync()
}

func (f failingTempFile) Close() error {
	err := f.File.Close()
	if f.failAt == "close" {
		return errInjected
	}
	return err
}

// testWriter returns a writer on the real filesystem that fails at step and
// never really sleeps. Steps: "create", "write", "chmod", "sync", "close",
// "replace"; "" means no failure.
func testWriter(step string) atomicWriter {
	w := defaultAtomicWriter()
	w.sleep = func(time.Duration) {}
	w.isRetry = func(error) bool { return false }
	w.createTemp = func(dir, pattern string) (tempFile, error) {
		if step == "create" {
			return nil, errInjected
		}
		f, err := os.CreateTemp(dir, pattern)
		return failingTempFile{File: f, failAt: step}, err
	}
	if step == "replace" {
		w.replace = func(string, string) error { return errInjected }
	}
	return w
}

func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestWriteFileAtomicHappyPath(t *testing.T) {
	for _, existing := range []bool{false, true} {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.json")
		if existing {
			if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if err := writeFileAtomic(path, []byte("new content"), 0o644); err != nil {
			t.Fatalf("existing=%v: %v", existing, err)
		}
		if got := mustReadFile(t, path); got != "new content" {
			t.Fatalf("existing=%v: content %q", existing, got)
		}
		if names := dirEntries(t, dir); len(names) != 1 {
			t.Fatalf("existing=%v: stray files %v", existing, names)
		}
		if runtime.GOOS == "windows" {
			continue
		}
		if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o644 {
			t.Fatalf("existing=%v: perm %v err %v", existing, info.Mode().Perm(), err)
		}
	}
}

func TestAtomicWriterFailureLeavesOriginalAndNoTemp(t *testing.T) {
	for _, step := range []string{"create", "write", "chmod", "sync", "close", "replace"} {
		t.Run(step, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.json")
			if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
				t.Fatal(err)
			}
			err := testWriter(step).writeFile(path, []byte("replacement"), 0o644)
			if !errors.Is(err, errInjected) {
				t.Fatalf("err = %v, want injected", err)
			}
			if got := mustReadFile(t, path); got != "original" {
				t.Fatalf("original modified: %q", got)
			}
			if names := dirEntries(t, dir); len(names) != 1 {
				t.Fatalf("stray files %v", names)
			}
		})
	}
}

func TestAtomicWriterFailureWithoutExistingFileLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	err := testWriter("sync").writeFile(filepath.Join(dir, "config.json"), []byte("x"), 0o644)
	if !errors.Is(err, errInjected) {
		t.Fatalf("err = %v", err)
	}
	if names := dirEntries(t, dir); len(names) != 0 {
		t.Fatalf("stray files %v", names)
	}
}

func TestReplaceWithRetry(t *testing.T) {
	retryable := errors.New("sharing violation")
	maxCalls := int(replaceRetryTotal/replaceRetryStep) + 1
	tests := []struct {
		name       string
		failures   int // calls that fail before the replacer succeeds
		failErr    error
		wantCalls  int
		wantSleeps int
		wantErr    error
	}{
		{"succeeds first time", 0, retryable, 1, 0, nil},
		{"succeeds after retries", 3, retryable, 4, 3, nil},
		{"always failing gives up after total", 1 << 30, retryable, maxCalls, maxCalls - 1, retryable},
		{"non-retryable fails immediately", 1 << 30, errInjected, 1, 0, errInjected},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls, slept := 0, time.Duration(0)
			w := atomicWriter{
				replace: func(string, string) error {
					calls++
					if calls <= tc.failures {
						return tc.failErr
					}
					return nil
				},
				isRetry: func(err error) bool { return errors.Is(err, retryable) },
				sleep:   func(d time.Duration) { slept += d },
			}
			err := w.replaceWithRetry("src", "dst")
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if calls != tc.wantCalls || slept != time.Duration(tc.wantSleeps)*replaceRetryStep {
				t.Fatalf("calls=%d slept=%v, want calls=%d sleeps=%d", calls, slept, tc.wantCalls, tc.wantSleeps)
			}
			if tc.wantSleeps == maxCalls-1 && slept != replaceRetryTotal {
				t.Fatalf("total wait %v, want %v", slept, replaceRetryTotal)
			}
		})
	}
}

func TestAtomicWriterRetriesThenRemovesTempOnFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := testWriter("")
	calls := 0
	w.replace = func(string, string) error { calls++; return errInjected }
	w.isRetry = func(error) bool { return true }
	if err := w.writeFile(path, []byte("new"), 0o644); !errors.Is(err, errInjected) {
		t.Fatalf("err = %v", err)
	}
	if calls < 2 {
		t.Fatalf("replacer called %d times, want retries", calls)
	}
	if got := mustReadFile(t, path); got != "original" || len(dirEntries(t, dir)) != 1 {
		t.Fatalf("original %q entries %v", got, dirEntries(t, dir))
	}
}

func TestWriteFileAtomicExportedWrapper(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.jsonl")
	for _, content := range []string{"first\n", "second\n"} {
		if err := WriteFileAtomic(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != content {
			t.Fatalf("content = %q, %v; want %q", got, err, content)
		}
	}
	if files, _ := os.ReadDir(filepath.Dir(path)); len(files) != 1 {
		t.Fatalf("temp files left behind: %v", files)
	}
}
