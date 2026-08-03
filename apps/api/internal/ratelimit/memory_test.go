package ratelimit

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"
)

// The counter is the part the middleware tests exercise only indirectly: exactly how many
// requests fit, whether a refusal extends the window, and whether the retry hint is honest.

const (
	testLimit  = 3
	testWindow = time.Minute
)

func TestMemory_AllowsExactlyTheLimitPerWindow(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	m := NewMemory(func() time.Time { return now })
	ctx := context.Background()

	for i := 1; i <= testLimit; i++ {
		allowed, _ := m.Allow(ctx, "k", testLimit, testWindow)
		if !allowed {
			t.Fatalf("request %d of %d was refused", i, testLimit)
		}
	}
	allowed, retryIn := m.Allow(ctx, "k", testLimit, testWindow)
	if allowed {
		t.Fatalf("request %d was allowed, want refused", testLimit+1)
	}
	if retryIn <= 0 || retryIn > testWindow {
		t.Fatalf("retryIn = %v, want a positive value inside the window", retryIn)
	}
}

// A refusal must not push the reset further out, or a caller hammering the endpoint locks
// itself out indefinitely.
func TestMemory_ARefusalDoesNotExtendTheWindow(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	m := NewMemory(func() time.Time { return now })
	ctx := context.Background()

	for i := 0; i < testLimit; i++ {
		m.Allow(ctx, "k", testLimit, testWindow)
	}
	_, first := m.Allow(ctx, "k", testLimit, testWindow)
	for i := 0; i < 20; i++ {
		m.Allow(ctx, "k", testLimit, testWindow)
	}
	_, afterHammering := m.Allow(ctx, "k", testLimit, testWindow)

	if afterHammering != first {
		t.Fatalf("retryIn moved from %v to %v after repeated refusals", first, afterHammering)
	}
}

func TestMemory_TheWindowResetsOnceItExpires(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	m := NewMemory(func() time.Time { return now })
	ctx := context.Background()

	for i := 0; i < testLimit; i++ {
		m.Allow(ctx, "k", testLimit, testWindow)
	}
	if allowed, _ := m.Allow(ctx, "k", testLimit, testWindow); allowed {
		t.Fatal("the allowance was not spent")
	}

	now = now.Add(testWindow + time.Second)
	if allowed, _ := m.Allow(ctx, "k", testLimit, testWindow); !allowed {
		t.Fatal("the window did not reset after it expired")
	}
}

func TestMemory_KeysDoNotShareAnAllowance(t *testing.T) {
	t.Parallel()
	m := NewMemory(nil)
	ctx := context.Background()

	for i := 0; i < testLimit; i++ {
		m.Allow(ctx, "a", testLimit, testWindow)
	}
	if allowed, _ := m.Allow(ctx, "a", testLimit, testWindow); allowed {
		t.Fatal("key a still had allowance")
	}
	if allowed, _ := m.Allow(ctx, "b", testLimit, testWindow); !allowed {
		t.Fatal("key b was refused because key a had spent its allowance")
	}
}

// Expired windows have to be dropped, or a stream of distinct keys grows the map forever.
func TestMemory_SweepsExpiredWindows(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	m := NewMemory(func() time.Time { return now })
	ctx := context.Background()

	for i := 0; i < 100; i++ {
		m.Allow(ctx, strconv.Itoa(i), testLimit, testWindow)
	}
	if len(m.windows) != 100 {
		t.Fatalf("held %d windows, want 100", len(m.windows))
	}

	now = now.Add(testWindow * sweepWindows).Add(time.Second)
	m.Allow(ctx, "fresh", testLimit, testWindow)
	if len(m.windows) != 1 {
		t.Fatalf("held %d windows after the sweep, want only the fresh one", len(m.windows))
	}
}

// The whole read-modify-write is under one lock, so the total allowed across racing callers
// has to be exactly the limit.
func TestMemory_ConcurrentCallersCannotExceedTheLimit(t *testing.T) {
	t.Parallel()
	m := NewMemory(nil)
	ctx := context.Background()

	const callers = 50
	var wg sync.WaitGroup
	var mu sync.Mutex
	allowedCount := 0

	start := make(chan struct{})
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if allowed, _ := m.Allow(ctx, "shared", testLimit, testWindow); allowed {
				mu.Lock()
				allowedCount++
				mu.Unlock()
			}
		}()
	}
	close(start)
	wg.Wait()

	if allowedCount != testLimit {
		t.Fatalf("%d of %d racing callers were allowed, want exactly %d", allowedCount, callers, testLimit)
	}
}
