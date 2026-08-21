package ai

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// instant keeps the waits out of the wall clock: the doubling is asserted on the recorded
// intervals, not on how long the test took.
func instant(attempts int) config.AIRetryPolicy {
	return config.AIRetryPolicy{MaxAttempts: attempts, Backoff: time.Microsecond}
}

func TestRetry_SucceedsWithoutRetryingOnFirstAttempt(t *testing.T) {
	t.Parallel()

	calls := 0
	attempts, err := Retry(context.Background(), instant(3), func(context.Context) error {
		calls++
		return nil
	})
	if err != nil {
		t.Fatalf("Retry() = %v, want nil", err)
	}
	if attempts != 1 || calls != 1 {
		t.Fatalf("attempts = %d, calls = %d, want 1 and 1", attempts, calls)
	}
}

func TestRetry_RetriesMarkedErrorUntilItSucceeds(t *testing.T) {
	t.Parallel()

	calls := 0
	attempts, err := Retry(context.Background(), instant(3), func(context.Context) error {
		calls++
		if calls < 3 {
			return Retryable(errors.New("provider overloaded"))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Retry() = %v, want nil", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestRetry_UnmarkedErrorStopsImmediately(t *testing.T) {
	t.Parallel()

	refused := errors.New("provider rejected the request")
	calls := 0
	attempts, err := Retry(context.Background(), instant(3), func(context.Context) error {
		calls++
		return refused
	})
	if !errors.Is(err, refused) {
		t.Fatalf("Retry() = %v, want %v", err, refused)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1: a rejected request must not be repeated", calls)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestRetry_ExhaustedAttemptsReportTheLastCause(t *testing.T) {
	t.Parallel()

	overloaded := errors.New("provider overloaded")
	calls := 0
	attempts, err := Retry(context.Background(), instant(3), func(context.Context) error {
		calls++
		return Retryable(overloaded)
	})
	if !errors.Is(err, overloaded) {
		t.Fatalf("Retry() = %v, want it to wrap %v", err, overloaded)
	}
	if calls != 3 || attempts != 3 {
		t.Fatalf("calls = %d, attempts = %d, want 3 and 3", calls, attempts)
	}
	// The marker is an internal signal and must not survive into the caller's error.
	var marker *retryable
	if errors.As(err, &marker) {
		t.Fatal("the retryable marker leaked into the returned error")
	}
}

func TestRetry_WaitDoublesBetweenAttempts(t *testing.T) {
	t.Parallel()

	const backoff = 20 * time.Millisecond
	policy := config.AIRetryPolicy{MaxAttempts: 4, Backoff: backoff}

	var at []time.Time
	if _, err := Retry(context.Background(), policy, func(context.Context) error {
		at = append(at, time.Now())
		return Retryable(errors.New("provider overloaded"))
	}); err == nil {
		t.Fatal("Retry() = nil, want the attempts to run out")
	}
	if len(at) != 4 {
		t.Fatalf("attempts recorded = %d, want 4", len(at))
	}

	// Each wait is measured against the interval it should have been, not against the previous
	// measurement: a scheduler runs late but never early, and comparing one measurement to
	// another would fold that lateness into the ratio.
	want := backoff
	for i := 1; i < len(at); i++ {
		if got := at[i].Sub(at[i-1]); got < want {
			t.Fatalf("wait %d was %s, want at least %s", i, got, want)
		}
		want *= 2
	}
}

func TestRetry_CancelledContextStopsBeforeCalling(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	calls := 0
	attempts, err := Retry(ctx, instant(3), func(context.Context) error {
		calls++
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Retry() = %v, want %v", err, context.Canceled)
	}
	if calls != 0 {
		t.Fatalf("calls = %d, want 0", calls)
	}
	if attempts != 0 {
		t.Fatalf("attempts = %d, want 0", attempts)
	}
}

func TestRetry_ContextCancelledMidRunStopsWaiting(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	policy := config.AIRetryPolicy{MaxAttempts: 5, Backoff: time.Hour}

	calls := 0
	_, err := Retry(ctx, policy, func(context.Context) error {
		calls++
		cancel()
		return Retryable(errors.New("provider overloaded"))
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Retry() = %v, want %v", err, context.Canceled)
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1: the hour-long wait must not have been served", calls)
	}
}

func TestUnavailable_CarriesTheDomainErrorAndItsCode(t *testing.T) {
	t.Parallel()

	cause := errors.New("provider overloaded")
	err := Unavailable(cause)

	if !errors.Is(err, domain.ErrAIUnavailable) {
		t.Fatalf("Unavailable() does not match domain.ErrAIUnavailable: %v", err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("Unavailable() dropped the cause: %v", err)
	}
	if got := domain.CodeOf(err); got != domain.CodeAIUnavailable {
		t.Fatalf("CodeOf() = %q, want %q", got, domain.CodeAIUnavailable)
	}
}

// The doubling has to have a ceiling: without one, raising the attempt count alone produces waits
// measured in minutes.
func TestRetry_WaitStopsDoublingAtTheCeiling(t *testing.T) {
	t.Parallel()

	policy := config.AIRetryPolicy{
		MaxAttempts: 5,
		Backoff:     10 * time.Millisecond,
		MaxBackoff:  20 * time.Millisecond,
	}
	var at []time.Time
	if _, err := Retry(context.Background(), policy, func(context.Context) error {
		at = append(at, time.Now())
		return Retryable(errors.New("provider overloaded"))
	}); err == nil {
		t.Fatal("Retry() = nil, want the attempts to run out")
	}

	// 10ms, 20ms, then held at 20ms rather than 40ms and 80ms. Only the ceiling is asserted: a
	// scheduler runs late, so the floor is the contract and an upper bound would be flaky.
	for i := 1; i < len(at); i++ {
		if got := at[i].Sub(at[i-1]); got < 10*time.Millisecond {
			t.Fatalf("wait %d was %s, want at least the 10ms backoff", i, got)
		}
	}
	if total := at[len(at)-1].Sub(at[0]); total > time.Second {
		t.Fatalf("the four waits took %s, so the ceiling did not hold", total)
	}
}

// The provider's own window beats our ladder, which is measured in single seconds.
func TestRetry_HonoursTheWaitTheProviderAskedFor(t *testing.T) {
	t.Parallel()

	policy := config.AIRetryPolicy{
		MaxAttempts: 2,
		Backoff:     time.Microsecond,
		MaxBackoff:  time.Second,
	}
	var at []time.Time
	calls := 0
	if _, err := Retry(context.Background(), policy, func(context.Context) error {
		at = append(at, time.Now())
		calls++
		if calls == 1 {
			return RetryableAfter(errors.New("rate limited"), 40*time.Millisecond)
		}
		return nil
	}); err != nil {
		t.Fatalf("Retry() = %v, want nil", err)
	}
	if got := at[1].Sub(at[0]); got < 40*time.Millisecond {
		t.Fatalf("waited %s, want at least the 40ms the provider asked for", got)
	}
}

// A window longer than we are willing to wait is not worth sitting out, and every attempt inside
// it would fail and spend the allowance.
func TestRetry_StopsWhenTheProviderAsksForLongerThanTheCeiling(t *testing.T) {
	t.Parallel()

	policy := config.AIRetryPolicy{
		MaxAttempts: 5,
		Backoff:     time.Microsecond,
		MaxBackoff:  time.Millisecond,
	}
	rateLimited := errors.New("rate limited")
	calls := 0
	attempts, err := Retry(context.Background(), policy, func(context.Context) error {
		calls++
		return RetryableAfter(rateLimited, time.Hour)
	})
	if !errors.Is(err, rateLimited) {
		t.Fatalf("Retry() = %v, want it to wrap %v", err, rateLimited)
	}
	if calls != 1 || attempts != 1 {
		t.Fatalf("calls = %d, attempts = %d, want 1 and 1", calls, attempts)
	}
}

// Retry is exported and config only guards the configured policy, so a zero one must still make one
// honest attempt rather than return an error wrapping nothing.
func TestRetry_ZeroPolicyMakesOneAttempt(t *testing.T) {
	t.Parallel()

	failed := errors.New("provider overloaded")
	calls := 0
	attempts, err := Retry(context.Background(), config.AIRetryPolicy{}, func(context.Context) error {
		calls++
		return Retryable(failed)
	})
	if calls != 1 || attempts != 1 {
		t.Fatalf("calls = %d, attempts = %d, want 1 and 1", calls, attempts)
	}
	if !errors.Is(err, failed) {
		t.Fatalf("Retry() = %v, want it to wrap %v rather than nothing", err, failed)
	}
	if strings.Contains(err.Error(), "%!w") {
		t.Fatalf("Retry() = %q, which wraps nothing and breaks errors.Is", err)
	}
}

func TestRetryableStatus(t *testing.T) {
	t.Parallel()

	cases := map[int]bool{
		http.StatusOK:                  false,
		http.StatusBadRequest:          false,
		http.StatusUnauthorized:        false,
		http.StatusNotFound:            false,
		http.StatusTooManyRequests:     true,
		http.StatusInternalServerError: true,
		http.StatusBadGateway:          true,
		http.StatusServiceUnavailable:  true,
	}
	for code, want := range cases {
		if got := RetryableStatus(code); got != want {
			t.Errorf("RetryableStatus(%d) = %v, want %v", code, got, want)
		}
	}
}

func TestRetryAfter(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		header string
		want   time.Duration
	}{
		{"absent", "", 0},
		{"seconds", "30", 30 * time.Second},
		{"zero seconds", "0", 0},
		{"negative seconds", "-5", 0},
		{"unparseable", "soon", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			header := http.Header{}
			if tc.header != "" {
				header.Set("Retry-After", tc.header)
			}
			if got := RetryAfter(header); got != tc.want {
				t.Fatalf("RetryAfter(%q) = %s, want %s", tc.header, got, tc.want)
			}
		})
	}

	// The date form is relative to now, so only its sign is worth asserting.
	future := http.Header{}
	future.Set("Retry-After", time.Now().Add(time.Minute).UTC().Format(http.TimeFormat))
	if got := RetryAfter(future); got <= 0 {
		t.Fatalf("RetryAfter(a minute from now) = %s, want a positive wait", got)
	}
	past := http.Header{}
	past.Set("Retry-After", time.Now().Add(-time.Minute).UTC().Format(http.TimeFormat))
	if got := RetryAfter(past); got != 0 {
		t.Fatalf("RetryAfter(a minute ago) = %s, want zero", got)
	}
}

// Fail is what decides whether a caller is told "the provider is down" or "this was our fault",
// and the difference is whether retrying could ever help.
func TestFail(t *testing.T) {
	t.Parallel()

	cause := errors.New("provider answered 400")

	if err := Fail(Rejected(cause)); errors.Is(err, domain.ErrAIUnavailable) {
		t.Fatalf("Fail(Rejected) = %v, want it NOT to read as an outage", err)
	}
	if err := Fail(Rejected(cause)); !errors.Is(err, cause) {
		t.Fatalf("Fail(Rejected) = %v, want it to keep the cause", err)
	}
	if err := Fail(context.Canceled); errors.Is(err, domain.ErrAIUnavailable) {
		t.Fatalf("Fail(context.Canceled) = %v, want it NOT to read as an outage", err)
	}
	if err := Fail(cause); !errors.Is(err, domain.ErrAIUnavailable) {
		t.Fatalf("Fail(%v) = %v, want the domain error", cause, err)
	}
}
