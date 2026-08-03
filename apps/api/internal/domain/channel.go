package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ChannelType is the integrated intake and return channel for an RFQ.
type ChannelType string

const (
	ChannelTypeEmail       ChannelType = "EMAIL"
	ChannelTypeManualEntry ChannelType = "MANUAL_ENTRY"
	ChannelTypeWebApp      ChannelType = "WEBAPP"
	ChannelTypeWhatsApp    ChannelType = "WHATSAPP"
)

// Channel is one configured communication channel for a branch.
type Channel struct {
	ID         uuid.UUID
	AccountID  uuid.UUID
	BranchID   uuid.UUID
	Type       ChannelType
	Config     json.RawMessage
	IsActive   bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
	Identifier *string
}
