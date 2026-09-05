package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// SendFormat is the representation delivered through a channel.
type SendFormat string

const SendFormatWebAppLink SendFormat = "WEBAPP_LINK"

// SendTrackingStatus is the durable outcome of one channel attempt.
type SendTrackingStatus string

const (
	SendTrackingStatusPending   SendTrackingStatus = "PENDING"
	SendTrackingStatusSent      SendTrackingStatus = "SENT"
	SendTrackingStatusDelivered SendTrackingStatus = "DELIVERED"
	SendTrackingStatusViewed    SendTrackingStatus = "VIEWED"
	SendTrackingStatusFailed    SendTrackingStatus = "FAILED"
)

// QuoteSend is one channel-specific attempt to deliver one frozen quote version.
type QuoteSend struct {
	ID                uuid.UUID
	AccountID         uuid.UUID
	VersionID         uuid.UUID
	ChannelID         uuid.UUID
	ChannelType       ChannelType
	IdempotencyKey    uuid.UUID
	Destination       string
	ProviderReference *string
	PublicToken       string
	PublicURL         string
	Format            SendFormat
	ValidityDays      int
	SentAt            *time.Time
	ExpiresAt         *time.Time
	TrackingStatus    SendTrackingStatus
	CreatedAt         time.Time
}

// NewQuoteSend is one pending channel attempt created before contacting a provider.
type NewQuoteSend struct {
	ID             uuid.UUID
	VersionID      uuid.UUID
	ChannelID      uuid.UUID
	IdempotencyKey uuid.UUID
	Destination    string
	PublicToken    string
	Format         SendFormat
	ValidityDays   int
}

// QuoteSendOutcome closes one pending channel attempt.
type QuoteSendOutcome struct {
	ID                uuid.UUID
	Status            SendTrackingStatus
	ProviderReference *string
	SentAt            *time.Time
	ExpiresAt         *time.Time
}

// QuoteDeliveryInput is the seller-approved delivery request.
type QuoteDeliveryInput struct {
	IdempotencyKey uuid.UUID
	Phone          string
	Email          *string
	ExpiryDays     *int
}

// QuoteDeliveryResult is the committed result of a delivery operation.
type QuoteDeliveryResult struct {
	QuoteID       uuid.UUID
	VersionID     uuid.UUID
	CurrentStatus QuoteStatus
	ExpiresAt     *time.Time
	Deliveries    []QuoteSend
	Replay        bool
}

// QuoteWhatsAppMessage is the provider-neutral message sent to the required phone number.
type QuoteWhatsAppMessage struct {
	DeliveryID uuid.UUID
	To         string
	Body       string
	PublicURL  string
}

// DeliveryReceipt is the external provider's durable acknowledgement.
type DeliveryReceipt struct {
	ProviderReference string
}

// QuoteWhatsAppSender delivers a seller-approved public quote link through WhatsApp.
type QuoteWhatsAppSender interface {
	SendQuote(ctx context.Context, message QuoteWhatsAppMessage) (*DeliveryReceipt, error)
}

// PublicQuoteSend is the minimal sessionless state exposed for a delivery token.
type PublicQuoteSend struct {
	Status    string
	ExpiresAt time.Time
}

// PendingQuoteEvaluation identifies a sent frozen version lacking its deterministic label.
type PendingQuoteEvaluation struct {
	AccountID uuid.UUID
	BranchID  uuid.UUID
	QuoteID   uuid.UUID
	VersionID uuid.UUID
}
