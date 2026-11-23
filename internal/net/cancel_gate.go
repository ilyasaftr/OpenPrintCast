package net

import "sync/atomic"

const (
	gateOpen int32 = iota
	gateCancelled
	gateFinished
)

// cancelGate arbitrates between a cancel request and the job finishing:
// whichever claims the gate first wins, so a cancel that reported success is
// never followed by a different outcome. The zero value is open.
type cancelGate struct{ state atomic.Int32 }

// cancel claims the gate for a cancel request.
func (g *cancelGate) cancel() bool {
	return g.state.CompareAndSwap(gateOpen, gateCancelled)
}

// finish claims the gate for the job's outcome and reports whether a cancel
// request had already won. It may be called repeatedly.
func (g *cancelGate) finish() (cancelled bool) {
	if g.state.CompareAndSwap(gateOpen, gateFinished) {
		return false
	}
	return g.state.Load() == gateCancelled
}
