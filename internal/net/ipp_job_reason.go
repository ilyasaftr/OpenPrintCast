package net

import (
	"strings"

	"github.com/OpenPrinting/goipp"
)

// Short user-facing reasons recorded on failed or cancelled jobs.
const (
	reasonPaused            = "Sharing was paused"
	reasonUnsupportedFormat = "Unsupported document format"
	reasonBadDocument       = "The document is not in the declared format"
	reasonUnsupportedValues = "Unsupported print options"
	reasonCompression       = "The document could not be decompressed"
	reasonNoBackend         = "No printer backend is available"
	reasonNotAccepting      = "Printer is not accepting jobs"
	reasonRejected          = "Print job was rejected"
	reasonSpoolFailed       = "Printing failed"
	reasonCancelled         = "Cancelled"

	maxReasonRunes = 160
	reasonEllipsis = "…"
)

// rejectionReason explains why a job was refused before spooling with
// status, whether at admission or while preparing it.
func rejectionReason(status goipp.Status) string {
	switch status {
	case goipp.StatusErrorBusy:
		return reasonPaused
	case goipp.StatusErrorNotAcceptingJobs:
		return reasonNotAccepting
	case goipp.StatusErrorDocumentFormatNotSupported:
		return reasonUnsupportedFormat
	case goipp.StatusErrorDocumentFormatError:
		return reasonBadDocument
	case goipp.StatusErrorCompressionError, goipp.StatusErrorCompressionNotSupported, goipp.StatusErrorRequestEntity:
		return reasonCompression
	case goipp.StatusErrorAttributesOrValues:
		return reasonUnsupportedValues
	case goipp.StatusErrorInternal:
		return reasonNoBackend
	}
	return reasonRejected
}

// spoolFailureReason turns a spooler error into a single line of at most
// maxReasonRunes runes.
func spoolFailureReason(err error) string {
	text := strings.Join(strings.Fields(err.Error()), " ")
	if text == "" {
		return reasonSpoolFailed
	}
	runes := []rune(text)
	if len(runes) <= maxReasonRunes {
		return text
	}
	return string(runes[:maxReasonRunes-1]) + reasonEllipsis
}
