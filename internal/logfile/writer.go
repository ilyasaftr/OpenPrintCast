package logfile

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Defaults applied by Open for unset Options fields.
const (
	DefaultPrefix   = "openprintcast"
	DefaultMaxBytes = 5 << 20
	DefaultKeep     = 5
)

const (
	dirPerm       = 0o755
	filePerm      = 0o644
	retryInterval = time.Minute
)

// Options configures a Writer.
type Options struct {
	Dir      string           // required
	Prefix   string           // default DefaultPrefix
	MaxBytes int64            // default DefaultMaxBytes; one larger write still goes to a fresh file
	Keep     int              // newest files kept, default DefaultKeep (minimum 1)
	Now      func() time.Time // clock, default time.Now
	OnError  func(error)      // non-fatal problems (rotate/prune); default ignore
}

func (o Options) withDefaults() Options {
	if o.Prefix == "" {
		o.Prefix = DefaultPrefix
	}
	if o.MaxBytes <= 0 {
		o.MaxBytes = DefaultMaxBytes
	}
	if o.Keep <= 0 {
		o.Keep = DefaultKeep
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.OnError == nil {
		o.OnError = func(error) {}
	}
	return o
}

// Writer is a goroutine-safe io.WriteCloser over date-stamped log files.
type Writer struct {
	opts   Options
	remove func(path string) error // seam for tests

	mu      sync.Mutex
	file    *os.File
	name    logName
	size    int64
	closed  bool
	retryAt time.Time // no rollover attempt before this instant after a failure
}

// Open creates Dir and opens (or creates) the current log file in append mode.
func Open(opts Options) (*Writer, error) {
	if opts.Dir == "" {
		return nil, errors.New("logfile: Dir is required")
	}
	opts = opts.withDefaults()
	if err := os.MkdirAll(opts.Dir, dirPerm); err != nil {
		return nil, err
	}
	w := &Writer{opts: opts, remove: os.Remove}
	if err := w.switchTo(w.newestFor(dayOf(opts.Now()))); err != nil {
		return nil, err
	}
	w.prune()
	return w, nil
}

// Write appends p, first rolling over when the day changed or p would not fit.
func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, os.ErrClosed
	}
	w.rollIfNeeded(int64(len(p)))
	n, err := w.file.Write(p)
	w.size += int64(n)
	return n, err
}

// CurrentPath returns the path of the file being written.
func (w *Writer) CurrentPath() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.path(w.name)
}

// Close closes the current file; it is idempotent and later Writes fail with os.ErrClosed.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	return w.file.Close()
}

func (w *Writer) path(n logName) string {
	return filepath.Join(w.opts.Dir, formatName(w.opts.Prefix, n))
}
