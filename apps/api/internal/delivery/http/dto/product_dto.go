package dto

import (
	"time"

	"github.com/google/uuid"
)

// ListProductsQuery is the query string for GET /v1/products.
type ListProductsQuery struct {
	Search          string `form:"search" binding:"omitempty,max=255"`
	Category        string `form:"category" binding:"omitempty,max=255"`
	IncludeInactive bool   `form:"include_inactive"`
	Limit           int    `form:"limit" binding:"omitempty,min=1"`
	Offset          int    `form:"offset" binding:"omitempty,min=0"`
}

// CreateProductRequest is the body for POST /v1/products. The account comes from the
// tenant context, never the body.
type CreateProductRequest struct {
	Code          *string `json:"code" binding:"omitempty,max=255"`
	CanonicalName string  `json:"canonical_name" binding:"required,min=1,max=255"`
	Description   *string `json:"description" binding:"omitempty,max=512"`
	Unit          *string `json:"unit" binding:"omitempty,max=64"`
	Category      *string `json:"category" binding:"omitempty,max=255"`
}

// UpdateProductRequest is the body for PUT /v1/products/:productId.
//
// It replaces the editable attributes: an omitted nullable field clears the column, so
// the caller sends the item as it should end up. is_active is the exception — omitted
// leaves the flag alone, which keeps DELETE the only way to deactivate an item.
type UpdateProductRequest struct {
	Code          *string `json:"code" binding:"omitempty,max=255"`
	CanonicalName string  `json:"canonical_name" binding:"required,min=1,max=255"`
	Description   *string `json:"description" binding:"omitempty,max=512"`
	Unit          *string `json:"unit" binding:"omitempty,max=64"`
	Category      *string `json:"category" binding:"omitempty,max=255"`
	IsActive      *bool   `json:"is_active"`
}

// AddSynonymRequest is the body for POST /v1/products/:productId/synonyms. source
// defaults to MANUAL, which is what a person loading terms in the backoffice produces.
type AddSynonymRequest struct {
	Term   string `json:"term" binding:"required,min=1,max=255"`
	Source string `json:"source" binding:"omitempty,oneof=MANUAL LEARNED"`
}

// ListAlternativesQuery is the query string for GET /v1/products/:productId/alternatives.
// direction defaults to OUTGOING.
type ListAlternativesQuery struct {
	Direction string `form:"direction" binding:"omitempty,oneof=OUTGOING INCOMING"`
}

// AddAlternativeRequest is the body for POST /v1/products/:productId/alternatives. The
// product in the path is the base; the body carries the product that can stand in for it.
type AddAlternativeRequest struct {
	AlternativeProductID uuid.UUID `json:"alternative_product_id" binding:"required"`
	Type                 string    `json:"type" binding:"required,oneof=EQUIVALENT PREMIUM ECONOMY"`
}

// ProductResponse is returned by list, get, create, and update.
type ProductResponse struct {
	ID            uuid.UUID `json:"id"`
	Code          *string   `json:"code"`
	CanonicalName string    `json:"canonical_name"`
	Description   *string   `json:"description"`
	Unit          *string   `json:"unit"`
	Category      *string   `json:"category"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ProductListResponse is returned by GET /v1/products. total counts every row the filter
// matches, not the ones on this page.
type ProductListResponse struct {
	Items  []ProductResponse `json:"items"`
	Total  int               `json:"total"`
	Limit  int               `json:"limit"`
	Offset int               `json:"offset"`
}

// SynonymResponse is returned by the synonym list and add endpoints.
type SynonymResponse struct {
	ID        uuid.UUID `json:"id"`
	ProductID uuid.UUID `json:"product_id"`
	Term      string    `json:"term"`
	Source    string    `json:"source"`
	CreatedAt time.Time `json:"created_at"`
}

// SynonymListResponse is returned by GET /v1/products/:productId/synonyms.
type SynonymListResponse struct {
	Items []SynonymResponse `json:"items"`
}

// AlternativeResponse is returned by the alternative list and add endpoints. product is
// the item on the far end of the link, so a list renders without a follow-up request.
type AlternativeResponse struct {
	ID                   uuid.UUID        `json:"id"`
	BaseProductID        uuid.UUID        `json:"base_product_id"`
	AlternativeProductID uuid.UUID        `json:"alternative_product_id"`
	Type                 string           `json:"type"`
	Product              *ProductResponse `json:"product,omitempty"`
	CreatedAt            time.Time        `json:"created_at"`
}

// AlternativeListResponse is returned by GET /v1/products/:productId/alternatives.
type AlternativeListResponse struct {
	Items     []AlternativeResponse `json:"items"`
	Direction string                `json:"direction"`
}
