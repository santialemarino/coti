package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Combo is a named set of the account's products sold together. It has no price of its own:
// its items are quoted as ordinary lines, and any bundle discount is an ITEM_SET promotion.
type Combo struct {
	ID          uuid.UUID
	AccountID   uuid.UUID
	Name        string
	Description *string
	IsActive    bool
	Items       []ComboItem
	// The branches that sell the combo.
	BranchIDs []uuid.UUID
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ComboItem is one product of a combo with the quantity the combo carries.
type ComboItem struct {
	ProductID       uuid.UUID
	Quantity        decimal.Decimal
	ProductCode     *string
	ProductName     string
	ProductUnit     *string
	ProductIsActive bool
}

// ComboInput is the whole definition of a combo, for creating or replacing one.
type ComboInput struct {
	Name        string
	Description *string
	IsActive    bool
	Items       []ComboItemInput
	BranchIDs   []uuid.UUID
}

// ComboItemInput is one product of a combo definition.
type ComboItemInput struct {
	ProductID uuid.UUID
	Quantity  decimal.Decimal
}

// ComboSuggestion is a combo a quote already holds part of, with the products it still lacks.
type ComboSuggestion struct {
	ComboID     uuid.UUID
	Name        string
	Description *string
	Present     []ComboItem
	Missing     []ComboItem
}
