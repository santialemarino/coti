package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// ContactChallenge binds a one-use contact proof to the authenticated customer and supplier.
type ContactChallenge struct {
	ID         uuid.UUID
	AccountID  uuid.UUID
	CustomerID uuid.UUID
	ClientID   uuid.UUID
	Kind       ChannelType
	Value      string
	TokenHash  string
	Attempts   int
	ExpiresAt  time.Time
	ConsumedAt *time.Time
}

// ContactMessage is a deterministic onboarding or verification message.
type ContactMessage struct {
	AccountID uuid.UUID
	BranchID  uuid.UUID
	ChannelID uuid.UUID
	Kind      ChannelType
	To        string
	Body      string
	Code      string
}

// ContactMessenger delivers deterministic contact messages through configured channels.
type ContactMessenger interface {
	SendContact(context.Context, ContactMessage) (string, error)
}

// ContactWhatsAppMessage carries one channel's credentials for a deterministic customer message.
type ContactWhatsAppMessage struct {
	To          string
	Body        string
	Code        string
	Template    string
	Language    string
	Credentials WhatsAppDeliveryCredentials
}

// ContactWhatsAppSender supports onboarding text and approved verification templates.
type ContactWhatsAppSender interface {
	SendContact(context.Context, ContactWhatsAppMessage) (*DeliveryReceipt, error)
}
