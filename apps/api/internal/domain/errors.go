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

	// ErrNoTenantContext is a programming error, not a client one: a request-scoped query
	// without tenant context silently returns nothing under row level security.
	ErrNoTenantContext = errors.New("no tenant context")
)
