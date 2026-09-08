package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// PromotionConditionType is the condition family the deterministic engine applies a
// discount under.
type PromotionConditionType string

const (
	PromotionConditionPerItem      PromotionConditionType = "PER_ITEM"
	PromotionConditionQuantityTier PromotionConditionType = "QUANTITY_TIERED"
	PromotionConditionItemSet      PromotionConditionType = "ITEM_SET"
	PromotionConditionOnTotal      PromotionConditionType = "ON_TOTAL"
)

// DiscountScope is what a discount application covers.
type DiscountScope string

const (
	DiscountScopeItem    DiscountScope = "ITEM"
	DiscountScopeItemSet DiscountScope = "ITEM_SET"
	DiscountScopeTotal   DiscountScope = "TOTAL"
)

// DiscountOrigin is who decided the discount; only MANUAL_SELLER is deletable by the
// seller, suppressing an AUTOMATIC one is reversible instead.
type DiscountOrigin string

const (
	DiscountOriginAutomatic    DiscountOrigin = "AUTOMATIC"
	DiscountOriginAIAdaptation DiscountOrigin = "AI_ADAPTATION"
	DiscountOriginManualSeller DiscountOrigin = "MANUAL_SELLER"
)

// PromotionActionType is how a discount rule turns into money: a fixed amount or a
// percentage over its scope base. SPECIAL_PRICE belongs to per-item promotion rules and
// is never accepted from a seller-typed discount.
type PromotionActionType string

const (
	PromotionActionFixedAmount  PromotionActionType = "FIXED_AMOUNT"
	PromotionActionPercentage   PromotionActionType = "PERCENTAGE"
	PromotionActionSpecialPrice PromotionActionType = "SPECIAL_PRICE"
)

// QuoteDiscount is one application of a discount to a quote version. The amount is
// computed by the deterministic engine, never by the AI; a MANUAL_SELLER one also keeps
// the rule it was typed with — the action type and its raw value — so the engine can
// recompute the amount whenever the items change.
type QuoteDiscount struct {
	ID             uuid.UUID
	AccountID      uuid.UUID
	QuoteVersionID uuid.UUID
	PromotionID    *uuid.UUID
	PromotionName  *string // COALESCE(promotion.name, quote_discount.description) via join.
	ConditionType  PromotionConditionType
	Scope          DiscountScope
	Origin         DiscountOrigin
	Amount         decimal.Decimal // NUMERIC(14,2); computed money, never the rule.
	ActionType     PromotionActionType
	ActionValue    *decimal.Decimal // raw seller input: money or percentage.
	// ItemIDs are the quote lines an ITEM/ITEM_SET discount covers, in link order; empty
	// for a TOTAL one and on create/update responses.
	ItemIDs            []uuid.UUID
	Description        *string
	SuppressedBySeller bool
	CreatedAt          time.Time
}

// QuoteDiscountCreate is the input for a seller-typed discount. Value is the raw figure
// the seller typed, its meaning depending on ActionType: money for FIXED_AMOUNT, a
// percentage for PERCENTAGE. ItemIDs scope the discount to lines; it stays empty for a
// TOTAL discount. The service computes the money amount from these before persisting.
type QuoteDiscountCreate struct {
	Description string
	ActionType  PromotionActionType
	Value       decimal.Decimal
	Amount      decimal.Decimal // computed money, set by the service.
	Scope       DiscountScope
	ItemIDs     []uuid.UUID
}

// QuoteDiscountUpdate is the mutable surface of a quote discount. All fields are
// optional: only present fields are written. Value along with ActionType re-enters the
// rule and makes the engine recompute the amount; ItemIDs replaces the lines the discount
// covers. ConditionType follows Scope (ITEM→PER_ITEM and so on) and is set by the caller
// when the scope changes.
type QuoteDiscountUpdate struct {
	Value              *decimal.Decimal
	ActionType         *PromotionActionType
	ItemIDs            []uuid.UUID
	Scope              *DiscountScope
	ConditionType      *PromotionConditionType
	Description        *string
	SuppressedBySeller *bool
}
