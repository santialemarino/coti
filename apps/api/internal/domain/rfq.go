package domain

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// RFQStatus is the pre-quote lifecycle state, living on the rfq entity.
type RFQStatus string

const (
	RFQStatusReceived  RFQStatus = "RECEIVED"
	RFQStatusGenerated RFQStatus = "GENERATED"
)

// QuantitySource is where an extracted line's quantity came from, and it is a closed enum in the
// extraction schema so the model cannot answer outside it.
type QuantitySource string

const (
	// QuantitySourceExplicit is a quantity the client stated.
	QuantitySourceExplicit QuantitySource = "EXPLICIT"
	// QuantitySourceDerived is a quantity computed from what the client stated.
	QuantitySourceDerived QuantitySource = "DERIVED"
	// QuantitySourceUnresolved is the schema's escape value: the material is recognisable but
	// the message carries no quantity that can be defended. It exists so "I cannot tell how many"
	// is a structurally valid answer instead of an invented number.
	QuantitySourceUnresolved QuantitySource = "UNRESOLVED"
)

// RFQ is the original request a quote is built from. The raw text is stored before anything is
// extracted from it, so a quote can always be reconstructed from its source.
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

// ExtractedRFQLine is one material the extractor read out of informal RFQ text.
type ExtractedRFQLine struct {
	// RequestedDescription is what the client wrote, unnormalised, so the seller can read the
	// interpretation against the original.
	RequestedDescription string
	// Quantity is meaningless unless Source is EXPLICIT or DERIVED, and the service zeroes it on
	// the escape value rather than trusting a number the model was told not to produce.
	Quantity decimal.Decimal
	Unit     *string
	Source   QuantitySource
	// QuantityRationale is why the quantity is what it is, in one short defensible sentence. It
	// is required on every line: a seller has to understand the number without reopening the
	// message, and on the escape value it is what says which datum is missing.
	QuantityRationale string
}

// RFQExtractor turns informal RFQ text into the lines a quote draft is built from. It is a
// feature port: its adapter owns the prompt and the schema and reaches the model through
// StructuredGenerator, never a provider SDK.
type RFQExtractor interface {
	Extract(ctx context.Context, raw string) ([]ExtractedRFQLine, error)
}

// TextRFQDraftInput is one plain-text order to run through the RFQ pipeline.
type TextRFQDraftInput struct {
	ChannelID   uuid.UUID
	ClientID    *uuid.UUID
	ClientLabel *string
	RawText     string
	WorkType    *string
}

// WhatsAppMockRFQInput simulates one inbound WhatsApp text message outside production.
type WhatsAppMockRFQInput struct {
	ChannelID   *uuid.UUID
	From        string
	ProfileName *string
	Text        string
}

// TextRFQDraft is what the pipeline persisted. Quote, Version and Items are absent when the
// extractor read no material at all: the text is kept and the RFQ stays RECEIVED, because a
// quote whose generation produced nothing has not reached GENERATED.
type TextRFQDraft struct {
	RFQ     RFQ
	Quote   *Quote
	Version *QuoteVersion
	Items   []QuoteItem
}
