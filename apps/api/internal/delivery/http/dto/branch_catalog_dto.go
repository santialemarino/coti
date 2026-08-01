package dto

import (
	"time"

	"github.com/google/uuid"
)

// SetAvailabilityRequest is the body for PUT /v1/products/:productId/availability.
//
// stock is a decimal string; omitting it means the branch does not track stock for the
// item, which is not the same as zero. is_active defaults to true, since the point of the
// call is normally that the branch does sell it.
type SetAvailabilityRequest struct {
	Stock    *string `json:"stock" binding:"omitempty,numeric"`
	IsActive *bool   `json:"is_active"`
}

// SetPriceRequest is the body for POST /v1/products/:productId/prices.
//
// Money is a decimal STRING parsed server-side, never a float: a JSON number would lose
// NUMERIC(14,2) precision on the round trip. valid_from defaults to now, and the previous
// period is closed at exactly that instant.
type SetPriceRequest struct {
	Price      string     `json:"price" binding:"required,numeric"`
	Currency   string     `json:"currency" binding:"omitempty,len=3"` // e.g. "ARS"; defaults to ARS.
	MinPrice   *string    `json:"min_price" binding:"omitempty,numeric"`
	Conditions *string    `json:"conditions" binding:"omitempty,max=255"`
	ValidFrom  *time.Time `json:"valid_from"`
}

// AvailabilityResponse is returned by the availability list and set endpoints.
type AvailabilityResponse struct {
	ID        uuid.UUID `json:"id"`
	BranchID  uuid.UUID `json:"branch_id"`
	ProductID uuid.UUID `json:"product_id"`
	Stock     *string   `json:"stock"` // decimal string; null when the branch does not track it.
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AvailabilityListResponse is returned by GET /v1/products/:productId/availability.
type AvailabilityListResponse struct {
	Items []AvailabilityResponse `json:"items"`
}

// PriceResponse is one validity period. valid_to is null on the period in force.
type PriceResponse struct {
	ID         uuid.UUID  `json:"id"`
	BranchID   uuid.UUID  `json:"branch_id"`
	ProductID  uuid.UUID  `json:"product_id"`
	Price      string     `json:"price"` // decimal string, never a float.
	Currency   string     `json:"currency"`
	MinPrice   *string    `json:"min_price"` // discount-engine floor; decimal string.
	Conditions *string    `json:"conditions"`
	ValidFrom  time.Time  `json:"valid_from"`
	ValidTo    *time.Time `json:"valid_to"`
	SetBy      *uuid.UUID `json:"set_by"`
	CreatedAt  time.Time  `json:"created_at"`
}

// PriceListResponse is returned by GET /v1/products/:productId/prices, newest period first.
type PriceListResponse struct {
	Items []PriceResponse `json:"items"`
}
