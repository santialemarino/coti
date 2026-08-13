// Package ai holds the adapters behind the domain AI ports, plus the retry and usage-logging
// policy they share. Which provider backs each capability is a startup decision the composition
// root makes; nothing above the ports knows.
package ai

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// retryable marks an error the loop should try again. Only the adapter knows which of its
// failures are transient, so the decision is made there and read here.
type retryable struct{ err error }

func (e *retryable) Error() string { return e.err.Error() }

func (e *retryable) Unwrap() error { return e.err }

// Retryable marks err as worth another attempt: a rate limit, a provider-side fault, a broken
// connection, or an answer that did not satisfy the schema. Anything unmarked stops the loop.
func Retryable(err error) error {
	return &retryable{err: err}
}

// Retry runs call until it succeeds, its error turns out not to be worth retrying, the attempts
// run out, or ctx ends. The wait doubles from policy.Backoff, so a provider that is rate limiting
// gets progressively more room. It returns how many attempts were made, which the usage log
// records.
func Retry(ctx context.Context, policy config.AIRetryPolicy, call func(context.Context) error) (int, error) {
	wait := policy.Backoff
	var err error
	for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return attempt - 1, ctxErr
		}

		err = call(ctx)
		if err == nil {
			return attempt, nil
		}

		var transient *retryable
		if !errors.As(err, &transient) {
			return attempt, err
		}
		// Unwrapped so the caller sees the cause rather than the marker around it.
		err = transient.Unwrap()

		if attempt == policy.MaxAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return attempt, ctx.Err()
		case <-time.After(wait):
			wait *= 2
		}
	}
	return policy.MaxAttempts, fmt.Errorf("gave up after %d attempts: %w", policy.MaxAttempts, err)
}

// Unavailable wraps a provider failure in the domain error, so a caller can tell "the model could
// not answer" from a bug of its own without knowing which provider ran.
func Unavailable(err error) error {
	return domain.WithCode(domain.CodeAIUnavailable,
		fmt.Errorf("%w: %w", domain.ErrAIUnavailable, err))
}
