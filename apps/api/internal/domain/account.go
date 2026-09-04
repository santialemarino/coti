package domain

import (
	"time"

	"github.com/google/uuid"
)

// Account is one corralón. It is the tenant boundary every other table hangs off.
type Account struct {
	ID           uuid.UUID
	Name         string
	LegalName    *string
	TaxID        *string
	BrandLogoURL *string
	BrandColor   *string
	IsActive     bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// AccountUpdate replaces an account's editable fields. The brand pair is what the client
// webapp renders a quote with.
type AccountUpdate struct {
	Name         string
	LegalName    *string
	TaxID        *string
	BrandLogoURL *string
	BrandColor   *string
}

// Signup registers a corralón: the account, its first branch, and the administrator who
// operates them. The three are created together because none of them is usable alone.
type Signup struct {
	AccountName   string
	LegalName     *string
	TaxID         *string
	BranchName    string
	BranchAddress *string
	AdminName     string
	AdminEmail    string
	AdminPassword string
}

// SignupResult is what a completed registration produced.
type SignupResult struct {
	Account Account
	Branch  Branch
	Admin   AppUser
}
