package dto

import (
	"time"

	"github.com/google/uuid"
)

// ComboRequest is the body for POST /v1/combos and PUT /v1/combos/{comboId}: the whole combo.
type ComboRequest struct {
	Name        string             `json:"name" binding:"required,min=1,max=255"`
	Description *string            `json:"description" binding:"omitempty,max=512"`
	IsActive    *bool              `json:"is_active"` // defaults to true.
	Items       []ComboItemRequest `json:"items" binding:"required,min=2,dive"`
	BranchIDs   []uuid.UUID        `json:"branch_ids" binding:"required,min=1"`
}

// ComboItemRequest is one product of a combo, with its quantity as a decimal string.
type ComboItemRequest struct {
	ProductID uuid.UUID `json:"product_id" binding:"required"`
	Quantity  string    `json:"quantity" binding:"required,numeric"`
}

// ComboResponse is returned by every combo route.
type ComboResponse struct {
	ID          uuid.UUID           `json:"id"`
	Name        string              `json:"name"`
	Description *string             `json:"description"`
	IsActive    bool                `json:"is_active"`
	Items       []ComboItemResponse `json:"items"`
	BranchIDs   []uuid.UUID         `json:"branch_ids"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

// ComboItemResponse is one product of a combo.
type ComboItemResponse struct {
	ProductID       uuid.UUID `json:"product_id"`
	Quantity        string    `json:"quantity"`
	ProductCode     *string   `json:"product_code"`
	ProductName     string    `json:"product_name"`
	ProductUnit     *string   `json:"product_unit"`
	ProductIsActive bool      `json:"product_is_active"`
}

// ComboListResponse is returned by GET /v1/combos.
type ComboListResponse struct {
	Items []ComboResponse `json:"items"`
}

// ComboSuggestionResponse is a combo a priced quote holds part of.
type ComboSuggestionResponse struct {
	ComboID     uuid.UUID           `json:"combo_id"`
	Name        string              `json:"name"`
	Description *string             `json:"description"`
	Present     []ComboItemResponse `json:"present"`
	Missing     []ComboItemResponse `json:"missing"`
}

// ComboSuggestionListResponse is returned by GET /v1/quotes/{quoteId}/combo-suggestions.
type ComboSuggestionListResponse struct {
	Items []ComboSuggestionResponse `json:"items"`
}

// ComboItemsAddedResponse lists the lines added to complete a combo.
type ComboItemsAddedResponse struct {
	Items []QuoteItemResponse `json:"items"`
}
