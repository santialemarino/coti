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
