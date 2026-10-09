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
	// ChangeRequestOperationRequiresSeller leaves an ambiguous or unsupported request to the seller.
	ChangeRequestOperationRequiresSeller ChangeRequestOperationType = "REQUIRES_SELLER"
)

// ChangeRequestItem is the existing draft line the handler may name in a proposed operation.
type ChangeRequestItem struct {
	ID                   uuid.UUID       `json:"id"`
	RequestedDescription string          `json:"requested_description"`
	Quantity             decimal.Decimal `json:"quantity"`
	Unit                 *string         `json:"unit"`
}

// ChangeRequestInput is the bounded state and message the handler needs to propose a change.
type ChangeRequestInput struct {
	Message string
	Items   []ChangeRequestItem
}

// ChangeRequestOperation is one typed proposal that still needs backend validation and seller review.
type ChangeRequestOperation struct {
	Type                 ChangeRequestOperationType `json:"type"`
	TargetItemID         *uuid.UUID                 `json:"target_item_id"`
	RequestedDescription *string                    `json:"requested_description"`
	Quantity             *decimal.Decimal           `json:"quantity"`
	Unit                 *string                    `json:"unit"`
}

// ChangeRequestProposal is the model's structured interpretation of one customer change request.
type ChangeRequestProposal struct {
	Interpretation string                   `json:"interpretation"`
	Operations     []ChangeRequestOperation `json:"operations"`
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
