package jobhistory

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// loadLocked reads the whole file, refreshing the cached line count and
// tail state, and returns the newest max finished entries oldest first.
func (s *Store) loadLocked() ([]core.JobEntry, int, error) {
	f, err := os.Open(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		s.scanned, s.lines, s.tornTail = true, 0, false
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	entries, skipped, lines, torn, err := readEntries(f)
	if err != nil {
		return nil, 0, err
	}
	s.scanned, s.lines, s.tornTail = true, lines, torn
	if len(entries) > s.max {
		entries = entries[len(entries)-s.max:]
	}
	return entries, skipped, nil
}

// ensureScanned counts the lines of a file Load has not been called on.
func (s *Store) ensureScanned() error {
	if s.scanned {
		return nil
	}
	_, _, err := s.loadLocked()
	return err
}

// readEntries parses every line of r. It returns the finished entries in
// file order, how many non-blank lines were skipped, the number of
// non-blank lines, and whether the input ends without a line terminator.
func readEntries(r io.Reader) (entries []core.JobEntry, skipped, lines int, torn bool, err error) {
	br := bufio.NewReader(r)
	for {
		raw, readErr := br.ReadBytes(lineEnd)
		if readErr != nil && readErr != io.EOF {
			return nil, 0, 0, false, readErr
		}
		torn = len(raw) > 0 && raw[len(raw)-1] != lineEnd
		if line := bytes.TrimSpace(raw); len(line) > 0 {
			lines++
			if entry, ok := parseEntry(line); ok {
				entries = append(entries, entry)
			} else {
				skipped++
			}
		}
		if readErr == io.EOF {
			return entries, skipped, lines, torn, nil
		}
	}
}

// parseEntry decodes one line, accepting only finished jobs.
func parseEntry(line []byte) (core.JobEntry, bool) {
	var entry core.JobEntry
	if json.Unmarshal(line, &entry) != nil || !entry.Status.Finished() {
		return core.JobEntry{}, false
	}
	return entry, true
}
