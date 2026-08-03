package handler

import (
	"context"
	"strings"
	"testing"
	"time"
)

// The two things the per-address cap has to get right on its own: the key it counts by never
// carries the mailbox, and the same mailbox spelled differently is still one bucket.

const testAddress = "Vendedor@Corralon.test"

// countingLimiter records the keys it was asked about and refuses past its limit.
type countingLimiter struct {
	seen  []string
	calls map[string]int
	limit int
}

func newCountingLimiter(limit int) *countingLimiter {
	return &countingLimiter{calls: map[string]int{}, limit: limit}
}

func (l *countingLimiter) Allow(_ context.Context, key string, _ int, _ time.Duration) (bool, time.Duration) {
	l.seen = append(l.seen, key)
	l.calls[key]++
	return l.calls[key] <= l.limit, time.Minute
}

func limiterFor(counter *countingLimiter, perAddress int) *MailTargetLimiter {
	return NewMailTargetLimiter(counter, MailTargetLimitOptions{
		Enabled: true, Window: time.Minute, Limit: perAddress,
	})
}

func TestMailTargetLimiter_KeyCarriesNoAddress(t *testing.T) {
	t.Parallel()
	counter := newCountingLimiter(10)

	limiterFor(counter, 3).Allow(context.Background(), testAddress)

	if len(counter.seen) != 1 {
		t.Fatalf("the limiter was asked %d times, want 1", len(counter.seen))
	}
	key := counter.seen[0]
	for _, fragment := range []string{testAddress, strings.ToLower(testAddress), "corralon.test", "Vendedor"} {
		if strings.Contains(key, fragment) {
			t.Errorf("the counter key %q carries %q, so a dump of the store exposes the mailbox",
				key, fragment)
		}
	}
}

func TestMailTargetLimiter_CountsCaseAndPaddingAsOneAddress(t *testing.T) {
	t.Parallel()
	counter := newCountingLimiter(10)
	limiter := limiterFor(counter, 3)
	ctx := context.Background()

	limiter.Allow(ctx, testAddress)
	limiter.Allow(ctx, "  "+strings.ToUpper(testAddress)+"  ")

	if len(counter.calls) != 1 {
		t.Fatalf("the same address in two spellings opened %d buckets, want 1", len(counter.calls))
	}
}

func TestMailTargetLimiter_RefusesPastTheAllowance(t *testing.T) {
	t.Parallel()
	counter := newCountingLimiter(2)
	limiter := limiterFor(counter, 2)
	ctx := context.Background()

	for i := 1; i <= 2; i++ {
		if !limiter.Allow(ctx, testAddress) {
			t.Fatalf("message %d of 2 was refused", i)
		}
	}
	if limiter.Allow(ctx, testAddress) {
		t.Error("the third message was allowed, want refused")
	}
	// A different mailbox is a different bucket, so one flooded address cannot silence another.
	if !limiter.Allow(ctx, "otro@corralon.test") {
		t.Error("another address was refused on the first message")
	}
}

func TestMailTargetLimiter_AllowsEverythingWhenOff(t *testing.T) {
	t.Parallel()
	counter := newCountingLimiter(0)
	off := NewMailTargetLimiter(counter, MailTargetLimitOptions{
		Enabled: false, Window: time.Minute, Limit: 1,
	})

	if !off.Allow(context.Background(), testAddress) {
		t.Error("the cap refused a message with rate limiting off")
	}
	if len(counter.seen) != 0 {
		t.Errorf("the counter was consulted %d times with rate limiting off", len(counter.seen))
	}
}
