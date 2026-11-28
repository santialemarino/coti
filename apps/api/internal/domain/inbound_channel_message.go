package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// InboundTextMessage is one text message verified and parsed from an external channel provider.
type InboundTextMessage struct {
	ExternalMessageID     string
	DestinationIdentifier string
	SenderID              string
	SenderLabel           *string
	Body                  string
	// ConversationOnly marks a message, such as a reaction, that never starts an order.
	ConversationOnly bool
	// Media is the file a message carried, whose Body is then its caption and may be empty.
	Media              *InboundMedia
	ProviderReceivedAt time.Time
	Payload            []byte
}

// InboundMedia is a file a provider holds for a message: only its reference arrives with the
// message, and the bytes are fetched later with the channel's credentials.
type InboundMedia struct {
	ProviderID string
	MediaType  string
	Filename   string
	Kind       InboundMediaKind
}

// InboundMediaKind is how the client sent a file, which decides what the seller is told about it.
type InboundMediaKind string

const (
	InboundMediaImage    InboundMediaKind = "IMAGE"
	InboundMediaAudio    InboundMediaKind = "AUDIO"
	InboundMediaDocument InboundMediaKind = "DOCUMENT"
	InboundMediaVideo    InboundMediaKind = "VIDEO"
)

// WhatsAppMediaKeyPrefix marks an attachment whose file WhatsApp still holds: the rest of the key
// is the media ID, resolved when the file is about to be read, since its download link expires
// within minutes of being issued.
const WhatsAppMediaKeyPrefix = "whatsapp-media:"

// FetchedMedia is a file downloaded from the provider that held it.
type FetchedMedia struct {
	MediaType string
	Data      []byte
}

// WhatsAppMediaFetcher downloads a file a client sent a branch number, refusing one past maxBytes.
type WhatsAppMediaFetcher interface {
	FetchMedia(ctx context.Context, accessToken, mediaID string, maxBytes int64) (*FetchedMedia, error)
}

// InboundEmailMessage carries one mailbox message and its original RFC 5322 bytes.
type InboundEmailMessage struct {
	ExternalMessageID     string
	DestinationIdentifier string
	SenderID              string
	SenderLabel           string
	Subject               string
	Body                  string
	Original              []byte
	Attachments           []InboundEmailAttachment
	ProviderReceivedAt    time.Time
	// InternetMessageID is the validated RFC 5322 Message-ID, angle brackets included.
	InternetMessageID string
	// Automated is set by Auto-Submitted, Precedence bulk/list/junk or mailing-list headers.
	Automated bool
}

// InboundEmailAttachment is one decoded MIME part of an incoming email.
type InboundEmailAttachment struct {
	Filename    string
	ContentType string
	Inline      bool
	Content     []byte
}

// InboundMessageKind is what a message that opened no order was read as.
type InboundMessageKind string

const (
	InboundMessageGreeting  InboundMessageKind = "GREETING"
	InboundMessageQuestion  InboundMessageKind = "QUESTION"
	InboundMessageOther     InboundMessageKind = "OTHER"
	InboundMessageAutomated InboundMessageKind = "AUTOMATED"
)

// InboundMessageKindFor maps an extractor's reading of a message that named no material.
func InboundMessageKindFor(kind RFQMessageKind) InboundMessageKind {
	switch kind {
	case RFQMessageGreeting:
		return InboundMessageGreeting
	case RFQMessageQuestion:
		return InboundMessageQuestion
	default:
		return InboundMessageOther
	}
}

// InboundChannelMessage retains a provider event and the RFQ or conversation message it became.
// At most one of RFQID and QuoteMessageID is set; a message with neither carries its Kind.
type InboundChannelMessage struct {
	ID                 uuid.UUID
	AccountID          uuid.UUID
	BranchID           uuid.UUID
	ChannelID          uuid.UUID
	RFQID              *uuid.UUID
	QuoteMessageID     *uuid.UUID
	Kind               *InboundMessageKind
	ExternalMessageID  string
	SenderID           string
	SenderLabel        *string
	Body               string
	Payload            []byte
	ProviderReceivedAt time.Time
	DismissedAt        *time.Time
	CreatedAt          time.Time
}

// NewInboundChannelMessage is the provider event retained before its RFQ or conversation work
// runs. Exactly one of RFQID, QuoteMessageID and Kind is set.
type NewInboundChannelMessage struct {
	ID             uuid.UUID
	RFQID          *uuid.UUID
	QuoteMessageID *uuid.UUID
	Kind           *InboundMessageKind
	// Dismissed files a non-order the seller never needs to see, such as a name reply.
	Dismissed          bool
	ExternalMessageID  string
	SenderID           string
	SenderLabel        *string
	Body               string
	Payload            []byte
	ProviderReceivedAt time.Time
}
