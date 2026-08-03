package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// RFQStatus is the pre-quote lifecycle state, living on the rfq entity.
type RFQStatus string

const (
	RFQStatusGenerated RFQStatus = "GENERATED"
	RFQStatusReceived  RFQStatus = "RECEIVED"
)

// RFQ is the persisted source request before a quote exists.
type RFQ struct {
	ID          uuid.UUID
	AccountID   uuid.UUID
	BranchID    uuid.UUID
	ClientID    *uuid.UUID
	ChannelID   uuid.UUID
	RawText     *string
	Status      RFQStatus
	WorkType    *string
	ReceivedAt  time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
	ClientLabel *string
}

// InboundChannelMessage is one external message received through a configured channel.
type InboundChannelMessage struct {
	ID                uuid.UUID
	AccountID         uuid.UUID
	BranchID          uuid.UUID
	ChannelID         uuid.UUID
	RFQID             *uuid.UUID
	ExternalMessageID string
	ExternalSenderID  string
	Body              string
	RawPayload        json.RawMessage
	ReceivedAt        time.Time
	CreatedAt         time.Time
}

// NewInboundChannelMessage is an external channel message to persist before processing.
type NewInboundChannelMessage struct {
	AccountID         uuid.UUID
	BranchID          uuid.UUID
	ChannelID         uuid.UUID
	ExternalMessageID string
	ExternalSenderID  string
	Body              string
	RawPayload        json.RawMessage
	ReceivedAt        time.Time
}

// NewRFQ is the original client request to persist before extraction and matching.
type NewRFQ struct {
	AccountID   uuid.UUID
	BranchID    uuid.UUID
	ClientID    *uuid.UUID
	ChannelID   uuid.UUID
	RawText     string
	ReceivedAt  time.Time
	ClientLabel *string
}
