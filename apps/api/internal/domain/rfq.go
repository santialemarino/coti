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
	// QuantitySourceUnresolved is the schema's escape value, so "I cannot tell how many" is a
	// structurally valid answer instead of an invented number.
	QuantitySourceUnresolved QuantitySource = "UNRESOLVED"
)

// RFQ is the original request a quote is built from. The raw text is stored before anything is
// extracted from it, so a quote can always be reconstructed from its source.
type RFQ struct {
	ID          uuid.UUID
	AccountID   uuid.UUID
	BranchID    uuid.UUID
	ClientID    *uuid.UUID
	ClientLabel *string
	ChannelID   uuid.UUID
	RawText     *string
	Status      RFQStatus
	WorkType    *string
	ReceivedAt  time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
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

// NewRfq is the input for a manual entry: free text, structured lines, or both. At
// least one must be present, enforced by the service.
type NewRfq struct {
	RawText     *string
	WorkType    *string
	ClientLabel *string
	Items       []NewRfqItem
}

// NewRfqItem is one structured line the seller typed. Quantity is a decimal carried
// end to end; the price snapshots stay NULL until the pricing step runs.
type NewRfqItem struct {
	ProductID            *uuid.UUID // a catalog product the seller picked.
	RequestedDescription string
	Quantity             decimal.Decimal // NUMERIC(14,2), greater than zero.
	Unit                 *string
}

// RfqCreation is what the manual-entry endpoint persists as one atomic unit: the RFQ
// born GENERATED, its quote born DRAFT, and version v1 of the quote.
type RfqCreation struct {
	Rfq     RFQ
	Quote   Quote
	Version QuoteVersion
}

// RfqListItem is a denormalized projection of rfq + quote + branch + seller for the
// Backoffice list view. The display status merges rfq.status and quote.current_status
// into a single timeline the UI can render.
type RfqListItem struct {
	ID            uuid.UUID
	ClientID      *uuid.UUID
	ClientLabel   *string // display name: ficha client name when set, else rfq.client_label.
	CreatedAt     time.Time
	Channel       string // lowercase channel_type: whatsapp, email, webapp, manual_entry.
	SellerID      *uuid.UUID
	SellerName    string
	BranchID      uuid.UUID
	BranchName    string
	ItemCount     int
	Total         *string // decimal string from quote_version.total; NULL when no priced version.
	Status        string  // merged: rfq.status when no quote, otherwise quote.current_status.
	ArchivedAt    *time.Time
	NeedsFollowup bool
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
	// Quantity means nothing unless Source is EXPLICIT or DERIVED: on the escape value it is
	// zeroed rather than trusted.
	Quantity decimal.Decimal
	Unit     *string
	Source   QuantitySource
	// QuantityRationale is why the quantity is what it is, and is required on every line: a
	// seller reads it instead of reopening the message, and it names what is missing.
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

// TextRFQDraft is what the pipeline persisted. Quote, Version and Items are absent when no
// material was read at all: the text is kept and the RFQ stays RECEIVED.
type TextRFQDraft struct {
	RFQ     RFQ
	Quote   *Quote
	Version *QuoteVersion
	Items   []QuoteItem
	// Alternatives are the candidates each flagged line was decided from, keyed by line id.
	Alternatives map[uuid.UUID][]QuoteItemAlternative
}

// RfqDetail is the full detail view projection of one RFQ plus its associated
// quote data, items, and alternatives. This is what the detail endpoint returns.
type RfqDetail struct {
	Rfq          RfqListItem
	Quote        *Quote
	Version      *QuoteVersion
	Items        []QuoteItem
	Alternatives map[uuid.UUID][]QuoteItemAlternative
}
