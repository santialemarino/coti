// Package ai holds the adapters behind the domain AI ports, plus the retry and usage-logging
// policy they share. Which provider backs each capability is a startup decision the composition
// root makes; nothing above the ports knows.
package ai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// retryable marks an error the loop should try again, carrying the wait the provider asked for when
// it named one. Only the adapter knows which of its failures are transient, so the decision is made
// there and read here.
type retryable struct {
	err   error
	after time.Duration
}

func (e *retryable) Error() string { return e.err.Error() }

func (e *retryable) Unwrap() error { return e.err }

// rejected marks a request the provider would not serve at all — a schema it will not accept, a
// model or key that is wrong, a safety refusal. The provider is healthy and another attempt would
// return the same answer, so this is a fault on our side rather than an outage.
type rejected struct{ err error }

func (e *rejected) Error() string { return e.err.Error() }

func (e *rejected) Unwrap() error { return e.err }

// Retryable marks err as worth another attempt: a rate limit, a provider-side fault, a broken
// connection, or an answer that did not satisfy the schema. Anything unmarked stops the loop.
func Retryable(err error) error {
	return &retryable{err: err}
}

// RetryableAfter marks err as worth another attempt no sooner than after, which is what the
// provider's own Retry-After header asked for.
func RetryableAfter(err error, after time.Duration) error {
	return &retryable{err: err, after: after}
}

// Rejected marks a request the provider refused to serve, so the caller is told it is our fault
// rather than an outage it should retry.
func Rejected(err error) error {
	return &rejected{err: err}
}

// RetryableStatus reports whether an HTTP status is worth another attempt: a rate limit or a
// provider-side fault clears on its own, anything the provider rejected outright does not. One
// definition for every adapter, so adding a status is one edit rather than one per provider.
func RetryableStatus(code int) bool {
	return code == http.StatusTooManyRequests || code >= http.StatusInternalServerError
}

// RetryAfter reads the wait a provider asked for, in either form the header takes — a count of
// seconds or an HTTP date. Zero when it said nothing usable, which leaves our own ladder in charge.
func RetryAfter(header http.Header) time.Duration {
	raw := strings.TrimSpace(header.Get("Retry-After"))
	if raw == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(raw); err == nil {
		if seconds <= 0 {
			return 0
		}
		return time.Duration(seconds) * time.Second
	}
	if at, err := http.ParseTime(raw); err == nil {
		if wait := time.Until(at); wait > 0 {
			return wait
		}
	}
	return 0
}

// Retry runs call until it succeeds, its error turns out not to be worth retrying, the attempts run
// out, or ctx ends. The wait doubles from policy.Backoff up to policy.MaxBackoff, and a provider
// that named its own window in a Retry-After header gets that instead. It returns how many attempts
// were made, which the usage log records.
func Retry(ctx context.Context, policy config.AIRetryPolicy, call func(context.Context) error) (int, error) {
	// Retry is exported and config only guards the configured policy, so a zero one still makes
	// one honest attempt rather than returning an error that wraps nothing.
	attempts := policy.MaxAttempts
	if attempts < 1 {
		attempts = 1
	}

	wait := policy.Backoff
	var err error
	for attempt := 1; attempt <= attempts; attempt++ {
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
		asked := transient.after
		// Unwrapped so the caller sees the cause rather than the marker around it.
		err = transient.Unwrap()

		if attempt == attempts {
			break
		}

		next := wait
		if asked > 0 {
			// A window longer than we are willing to wait is not worth sitting out, and every
			// attempt inside it would fail and spend the allowance anyway.
			if policy.MaxBackoff > 0 && asked > policy.MaxBackoff {
				return attempt, fmt.Errorf("provider asked for %s, longer than the %s we wait: %w",
					asked, policy.MaxBackoff, err)
			}
			next = asked
		}
		if policy.MaxBackoff > 0 && next > policy.MaxBackoff {
			next = policy.MaxBackoff
		}

		select {
		case <-ctx.Done():
			return attempt, ctx.Err()
		case <-time.After(next):
			wait *= 2
		}
	}

	// A caller who walked away mid-attempt is told that, not that the provider is down.
	if ctxErr := ctx.Err(); ctxErr != nil {
		return attempts, ctxErr
	}
	return attempts, fmt.Errorf("gave up after %d attempts: %w", attempts, err)
}

// Fail turns a provider failure into the error a caller sees. A request the provider rejected, and
// a caller who cancelled, stay as they are — neither is an outage, and reporting them as one would
// invite a retry that cannot succeed. Everything else becomes the domain's ErrAIUnavailable.
func Fail(err error) error {
	if errors.Is(err, context.Canceled) {
		return err
	}
	var refused *rejected
	if errors.As(err, &refused) {
		return err
	}
	return Unavailable(err)
}

// Unavailable wraps a provider failure in the domain error, so a caller can tell "the model could
// not answer" from a bug of its own without knowing which provider ran.
func Unavailable(err error) error {
	return domain.WithCode(domain.CodeAIUnavailable,
		fmt.Errorf("%w: %w", domain.ErrAIUnavailable, err))
}
