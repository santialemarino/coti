package domain

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ChangeRequestOperationType is one atomic draft mutation proposed from a customer message.
type ChangeRequestOperationType string

const (
	// ChangeRequestOperationAddItem appends a newly requested material to the draft.
	ChangeRequestOperationAddItem ChangeRequestOperationType = "ADD_ITEM"
	// ChangeRequestOperationRemoveItem removes a material the customer no longer wants.
	ChangeRequestOperationRemoveItem ChangeRequestOperationType = "REMOVE_ITEM"
	// ChangeRequestOperationReplaceItem substitutes one requested material for another.
	ChangeRequestOperationReplaceItem ChangeRequestOperationType = "REPLACE_ITEM"
	// ChangeRequestOperationUpdateQuantity changes the requested quantity of one material.
	ChangeRequestOperationUpdateQuantity ChangeRequestOperationType = "UPDATE_QUANTITY"
	// ChangeRequestOperationAddDiscount names an in-force promotion; the engine computes its money.
	ChangeRequestOperationAddDiscount ChangeRequestOperationType = "ADD_DISCOUNT"
	// ChangeRequestOperationRequiresSeller leaves an ambiguous or unsupported request to the seller.
	ChangeRequestOperationRequiresSeller ChangeRequestOperationType = "REQUIRES_SELLER"
)

// ChangeRequestLevel is how far the model could resolve a request: direct line changes, a goal the
// backend reaches through a recipe, or nothing it can act on.
type ChangeRequestLevel string

const (
	ChangeRequestLevelAtomic  ChangeRequestLevel = "ATOMIC"
	ChangeRequestLevelComplex ChangeRequestLevel = "COMPLEX"
	ChangeRequestLevelNoMatch ChangeRequestLevel = "NO_MATCH"
)

// ChangeRequestIntent is one goal of the closed set the backend has a recipe for.
type ChangeRequestIntent string

const (
	// ChangeRequestIntentLowerTotalPrice asks for a lower total without the customer naming how.
	ChangeRequestIntentLowerTotalPrice ChangeRequestIntent = "LOWER_TOTAL_PRICE"
	// ChangeRequestIntentImproveQuality asks for better materials than the ones quoted.
	ChangeRequestIntentImproveQuality ChangeRequestIntent = "IMPROVE_QUALITY"
)

// ChangeRequestPromotion is an in-force promotion the handler may name in an ADD_DISCOUNT.
type ChangeRequestPromotion struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
}

// ChangeRequestItem is the existing draft line the handler may name in a proposed operation.
type ChangeRequestItem struct {
	ID                   uuid.UUID       `json:"id"`
	RequestedDescription string          `json:"requested_description"`
	Quantity             decimal.Decimal `json:"quantity"`
	Unit                 *string         `json:"unit"`
}

// ConversationLine is one earlier message the handler reads as context, never as a request.
type ConversationLine struct {
	Author MessageAuthorType `json:"author"`
	Body   string            `json:"text"`
}

// ChangeRequestInput is the bounded state and messages the handler needs to propose a change.
type ChangeRequestInput struct {
	Message string
	Context []ConversationLine
	Items   []ChangeRequestItem
	// Promotions are the rules in force at the quote's branch; ADD_DISCOUNT names one of them.
	Promotions []ChangeRequestPromotion
}

// ChangeRequestOperation is one typed proposal that still needs backend validation and seller review.
type ChangeRequestOperation struct {
	Type                 ChangeRequestOperationType `json:"type"`
	TargetItemID         *uuid.UUID                 `json:"target_item_id"`
	RequestedDescription *string                    `json:"requested_description"`
	Quantity             *decimal.Decimal           `json:"quantity"`
	Unit                 *string                    `json:"unit"`
	PromotionID          *uuid.UUID                 `json:"promotion_id,omitempty"`
}

// ChangeRequestProposal is the model's structured interpretation of one customer change request.
// A COMPLEX proposal carries an intent and no operations: the backend's recipe produces them.
type ChangeRequestProposal struct {
	Level          ChangeRequestLevel   `json:"level"`
	Interpretation string               `json:"interpretation"`
	Intent         *ChangeRequestIntent `json:"intent,omitempty"`
	// IntentItemIDs are the lines the customer named for the intent; empty means the whole quote.
	IntentItemIDs []uuid.UUID              `json:"intent_item_ids,omitempty"`
	Operations    []ChangeRequestOperation `json:"operations"`
}

// ChangeRequestHandler turns a customer message into reviewable draft operations.
type ChangeRequestHandler interface {
	Propose(ctx context.Context, in ChangeRequestInput) (*ChangeRequestProposal, error)
}

// QuoteItemQuantityUpdate is one quantity mutation scoped to a mutable quote version.
type QuoteItemQuantityUpdate struct {
	ItemID   uuid.UUID
	Quantity decimal.Decimal
}

// QuoteItemReplacement is one full material replacement scoped to a mutable quote version.
type QuoteItemReplacement struct {
	ItemID               uuid.UUID
	ProductID            *uuid.UUID
	RequestedDescription string
	Quantity             decimal.Decimal
	Unit                 *string
	ConfidenceScore      decimal.NullDecimal
	MatchStatus          ItemMatchStatus
}
