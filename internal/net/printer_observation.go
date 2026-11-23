package net

import (
	"sync"
	"time"
)

type printerObservation struct {
	config       string
	state        int
	reasons      string
	configChange time.Time
	stateChange  time.Time
}

// printerObservations remembers the last printer config and state seen per
// printer so change timestamps only move when something actually changed.
type printerObservations struct {
	mu       sync.Mutex
	observed map[string]printerObservation
}

func (o *printerObservations) observe(id, config string, state int, reasons string) printerObservation {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.observed == nil {
		o.observed = make(map[string]printerObservation)
	}
	last := o.observed[id]
	now := time.Now()
	if last.configChange.IsZero() || last.config != config {
		last.config, last.configChange = config, now
	}
	if last.stateChange.IsZero() || last.state != state || last.reasons != reasons {
		last.state, last.reasons, last.stateChange = state, reasons, now
	}
	o.observed[id] = last
	return last
}
