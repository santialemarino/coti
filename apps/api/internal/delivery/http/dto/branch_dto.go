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
