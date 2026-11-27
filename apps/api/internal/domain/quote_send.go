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

// QuoteDeliveryPurpose distinguishes a quote publication from a seller-confirmed reminder.
type QuoteDeliveryPurpose string

const (
	QuoteDeliveryPurposeQuote    QuoteDeliveryPurpose = "QUOTE"
	QuoteDeliveryPurposeFollowup QuoteDeliveryPurpose = "FOLLOWUP"
)

// QuoteDeliveryMode names the seller action behind a delivery; each mode has its own state and
// medium rules.
type QuoteDeliveryMode string

const (
	// QuoteDeliveryModeFirst publishes a QUOTED version.
	QuoteDeliveryModeFirst QuoteDeliveryMode = "FIRST"
	// QuoteDeliveryModeOtherMedium reaches the customer of a SENT quote through a medium not used yet.
	QuoteDeliveryModeOtherMedium QuoteDeliveryMode = "OTHER_MEDIUM"
	// QuoteDeliveryModeResend repeats a SENT quote through any medium, keeping its expiry.
	QuoteDeliveryModeResend QuoteDeliveryMode = "RESEND"
	// QuoteDeliveryModeFollowup sends a seller-confirmed reminder for a SENT quote.
	QuoteDeliveryModeFollowup QuoteDeliveryMode = "FOLLOWUP"
)

// Valid reports whether m is a known delivery mode.
func (m QuoteDeliveryMode) Valid() bool {
	switch m {
	case QuoteDeliveryModeFirst, QuoteDeliveryModeOtherMedium, QuoteDeliveryModeResend,
		QuoteDeliveryModeFollowup:
		return true
	}
	return false
}

// QuoteDeliveryPresetSource says where a delivery preset's destinations come from.
type QuoteDeliveryPresetSource string

const (
	// QuoteDeliveryPresetVersion reuses the destinations the current version reached.
	QuoteDeliveryPresetVersion QuoteDeliveryPresetSource = "VERSION"
	// QuoteDeliveryPresetPreviousVersion reuses the destinations of the quote's last sent version.
	QuoteDeliveryPresetPreviousVersion QuoteDeliveryPresetSource = "PREVIOUS_VERSION"
	// QuoteDeliveryPresetClientLast reuses the destinations of the client's last sent quote.
	QuoteDeliveryPresetClientLast QuoteDeliveryPresetSource = "CLIENT_LAST"
	// QuoteDeliveryPresetNone leaves the selection to the WhatsApp-first rule.
	QuoteDeliveryPresetNone QuoteDeliveryPresetSource = "NONE"
)

// QuoteDeliveryPreset is what a send dialog preselects for one mode, and the destinations the
// current send cycle already reached, which «otro medio» must not offer again.
type QuoteDeliveryPreset struct {
	Source     QuoteDeliveryPresetSource
	Phones     []string
	Emails     []string
	UsedPhones []string
	UsedEmails []string
}

// QuoteSend is one channel-specific attempt to deliver one frozen quote version.
type QuoteSend struct {
	Purpose     QuoteDeliveryPurpose
	Message     *string
	ID          uuid.UUID
	AccountID   uuid.UUID
	VersionID   uuid.UUID
	ChannelID   uuid.UUID
	ChannelType ChannelType
	// ChannelIdentifier is the branch mailbox or number the send went through, nil when unset.
	ChannelIdentifier     *string
	IdempotencyKey        uuid.UUID
	ClientAssociationHash string
	Destination           string
	ProviderReference     *string
	PublicToken           string
	PublicURL             string
	Format                SendFormat
	ValidityDays          int
	SentAt                *time.Time
	ExpiresAt             *time.Time
	TrackingStatus        SendTrackingStatus
	CreatedAt             time.Time
}

// NewQuoteSend is one pending channel attempt created before contacting a provider.
type NewQuoteSend struct {
	Purpose               QuoteDeliveryPurpose
	Message               *string
	ClientAssociationHash string
	ID                    uuid.UUID
	VersionID             uuid.UUID
	ChannelID             uuid.UUID
	IdempotencyKey        uuid.UUID
	Destination           string
	PublicToken           string
	Format                SendFormat
	ValidityDays          int
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
	Mode              QuoteDeliveryMode
	Phones            []string
	Emails            []string
	OtherChannelsOnly bool
	FollowupMessage   *string
	ClientAssociation *AssociateQuoteClientInput
	IdempotencyKey    uuid.UUID
	Phone             string
	Email             *string
	ExpiryDays        *int
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
	DeliveryID  uuid.UUID
	To          string
	Body        string
	PublicURL   string
	Credentials WhatsAppDeliveryCredentials
}

// WhatsAppDeliveryCredentials are the sealed channel settings opened only for one provider call.
type WhatsAppDeliveryCredentials struct {
	PhoneNumberID string
	AccessToken   string
}

// ProviderDeliveryStatus is one outcome a channel provider reported for a message it accepted.
type ProviderDeliveryStatus struct {
	DestinationIdentifier string
	ProviderReference     string
	Status                SendTrackingStatus
	ErrorCode             string
	// OutsideWindow is a refusal because the customer last wrote more than 24 hours earlier.
	OutsideWindow bool
	OccurredAt    time.Time
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
