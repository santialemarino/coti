// Package ratelimit holds the counters behind the rate-limit middleware. In-memory is
// enough for a single instance; a shared store swaps in behind the same interface.
package ratelimit

import (
	"context"
	"sync"
	"time"
)

// sweepWindows is how many windows pass between sweeps of expired keys, so an unbounded
// stream of distinct keys cannot grow the map forever. Derived from the window rather than
// fixed, so tuning the window tunes this too.
const sweepWindows = 5

type window struct {
	count     int
	expiresAt time.Time
}

// Memory counts requests per key in fixed windows, held in this process.
type Memory struct {
	mu         sync.Mutex
	windows    map[string]window
	now        func() time.Time
	lastSweep  time.Time
	sweepAfter time.Duration
}

// NewMemory builds an in-memory limiter. now is injectable so window expiry is testable
// without sleeping.
func NewMemory(now func() time.Time) *Memory {
	if now == nil {
		now = time.Now
	}
	return &Memory{windows: make(map[string]window), now: now, lastSweep: now()}
}

// Allow records a request against a key and reports whether it fits, plus how long until the
// window resets. The window is fixed, which is what makes that second answer exact.
func (m *Memory) Allow(_ context.Context, key string, limit int, per time.Duration) (bool, time.Duration) {
	now := m.now()

	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepAfter = per * sweepWindows
	m.sweep(now)

	current, found := m.windows[key]
	if !found || !current.expiresAt.After(now) {
		m.windows[key] = window{count: 1, expiresAt: now.Add(per)}
		return true, 0
	}

	retryIn := current.expiresAt.Sub(now)
	if current.count >= limit {
		return false, retryIn
	}
	current.count++
	m.windows[key] = current
	return true, retryIn
}

func (m *Memory) sweep(now time.Time) {
	if m.sweepAfter <= 0 || now.Sub(m.lastSweep) < m.sweepAfter {
		return
	}
	for key, w := range m.windows {
		if !w.expiresAt.After(now) {
			delete(m.windows, key)
		}
	}
	m.lastSweep = now
}
