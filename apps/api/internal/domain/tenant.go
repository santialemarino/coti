package domain

import (
	"time"

	"github.com/google/uuid"
)

// UserRole is the role of an app_user within an account.
type UserRole string

const (
	UserRoleAdmin  UserRole = "ADMIN"
	UserRoleSeller UserRole = "SELLER"
)

// Tenant is the authenticated caller's scope, resolved once per request from the
// access token and the active branch selection.
//
// AccountID is the hard boundary: it feeds the per-transaction GUC that row level
// security enforces. BranchID is the soft one — an admin legitimately reads across
// the branches of their own account, so branch filtering stays in the services and
// is empty when the caller is operating account-wide.
type Tenant struct {
	AccountID uuid.UUID
	UserID    uuid.UUID
	Role      UserRole
	BranchID  uuid.UUID
}

// IsAdmin reports whether the caller may operate across the account's branches.
func (t Tenant) IsAdmin() bool {
	return t.Role == UserRoleAdmin
}

// HasBranch reports whether a branch is selected for this request.
func (t Tenant) HasBranch() bool {
	return t.BranchID != uuid.Nil
}

// Branch is an operating location under an account. Quotes, RFQs, channels, and per-branch
// pricing hang off it.
type Branch struct {
	ID                uuid.UUID
	AccountID         uuid.UUID
	Name              string
	Address           *string
	DefaultExpiryDays int
	IsActive          bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
