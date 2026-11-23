package winspool

import "fmt"

// ErrorKind classifies a BackendError.
type ErrorKind string

// Backend error kinds, mirroring the Rust BackendError variants.
const (
	// KindUnsupported marks an operation with no meaning on this
	// platform (returned by every stub off Windows).
	KindUnsupported ErrorKind = "unsupported"
	// KindNotFound marks a request naming no installed printer.
	KindNotFound ErrorKind = "not_found"
	// KindSpool marks a job that reached the spooler but could not be
	// queued, or a document buffer the spooler cannot accept.
	KindSpool ErrorKind = "spool"
	// KindPlatform marks a failed Win32 spooler call.
	KindPlatform ErrorKind = "platform"
)

// BackendError is a failure of the Windows spool layer.
type BackendError struct {
	Kind ErrorKind
	// Printer names the target printer for KindNotFound and KindSpool.
	Printer string
	// Call names the failed Win32 call for KindPlatform.
	Call string
	// Reason carries human-readable detail for every kind.
	Reason string
}

// Error implements the error interface.
func (e *BackendError) Error() string {
	switch e.Kind {
	case KindNotFound:
		return fmt.Sprintf("printer not found: %s", e.Reason)
	case KindSpool:
		return fmt.Sprintf("spool failed for printer '%s': %s", e.Printer, e.Reason)
	case KindPlatform:
		return fmt.Sprintf("windows spooler call '%s' failed: %s", e.Call, e.Reason)
	default:
		return fmt.Sprintf("unsupported on this platform: %s", e.Reason)
	}
}

// UnsupportedError builds a KindUnsupported error.
func UnsupportedError(reason string) *BackendError {
	return &BackendError{Kind: KindUnsupported, Reason: reason}
}

// NotFoundError builds a KindNotFound error for printer.
func NotFoundError(printer string) *BackendError {
	return &BackendError{Kind: KindNotFound, Printer: printer, Reason: printer}
}

// SpoolError builds a KindSpool error for printer.
func SpoolError(printer, reason string) *BackendError {
	return &BackendError{Kind: KindSpool, Printer: printer, Reason: reason}
}

// PlatformError builds a KindPlatform error for a failed Win32 call.
func PlatformError(call, reason string) *BackendError {
	return &BackendError{Kind: KindPlatform, Call: call, Reason: reason}
}
