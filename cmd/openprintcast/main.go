package main

import (
	"os"

	"github.com/egoist/mygo"

	"github.com/ilyasaftr/OpenPrintCast/internal/applog"
)

func main() {
	// The elevated firewall step comes before the lock: it must not be
	// turned away by the running instance, and it never starts the UI.
	if handled, code := runCommandLine(os.Args[1:]); handled {
		os.Exit(code)
	}
	// The lock comes before everything else, so a second launch never
	// touches the log or the firewall; mygo tells the running instance.
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	// Logging next so every later failure lands in the log file.
	logs := startLogging()
	// Last hook of the quit sequence, also run when Windows ends the session.
	mygo.App.OnQuit(func() { _ = logs.Close() })
	componentLogger(componentApp).Info("openprintcast starting", "version", version, "log_path", logs.CurrentPath())
	if err := newBridgeApp(logs).run(); err != nil {
		componentLogger(componentApp).Error("app exited with error", "err", err)
		// os.Exit skips deferred calls and the quit hook, so close the log.
		_ = logs.Close()
		os.Exit(1)
	}
}

// startLogging installs the process logger. When it cannot start, the app
// keeps running on the default logger and gets a logger that does nothing.
func startLogging() loggerHandle {
	logs, err := applog.Init(applog.Options{})
	if err != nil {
		componentLogger(componentApp).Warn("file logging unavailable", "err", err)
		return nopLogControl{}
	}
	return logs
}
