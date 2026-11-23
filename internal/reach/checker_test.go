package reach

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeCollector struct {
	facts Facts
	err   error
	ctx   context.Context
}

func (f *fakeCollector) Collect(ctx context.Context) (Facts, error) {
	f.ctx = ctx
	return f.facts, f.err
}

func newChecker(c Collector, listen Listen) Checker {
	return Checker{Collector: c, Listen: func() Listen { return listen }, LocalIP: func() string { return "192.168.1.5" }, Port: 631}
}

func TestCheckerCollectsAndClassifies(t *testing.T) {
	collector := &fakeCollector{facts: Facts{Profiles: []ConnProfile{conn("Public")}, Firewall: firewallOn()}}
	got := newChecker(collector, Listen{Attempted: true, Listening: true}).Check(context.Background())
	if got.State != StateBlocked || got.Cause != CausePublicNetwork {
		t.Fatalf("got %v", got)
	}
	deadline, ok := collector.ctx.Deadline()
	if !ok || time.Until(deadline) > CollectTimeout {
		t.Fatalf("the collection has no %v limit", CollectTimeout)
	}
}

func TestCheckerTurnsFailuresIntoUnknown(t *testing.T) {
	for name, err := range map[string]error{
		"unavailable": ErrUnavailable,
		"timeout":     context.DeadlineExceeded,
		"policy":      errors.New("constrained language mode"),
	} {
		t.Run(name, func(t *testing.T) {
			got := newChecker(&fakeCollector{err: err}, Listen{Attempted: true, Listening: true}).Check(context.Background())
			if got.State != StateUnknown || got.Problem() || got.Detail == "" {
				t.Fatalf("got %v", got)
			}
		})
	}
}

func TestCheckerStillReportsTheServersOwnFailure(t *testing.T) {
	got := newChecker(&fakeCollector{err: ErrUnavailable}, Listen{Attempted: true, Err: "the port is already in use by another program"}).Check(context.Background())
	if got.State != StateNotListening || got.Cause != CauseBindFailed {
		t.Fatalf("got %v", got)
	}
}
