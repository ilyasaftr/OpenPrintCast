package net

import (
	"sync"
	"time"
)

const identifyMinInterval = 2 * time.Second

// identifyThrottle rate-limits Identify-Printer requests.
type identifyThrottle struct {
	mu   sync.Mutex
	last time.Time
}

func (t *identifyThrottle) allow() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if time.Since(t.last) < identifyMinInterval {
		return false
	}
	t.last = time.Now()
	return true
}
