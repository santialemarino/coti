package dto

import (
	"time"

	"github.com/google/uuid"
)

// BranchResponse is one branch the caller may operate on.
type BranchResponse struct {
	ID                uuid.UUID `json:"id"`
	Name              string    `json:"name"`
	Address           *string   `json:"address"`
	DefaultExpiryDays int       `json:"default_expiry_days"`
	IsActive          bool      `json:"is_active"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// BranchListResponse is returned by GET /v1/branches.
type BranchListResponse struct {
	Items []BranchResponse `json:"items"`
}

// ListBranchesQuery is the query for GET /v1/branches. Including the closed ones is for
// administering them, never for operating in one, so it is refused to a seller.
type ListBranchesQuery struct {
	IncludeInactive bool `form:"include_inactive"`
}

// CreateBranchRequest is the body for POST /v1/branches. Omitting the expiry takes the
// configured default.
type CreateBranchRequest struct {
	Name              string  `json:"name" binding:"required,min=1,max=255"`
	Address           *string `json:"address" binding:"omitempty,max=255"`
	DefaultExpiryDays int     `json:"default_expiry_days" binding:"omitempty,min=1,max=365"`
}

// UpdateBranchRequest is the body for PUT /v1/branches/:branchId. is_active omitted leaves the
// flag alone.
type UpdateBranchRequest struct {
	Name              string  `json:"name" binding:"required,min=1,max=255"`
	Address           *string `json:"address" binding:"omitempty,max=255"`
	DefaultExpiryDays int     `json:"default_expiry_days" binding:"required,min=1,max=365"`
	IsActive          *bool   `json:"is_active"`
}
