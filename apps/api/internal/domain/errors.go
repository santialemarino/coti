// Package domain holds entities, value objects, enums, domain errors, and the port
// interfaces the services consume. It imports no other internal package.
package domain

import "errors"

// Domain errors the handler layer maps to HTTP status codes. Services return these
// instead of HTTP concerns so the layers stay independent.
var (
	// ErrNotFound is returned when a requested row does not exist, or exists in
	// another account — under row level security the two are indistinguishable, and
	// they must stay that way so the API never confirms another tenant's data.
	ErrNotFound = errors.New("not found")

	// ErrConflict is returned when a write violates a uniqueness or state invariant.
	ErrConflict = errors.New("conflict")

	// ErrInvalidInput is returned when a value passes DTO binding but fails a
	// business rule.
	ErrInvalidInput = errors.New("invalid input")

	// ErrUnauthenticated is returned when a request carries no usable credential.
	ErrUnauthenticated = errors.New("unauthenticated")

	// ErrForbidden is returned when an authenticated caller lacks the role or scope
	// for the operation.
	ErrForbidden = errors.New("forbidden")

	// ErrImmutable is returned when a write targets a frozen quote version. Items
	// and discounts are editable only while their version has is_immutable = false.
	ErrImmutable = errors.New("target is immutable")

	// ErrNoTenantContext is returned when a request-scoped operation runs without a
	// resolved account. It is a programming error, not a client error: every query
	// on the restricted pool must carry tenant context or row level security silently
	// returns nothing.
	ErrNoTenantContext = errors.New("no tenant context")
)
