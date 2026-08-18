package domain

import (
	"time"

	"github.com/google/uuid"
)

// ChannelType is how an RFQ reached the branch.
type ChannelType string

const (
	ChannelTypeWhatsApp    ChannelType = "WHATSAPP"
	ChannelTypeEmail       ChannelType = "EMAIL"
	ChannelTypeWebApp      ChannelType = "WEBAPP"
	ChannelTypeManualEntry ChannelType = "MANUAL_ENTRY"
)

// Channel is an intake route configured for one branch.
type Channel struct {
	ID         uuid.UUID
	AccountID  uuid.UUID
	BranchID   uuid.UUID
	Type       ChannelType
	IsActive   bool
	Identifier *string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
