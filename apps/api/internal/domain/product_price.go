package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// DefaultCurrency is the currency a price takes when the caller does not name one.
const DefaultCurrency = "ARS"

// MoneyScale is how many decimals the NUMERIC(14,2) money and quantity columns keep. It
// fixes both what the API accepts and the scale amounts are rendered at.
const MoneyScale = 2

// ProductPrice is one validity period of a product's price at one branch. The table is
// append-only: a price is never overwritten, so it carries no updated_at.
type ProductPrice struct {
	ID         uuid.UUID
	AccountID  uuid.UUID
	BranchID   uuid.UUID
	ProductID  uuid.UUID
	UserID     *uuid.UUID      // who set it; nullable, since an import has no author.
	Price      decimal.Decimal // NUMERIC(14,2).
	Currency   string
	Conditions *string
	MinPrice   decimal.NullDecimal // NUMERIC(14,2); the floor the discount engine may not cross.
	ValidFrom  time.Time
	ValidTo    *time.Time // NULL on the open period — the price in force.
	CreatedAt  time.Time
}

// NewProductPrice is the input for opening a price period. An empty Currency resolves to
// DefaultCurrency and a zero ValidFrom to now, both in the service.
type NewProductPrice struct {
	Price      decimal.Decimal
	Currency   string
	Conditions *string
	MinPrice   decimal.NullDecimal
	ValidFrom  time.Time
}
