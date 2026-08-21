package dto

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ChannelResponse represents one intake channel of the selected branch. It deliberately omits
// channel.config: that is where a provider credential lives, and a caller needs to know a channel
// is configured, never with what. is_configured is that indicator.
type ChannelResponse struct {
	ID           uuid.UUID `json:"id"`
	BranchID     uuid.UUID `json:"branch_id"`
	Type         string    `json:"type"`
	Identifier   *string   `json:"identifier"`
	IsActive     bool      `json:"is_active"`
	IsConfigured bool      `json:"is_configured"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ChannelListResponse is returned by GET /v1/channels.
type ChannelListResponse struct {
	Items []ChannelResponse `json:"items"`
}

// ListChannelsQuery is the query for GET /v1/channels. Including the closed ones is for
// administering them, never for taking an order through one, so it is refused to a seller.
type ListChannelsQuery struct {
	IncludeInactive bool `form:"include_inactive"`
}

// CreateChannelRequest is the body for POST /v1/channels. The branch comes from X-Branch-Id, never
// the body, and config is validated against type — see the channel section of
// docs/technical/accounts-and-branches.md for the shape each type takes.
type CreateChannelRequest struct {
	Type       string          `json:"type" binding:"required,oneof=WHATSAPP EMAIL WEBAPP MANUAL_ENTRY"`
	Identifier *string         `json:"identifier" binding:"omitempty,max=255"`
	Config     json.RawMessage `json:"config" swaggertype:"object"`
}

// UpdateChannelRequest is the body for PUT /v1/channels/:channelId. The type cannot change, because
// the shape of the configuration depends on it. is_active and config omitted leave what is stored
// alone; an explicit null config removes it, since no response returns one to send back.
type UpdateChannelRequest struct {
	Identifier *string         `json:"identifier" binding:"omitempty,max=255"`
	IsActive   *bool           `json:"is_active"`
	Config     json.RawMessage `json:"config" swaggertype:"object"`
}
