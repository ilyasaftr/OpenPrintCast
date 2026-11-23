package applog

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/ilyasaftr/OpenPrintCast/internal/logfile"
)

// Options configures Init.
type Options struct {
	Dir     string           // log directory; "" -> DefaultDir()
	Stderr  io.Writer        // optional mirror, default os.Stderr; write errors are ignored
	Verbose bool             // initial level: Debug when true, else Info
	Now     func() time.Time // forwarded to logfile (tests)
}

// fileTuning overrides logfile limits; zero values keep the logfile defaults.
type fileTuning struct {
	maxBytes int64
	keep     int
}

// Logger is the handle of the installed process logger.
type Logger struct {
	file    *logfile.Writer
	level   *slog.LevelVar
	handler slog.Handler

	closeOnce sync.Once
	closeErr  error
}

// Init opens the rotating log file, installs a slog text handler writing to
// it (and to the stderr mirror) as the default logger and returns its handle.
// The stdlib log package is routed through the same handler at Info level.
func Init(opts Options) (*Logger, error) {
	return initLogger(opts, fileTuning{})
}

func initLogger(opts Options, tune fileTuning) (*Logger, error) {
	file, err := openFile(opts, tune)
	if err != nil {
		return nil, err
	}
	l := &Logger{file: file, level: new(slog.LevelVar)}
	l.SetVerbose(opts.Verbose)
	out := io.MultiWriter(file, tolerant(mirrorOf(opts)))
	l.handler = newHandler(out, l.level)
	slog.SetDefault(slog.New(l.handler))
	return l, nil
}

func mirrorOf(opts Options) io.Writer {
	if opts.Stderr != nil {
		return opts.Stderr
	}
	return os.Stderr
}

// openFile opens the rotating file in opts.Dir, falling back to the temp dir.
func openFile(opts Options, tune fileTuning) (*logfile.Writer, error) {
	dir := opts.Dir
	if dir == "" {
		dir = DefaultDir()
	}
	w, err := logfile.Open(fileOptions(dir, opts.Now, tune))
	if err == nil {
		return w, nil
	}
	fallback := fallbackDir()
	if fallback == dir {
		return nil, err
	}
	w, fbErr := logfile.Open(fileOptions(fallback, opts.Now, tune))
	if fbErr != nil {
		return nil, errors.Join(err, fbErr)
	}
	return w, nil
}

func fileOptions(dir string, now func() time.Time, tune fileTuning) logfile.Options {
	return logfile.Options{Dir: dir, Now: now, MaxBytes: tune.maxBytes, Keep: tune.keep}
}

// CurrentPath returns the log file currently being written.
func (l *Logger) CurrentPath() string { return l.file.CurrentPath() }

// SetVerbose switches the level between Debug (true) and Info (false) live.
func (l *Logger) SetVerbose(verbose bool) {
	if verbose {
		l.level.Set(slog.LevelDebug)
		return
	}
	l.level.Set(slog.LevelInfo)
}

// Verbose reports whether Debug records are currently logged.
func (l *Logger) Verbose() bool { return l.level.Level() <= slog.LevelDebug }

// Close restores a stderr-only default logger and closes the file. It is
// idempotent. Another handler installed in the meantime is left untouched.
func (l *Logger) Close() error {
	l.closeOnce.Do(func() {
		if slog.Default().Handler() == l.handler {
			restoreDefault()
		}
		l.closeErr = l.file.Close()
	})
	return l.closeErr
}

func restoreDefault() {
	slog.SetDefault(slog.New(newHandler(tolerant(os.Stderr), slog.LevelInfo)))
}
