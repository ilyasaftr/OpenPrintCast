package net

import (
	"context"

	"github.com/ilyasaftr/OpenPrintCast/internal/core"
)

// Spooler renders an accepted document through the OS print path. It is
// an interface (not the concrete Windows backend) so the IPP handler
// stays unit-testable and the app wires in the real spooler at startup.
type Spooler interface {
	// SpoolBuffer spools data (already in mime format) to a named printer.
	SpoolBuffer(printer string, data []byte, mime string) error
}

type cancellableSpooler interface {
	SpoolJobContext(context.Context, string, []byte, string, core.PrintOptions) error
}
