package ai

import (
	"context"
	"errors"
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
