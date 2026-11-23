package main

import (
	"sync"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// fakeLogControl records verbosity changes and serves a settable path.
type fakeLogControl struct {
	mu      sync.Mutex
	path    string
	verbose []bool
}

func (f *fakeLogControl) SetVerbose(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.verbose = append(f.verbose, v)
}

func (f *fakeLogControl) CurrentPath() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.path
}

func (f *fakeLogControl) setPath(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.path = path
}

func (f *fakeLogControl) calls() []bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]bool(nil), f.verbose...)
}

// fakeStartupItem is a login item that remembers its state and can fail.
type fakeStartupItem struct {
	enabled bool
	err     error
	calls   []bool
}

func (f *fakeStartupItem) Enabled() bool { return f.enabled }

func (f *fakeStartupItem) SetEnabled(on bool) error {
	f.calls = append(f.calls, on)
	if f.err != nil {
		return f.err
	}
	f.enabled = on
	return nil
}

// openRecorder is a pathOpener that records the paths it was given.
type openRecorder struct {
	paths []string
	err   error
}

func (r *openRecorder) open(path string) error {
	r.paths = append(r.paths, path)
	return r.err
}

// fakeHistoryStore is a historyStore that records what it is told and can
// fail Clear.
type fakeHistoryStore struct {
	mu       sync.Mutex
	observed []string
	clears   int
	clearErr error

	restored   [][]string
	restoreErr error
}

func (f *fakeHistoryStore) Observe(entry core.JobEntry) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.observed = append(f.observed, entry.ID)
}

func (f *fakeHistoryStore) Clear() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clears++
	return f.clearErr
}

func (f *fakeHistoryStore) Restore(entries []core.JobEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.ID
	}
	f.restored = append(f.restored, ids)
	return f.restoreErr
}

func (f *fakeHistoryStore) restores() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]string(nil), f.restored...)
}

func (f *fakeHistoryStore) state() (observed []string, clears int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.observed...), f.clears
}
