package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// BranchProduct is a product's commercial availability at one branch: whether the branch
// sells it, and with how much stock. Price lives in ProductPrice, not here.
type BranchProduct struct {
	ID        uuid.UUID
	AccountID uuid.UUID
	BranchID  uuid.UUID
	ProductID uuid.UUID
	Stock     decimal.NullDecimal // NUMERIC(14,2); NULL when the branch does not track it.
	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// BranchAvailability is the input for setting what a branch carries. Stock invalid means
// the branch does not track it, which is not the same as zero.
type BranchAvailability struct {
	Stock    decimal.NullDecimal
	IsActive bool
}
