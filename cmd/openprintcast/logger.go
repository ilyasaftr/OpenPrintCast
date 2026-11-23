package main

import "log/slog"

// Log components of this package, the value of the "component" attribute.
const (
	componentApp      = "app"
	componentConfig   = "config"
	componentTray     = "tray"
	componentNotify   = "notify"
	componentFirewall = "firewall"
	componentIPP      = "ipp"
	componentHistory  = "history"
	componentUpdate   = "update"
)

// componentLogger returns the default logger tagged with component. It is
// resolved on every call because the default logger is installed after
// package initialization.
func componentLogger(component string) *slog.Logger {
	return slog.Default().With("component", component)
}
