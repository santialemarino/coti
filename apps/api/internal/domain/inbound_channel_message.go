package domain

import (
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
	ProviderReceivedAt    time.Time
	Payload               []byte
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

// InboundChannelMessage retains a provider event and the RFQ or conversation message it became.
// Exactly one of RFQID and QuoteMessageID is set.
type InboundChannelMessage struct {
	ID                 uuid.UUID
	AccountID          uuid.UUID
	BranchID           uuid.UUID
	ChannelID          uuid.UUID
	RFQID              *uuid.UUID
	QuoteMessageID     *uuid.UUID
	ExternalMessageID  string
	SenderID           string
	SenderLabel        *string
	Body               string
	Payload            []byte
	ProviderReceivedAt time.Time
	CreatedAt          time.Time
}

// NewInboundChannelMessage is the provider event retained before its RFQ or conversation work
// runs. Exactly one of RFQID and QuoteMessageID is set.
type NewInboundChannelMessage struct {
	ID                 uuid.UUID
	RFQID              *uuid.UUID
	QuoteMessageID     *uuid.UUID
	ExternalMessageID  string
	SenderID           string
	SenderLabel        *string
	Body               string
	Payload            []byte
	ProviderReceivedAt time.Time
}
