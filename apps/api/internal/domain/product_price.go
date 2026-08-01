package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// DefaultCurrency is the currency a price takes when the caller does not name one. The
// pilot quotes in Argentine pesos.
const DefaultCurrency = "ARS"

// MoneyScale is how many decimals the NUMERIC(14,2) money and quantity columns keep. It is
// a schema fact, not a preference: it fixes both what the API accepts and the scale amounts
// are rendered at, so "8500" and "8500.00" never describe the same row differently.
const MoneyScale = 2

// ProductPrice is one validity period of a product's price at one branch.
//
// The table is append-only, which is why it has no updated_at: setting a price closes the
// open period and opens a new row, so the history of what was quoted when survives. A
// price is never overwritten.
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
