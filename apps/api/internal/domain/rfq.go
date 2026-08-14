package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// RFQStatus is the pre-quote lifecycle state.
type RFQStatus string

const (
	RFQStatusReceived  RFQStatus = "RECEIVED"
	RFQStatusGenerated RFQStatus = "GENERATED"
)

// RFQ is the original request the quote is built from.
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

// NewRFQ is the input for creating an RFQ source record.
type NewRFQ struct {
	BranchID    uuid.UUID
	ClientID    *uuid.UUID
	ChannelID   uuid.UUID
	RawText     *string
	Status      RFQStatus
	WorkType    *string
	ClientLabel *string
}

// RFQStatusChange records an RFQ lifecycle transition.
type RFQStatusChange struct {
	ID             uuid.UUID
	AccountID      uuid.UUID
	RFQID          uuid.UUID
	PreviousStatus *RFQStatus
	NewStatus      RFQStatus
	UserID         *uuid.UUID
	ChangedAt      time.Time
	CreatedAt      time.Time
}

// ExtractedRFQLine is one schema-forced line item proposed from informal RFQ text.
type ExtractedRFQLine struct {
	RequestedDescription string
	Quantity             decimal.Decimal
	Unit                 *string
	QuantityRationale    *string
}

// RFQExtractor parses informal RFQ text into structured line items.
type RFQExtractor interface {
	Extract(ctx context.Context, raw string) ([]ExtractedRFQLine, error)
}

// TextRFQDraftInput creates the first reviewable quote draft from plain RFQ text.
type TextRFQDraftInput struct {
	ChannelID   uuid.UUID
	ClientID    *uuid.UUID
	ClientLabel *string
	RawText     string
	WorkType    *string
}

// WhatsAppMockRFQInput simulates one inbound WhatsApp text message in development.
type WhatsAppMockRFQInput struct {
	ChannelID   *uuid.UUID
	From        string
	ProfileName *string
	Text        string
}

// TextRFQDraft is the persisted result of the RFQ text pipeline.
type TextRFQDraft struct {
	RFQ     RFQ
	Quote   Quote
	Version QuoteVersion
	Items   []QuoteItem
}
