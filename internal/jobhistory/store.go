package jobhistory

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// FileName is the conventional name of the history file.
const FileName = "jobs.jsonl"

// File layout and compaction policy: the file may hold up to
// compactFactor*max lines before it is rewritten down to max.
const (
	dirPerm       fs.FileMode = 0o755
	filePerm      fs.FileMode = 0o644
	compactFactor             = 2
	lineEnd                   = '\n'
)

// Store is the JSON Lines history file at one path. Methods are safe for
// concurrent use. The zero value is not usable; construct it with Open.
type Store struct {
	// OnError, when set before the store is shared, receives the errors
	// Observe cannot return. nil ignores them.
	OnError func(error)

	path string
	max  int

	mu       sync.Mutex
	scanned  bool // lines and tornTail reflect the file
	lines    int  // non-blank lines currently in the file
	tornTail bool // the file ends without a line terminator
}

// Open returns the store for path, keeping at most limit entries (a value
// below 1 means core.MaxLogEntries). It performs no I/O.
func Open(path string, limit int) *Store {
	if limit < 1 {
		limit = core.MaxLogEntries
	}
	return &Store{path: path, max: limit}
}

// Load returns the newest max finished entries, oldest first. Blank lines
// are ignored; lines that are torn (e.g. by a crash), corrupt or not a
// finished job are skipped and counted. A missing file is an empty history.
func (s *Store) Load() (entries []core.JobEntry, skipped int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

// Append writes one finished entry as a new line. Unfinished entries are
// ignored. When the file grows past twice max lines it is compacted to the
// newest max entries; a compaction failure is returned but the entry is
// already stored.
func (s *Store) Append(entry core.JobEntry) error {
	if !entry.Status.Finished() {
		return nil
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureScanned(); err != nil {
		return err
	}
	if err := s.writeLine(line); err != nil {
		return err
	}
	if s.lines > compactFactor*s.max {
		return s.compactLocked()
	}
	return nil
}

// Clear removes the stored history. A missing file is not an error.
func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(s.path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	s.scanned, s.lines, s.tornTail = true, 0, false
	return nil
}

// Restore writes entries (oldest first) back into the history, before
// whatever it holds now: they are older than anything recorded since they
// were cleared. It is how an undone Clear Finished returns to disk. Entries
// that are not finished or whose id is already stored are skipped, so
// restoring twice changes nothing, and only the newest max entries stay.
// A missing or cleared file is fine: it is created. The file is replaced
// atomically, so a failure leaves the stored history as it was.
func (s *Store) Restore(entries []core.JobEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, _, err := s.loadLocked()
	if err != nil {
		return fmt.Errorf("restore: %w", err)
	}
	merged := mergeRestored(entries, current, s.max)
	if len(merged) == len(current) {
		return nil // nothing new to write
	}
	data, err := encodeLines(merged)
	if err != nil {
		return fmt.Errorf("restore: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), dirPerm); err != nil {
		return fmt.Errorf("restore: %w", err)
	}
	if err := core.WriteFileAtomic(s.path, data, filePerm); err != nil {
		return fmt.Errorf("restore: %w", err)
	}
	s.scanned, s.lines, s.tornTail = true, len(merged), false
	return nil
}

// mergeRestored is the finished restored entries not already in current,
// followed by current, trimmed to the newest max.
func mergeRestored(restored, current []core.JobEntry, max int) []core.JobEntry {
	have := make(map[string]bool, len(current))
	for _, e := range current {
		have[e.ID] = true
	}
	merged := make([]core.JobEntry, 0, len(restored)+len(current))
	for _, e := range restored {
		if e.Status.Finished() && !have[e.ID] {
			have[e.ID] = true
			merged = append(merged, e)
		}
	}
	merged = append(merged, current...)
	if len(merged) > max {
		merged = merged[len(merged)-max:]
	}
	return merged
}

// encodeLines is entries as JSON Lines, each terminated.
func encodeLines(entries []core.JobEntry) ([]byte, error) {
	var data []byte
	for _, e := range entries {
		line, err := json.Marshal(e)
		if err != nil {
			return nil, err
		}
		data = append(append(data, line...), lineEnd)
	}
	return data, nil
}

// Observe is a core.JobLog finished-job observer: it appends entry and
// reports a failure to OnError.
func (s *Store) Observe(entry core.JobEntry) {
	if err := s.Append(entry); err != nil && s.OnError != nil {
		s.OnError(err)
	}
}

// writeLine appends line plus its terminator in one write. A torn tail left
// by a crash is first closed off so it cannot swallow the new entry.
func (s *Store) writeLine(line []byte) error {
	if err := os.MkdirAll(filepath.Dir(s.path), dirPerm); err != nil {
		return err
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, filePerm)
	if err != nil {
		return err
	}
	if s.tornTail {
		line = append([]byte{lineEnd}, line...)
	}
	if _, err := f.Write(append(line, lineEnd)); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	s.lines++
	s.tornTail = false
	return nil
}

// compactLocked rewrites the file with its newest max valid entries.
func (s *Store) compactLocked() error {
	entries, _, err := s.loadLocked()
	if err != nil {
		return fmt.Errorf("compact: %w", err)
	}
	data, err := encodeLines(entries)
	if err != nil {
		return fmt.Errorf("compact: %w", err)
	}
	if err := core.WriteFileAtomic(s.path, data, filePerm); err != nil {
		return fmt.Errorf("compact: %w", err)
	}
	s.lines, s.tornTail = len(entries), false
	return nil
}
