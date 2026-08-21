// Package domain holds entities, value objects, enums, domain errors, and the port
// interfaces the services consume. It imports no other internal package.
package domain

import "errors"

// Domain errors, mapped to HTTP status codes by the handler layer.
var (
	// ErrNotFound covers "does not exist" and "belongs to another account" alike: under
	// row level security they are indistinguishable, and must stay that way.
	ErrNotFound = errors.New("not found")

	// ErrConflict is returned when a write violates a uniqueness or state invariant.
	ErrConflict = errors.New("conflict")

	// ErrInvalidInput is returned when a value passes DTO binding but fails a business rule.
	ErrInvalidInput = errors.New("invalid input")

	// ErrUnauthenticated is returned when a request carries no usable credential.
	ErrUnauthenticated = errors.New("unauthenticated")

	// ErrForbidden is returned when an authenticated caller lacks the role or scope.
	ErrForbidden = errors.New("forbidden")

	// ErrImmutable is returned when a write targets a frozen quote version.
	ErrImmutable = errors.New("target is immutable")

	// ErrLocked is returned inside a failed-attempt lockout window. Safe to surface: the
	// client needs to tell "wrong credentials" from "stop retrying".
	ErrLocked = errors.New("account locked")

	// ErrEmailNotVerified is returned when an authenticated caller reaches a closed route
	// with an unconfirmed address. Safe to surface, and it has to be: it is only reachable
	// with a live session, and the caller cannot act on it without being told.
	ErrEmailNotVerified = errors.New("email not verified")

	// ErrTooLarge is returned when an upload is over the configured size. Its own sentinel
	// because the status is 413, and the transport refuses the same thing the same way: one
	// refusal must not answer 413 from one layer and 422 from the next.
	ErrTooLarge = errors.New("payload too large")

	// ErrRateLimited is returned when a caller has spent its request allowance.
	ErrRateLimited = errors.New("too many requests")

	// ErrAIUnavailable is returned when an AI provider could not answer — no provider is bound,
	// it refused, or it kept failing. The caller handles it like any other refusal instead of
	// carrying on with a proposal it never got.
	ErrAIUnavailable = errors.New("ai provider unavailable")

	// ErrNotConfigured is returned when a request needs a capability this deployment never
	// configured — storing a channel credential with no encryption key set. Its own sentinel
	// because nothing about the request can be changed to make it succeed.
	ErrNotConfigured = errors.New("capability not configured")

	// ErrNoTenantContext is a programming error, not a client one: a request-scoped query
	// without tenant context silently returns nothing under row level security.
	ErrNoTenantContext = errors.New("no tenant context")
)
