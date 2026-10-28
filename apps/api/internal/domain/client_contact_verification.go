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

// DestinationWindows are the instants each destination cap counts from.
type DestinationWindows struct {
	Challenges   time.Time
	Failures     time.Time
	AllSuppliers time.Time
}

// DestinationActivity is what the destination caps decide on: one supplier's proofs and wrong proofs
// for a destination, and every supplier's proofs for it.
type DestinationActivity struct {
	SupplierChallenges int
	SupplierFailures   int
	AllChallenges      int
}

// ContactPurpose names why a deterministic message goes to a contact; email subjects follow it.
type ContactPurpose string

// Contact message purposes.
const (
	ContactPurposeNameRequest      ContactPurpose = "NAME_REQUEST"
	ContactPurposeVerification     ContactPurpose = "VERIFICATION"
	ContactPurposeRegistrationLink ContactPurpose = "REGISTRATION_LINK"
)

// ContactMessage is a deterministic onboarding or verification message.
type ContactMessage struct {
	AccountID uuid.UUID
	BranchID  uuid.UUID
	ChannelID uuid.UUID
	Purpose   ContactPurpose
	Kind      ChannelType
	To        string
	Body      string
	Code      string
	// InReplyTo is the RFC 5322 Message-ID an email reply threads under, blank otherwise.
	InReplyTo string
}

// ContactReplyContext describes the inbound message an automatic name prompt would answer.
type ContactReplyContext struct {
	// Automated marks bulk, list, auto-submitted or no-reply mail, which never gets a prompt.
	Automated bool
	MessageID string
}

// ContactMailbox is the supplier identity a branch mailbox answers a contact with.
type ContactMailbox struct {
	Address      string
	SupplierName string
	BranchName   string
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
