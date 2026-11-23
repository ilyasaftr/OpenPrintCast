package winspool

import (
	"sync"
	"time"
)

// capabilityCacheTTL is how long a driver capability answer stays valid.
const capabilityCacheTTL = time.Minute

type cachedCapability[T any] struct {
	value   T
	checked time.Time
}

// capabilityCache remembers per-printer driver answers. It does no locking
// of its own: every method takes the guarding mutex from the caller so
// several caches can share one lock.
type capabilityCache[T any] struct {
	entries map[string]cachedCapability[T]
}

// lookup returns a still-fresh cached value for printer.
func (c *capabilityCache[T]) lookup(mu *sync.RWMutex, printer string) (T, bool) {
	mu.RLock()
	cached, ok := c.entries[printer]
	mu.RUnlock()
	if ok && time.Since(cached.checked) < capabilityCacheTTL {
		return cached.value, true
	}
	var zero T
	return zero, false
}

// store records value for printer.
func (c *capabilityCache[T]) store(mu *sync.RWMutex, printer string, value T) {
	mu.Lock()
	if c.entries == nil {
		c.entries = make(map[string]cachedCapability[T])
	}
	c.entries[printer] = cachedCapability[T]{value: value, checked: time.Now()}
	mu.Unlock()
}

// probeCached answers from cache while the entry is fresh, otherwise it runs
// probe and remembers the answer.
func probeCached[T any](mu *sync.RWMutex, cache *capabilityCache[T], printer string, probe func(string) T) T {
	if value, ok := cache.lookup(mu, printer); ok {
		return value
	}
	value := probe(printer)
	cache.store(mu, printer, value)
	return value
}

type qualityCapability struct {
	supported []int
	def       int
	known     bool
}
