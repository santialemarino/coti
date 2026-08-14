package dto

import (
	"time"

	"github.com/google/uuid"
)

// ChannelResponse represents one active intake channel of the selected branch.
type ChannelResponse struct {
	ID         uuid.UUID `json:"id"`
	BranchID   uuid.UUID `json:"branch_id"`
	Type       string    `json:"type"`
	Identifier *string   `json:"identifier"`
	IsActive   bool      `json:"is_active"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ChannelListResponse is returned by GET /v1/channels.
type ChannelListResponse struct {
	Items []ChannelResponse `json:"items"`
}
