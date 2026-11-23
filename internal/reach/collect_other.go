//go:build !windows

package reach

import "context"

// unavailable is the Collector off Windows: there is nothing to ask.
type unavailable struct{}

func (unavailable) Collect(context.Context) (Facts, error) { return Facts{}, ErrUnavailable }

// NewCollector is the Collector of this system: it always says the facts
// are unavailable, so every check ends as unknown.
func NewCollector() Collector { return unavailable{} }
