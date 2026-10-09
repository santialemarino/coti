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

// InboundChannelMessage retains the provider event that materialized an RFQ.
type InboundChannelMessage struct {
	ID                 uuid.UUID
	AccountID          uuid.UUID
	BranchID           uuid.UUID
	ChannelID          uuid.UUID
	RFQID              uuid.UUID
	ExternalMessageID  string
	SenderID           string
	SenderLabel        *string
	Body               string
	Payload            []byte
	ProviderReceivedAt time.Time
	CreatedAt          time.Time
}

// NewInboundChannelMessage is the provider event retained before an RFQ pipeline runs.
type NewInboundChannelMessage struct {
	ID                 uuid.UUID
	RFQID              uuid.UUID
	ExternalMessageID  string
	SenderID           string
	SenderLabel        *string
	Body               string
	Payload            []byte
	ProviderReceivedAt time.Time
}
