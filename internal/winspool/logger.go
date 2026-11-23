package winspool

import "log/slog"

// Log components of this package, the value of the "component" attribute.
const (
	componentSpool  = "spool"
	componentPDF    = "pdf"
	componentPDFium = "pdfium"
	// componentSupplies is the SNMP supply-level reader.
	componentSupplies = "supplies"
)

// componentLogger returns the default logger tagged with component. It is
// resolved on every call because the default logger is installed after
// package initialization.
func componentLogger(component string) *slog.Logger {
	return slog.Default().With("component", component)
}
