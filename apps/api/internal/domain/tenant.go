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

// Tenant is the authenticated caller's scope, resolved once per request.
//
// AccountID is the hard boundary — it feeds the per-transaction GUC row level security
// reads. Branch reach is the soft one, filtered in the services: BranchFilter is the set a
// per-branch read narrows to.
type Tenant struct {
	AccountID uuid.UUID
	UserID    uuid.UUID
	Role      UserRole
	// EmailVerified is false until the caller proved the address they registered with. The
	// closed routes refuse on it; identity, logout and the address change do not.
	EmailVerified bool
	BranchID      uuid.UUID
	// AllowedBranchIDs confines a seller who selected no branch to the ones they are
	// assigned. Nil for an admin, who reaches the whole account.
	AllowedBranchIDs []uuid.UUID
}

// IsAdmin reports whether the caller may operate across the account's branches.
func (t Tenant) IsAdmin() bool {
	return t.Role == UserRoleAdmin
}

// HasBranch reports whether a branch is selected for this request.
func (t Tenant) HasBranch() bool {
	return t.BranchID != uuid.Nil
}

// BranchFilter is the branch set a per-branch read must narrow to. Nil means every branch in
// the account, which only an admin reaches.
func (t Tenant) BranchFilter() []uuid.UUID {
	if t.HasBranch() {
		return []uuid.UUID{t.BranchID}
	}
	if t.IsAdmin() {
		return nil
	}
	// Fails closed: a seller with no assignments, or whose assignments were never loaded,
	// reads nothing rather than the whole account.
	if t.AllowedBranchIDs == nil {
		return []uuid.UUID{}
	}
	return t.AllowedBranchIDs
}

// Branch is an operating location under an account.
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

// NewBranch is a branch to open under the caller's account. DefaultExpiryDays is how long a
// quote sent from here stays valid, and it lives on the branch because tolerance to inflation
// differs between locations.
type NewBranch struct {
	Name              string
	Address           *string
	DefaultExpiryDays int
}

// BranchUpdate replaces a branch's editable fields. IsActive is nil to leave it alone, so an
// edit form cannot silently revive a closed branch.
type BranchUpdate struct {
	Name              string
	Address           *string
	DefaultExpiryDays int
	IsActive          *bool
}
