package main

import (
	"io"

	"github.com/egoist/mygo"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// logControl is the part of the process logger the app needs: switching
// the verbosity and finding the file being written. *applog.Logger
// satisfies it; tests use a fake.
type logControl interface {
	SetVerbose(bool)
	CurrentPath() string
}

// loggerHandle is a logControl that can also be closed when the app quits.
type loggerHandle interface {
	logControl
	io.Closer
}

// nopLogControl is the logger used when file logging could not start, and
// by fixtures that never set one: it ignores changes and has no file.
type nopLogControl struct{}

func (nopLogControl) SetVerbose(bool)     {}
func (nopLogControl) CurrentPath() string { return "" }
func (nopLogControl) Close() error        { return nil }

// pathOpener opens a file or folder in the user's default app.
type pathOpener func(path string) error

// shellOpenPath is the production pathOpener: the system shell.
func shellOpenPath(path string) error { return mygo.Shell.OpenPath(path) }

// logger is the log control, never nil.
func (a *bridgeApp) logger() logControl {
	if a.logs == nil {
		return nopLogControl{}
	}
	return a.logs
}

// openLog opens the log file in use right now, which changes as the log
// rolls over; a failure is logged.
func (a *bridgeApp) openLog() {
	path := a.logger().CurrentPath()
	if path == "" {
		componentLogger(componentApp).Warn("open log file: no log file in use")
		return
	}
	open := a.openPath
	if open == nil {
		open = shellOpenPath
	}
	if err := open(path); err != nil {
		componentLogger(componentApp).Error("open log file failed", "path", path, "err", err)
	}
}

// applyLoggingConfig makes the logger follow the persisted verbosity; the
// config is loaded after logging starts, so this runs once right after.
func applyLoggingConfig(logs logControl, cfg core.AppConfig) {
	logs.SetVerbose(cfg.VerboseLogging)
}
