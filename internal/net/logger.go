package net

import "log/slog"

// Log components of this package, the value of the "component" attribute.
const (
	componentIPP       = "ipp"
	componentHTTP      = "http"
	componentSpool     = "spool"
	componentAdvertise = "advertise"
)

// componentLogger returns the default logger tagged with component. It is
// resolved on every call because the default logger is installed after
// package initialization.
func componentLogger(component string) *slog.Logger {
	return slog.Default().With("component", component)
}
