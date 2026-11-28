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
	RFQStatusReceived RFQStatus = "RECEIVED"
	// RFQStatusFailed is terminal and never carries a quote: reading the order never finished,
	// so the seller works it by hand instead of waiting on a pipeline that already gave up.
	RFQStatusFailed    RFQStatus = "FAILED"
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
	ArchivedAt  *time.Time
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
	ID          *uuid.UUID
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
	SellerID    *uuid.UUID
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
	ID          uuid.UUID
	ClientID    *uuid.UUID
	ClientLabel *string // display name: ficha client name when set, else rfq.client_label.
	CreatedAt   time.Time
	Channel     string // lowercase channel_type: whatsapp, email, webapp, manual_entry.
	SellerID    *uuid.UUID
	SellerName  string
	BranchID    uuid.UUID
	BranchName  string
	QuoteID     *uuid.UUID // NULL until the RFQ has a quote; the archive endpoints key off it.
	QuoteNumber *int64
	ItemCount   int
	// ReviewCount is the current version's lines matching did not settle, which the seller must.
	ReviewCount   int
	Total         *string // decimal string from quote_version.total; NULL when no priced version.
	Status        string  // merged: rfq.status when no quote, otherwise quote.current_status.
	ArchivedAt    *time.Time
	NeedsFollowup bool
	// FollowupFlaggedAt is when the quote was flagged for follow-up; NULL while it is not.
	FollowupFlaggedAt *time.Time
	Closure           SaleClosure
	// AcceptanceUnconfirmed is an acceptance read from a reply that waits for the seller.
	AcceptanceUnconfirmed bool
	// CustomerReply is the latest unread reply on a SENT quote that changed nothing; nil when none.
	CustomerReply *CustomerReply
}

// CustomerReply is a reply recorded on a SENT quote without changing it, waiting for the seller.
type CustomerReply struct {
	Reason HandlerNoMatchReason
	// ReplyAcceptance is set on an acceptance the backend weighed; a SENT quote only carries SIGNAL.
	ReplyAcceptance *ReplyAcceptance
	CreatedAt       time.Time
}

// SaleComplete reports whether the order is a fully closed sale.
func (i RfqListItem) SaleComplete() bool {
	return SaleComplete(QuoteStatus(i.Status), i.Closure)
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
	// QuantityRationale is why the quantity is what it is: a seller reads it instead of reopening
	// the message, and it names what is missing. Empty when the quantity had to be discarded.
	QuantityRationale   string
	SearchDescription   string
	SelectionPreference RFQSelectionPreference
}

// RFQExtraction is one reproducible model answer and the metadata needed to compare it with
// later prompt or model revisions.
type RFQExtraction struct {
	// Kind is what the message is; anything but ORDER with no lines is never an order.
	Kind          RFQMessageKind
	Lines         []ExtractedRFQLine
	Usage         GenerationUsage
	PromptVersion string
	SchemaVersion string
}

// RFQMessageKind is what the extractor read a message as, before any line is matched.
type RFQMessageKind string

const (
	RFQMessageOrder    RFQMessageKind = "ORDER"
	RFQMessageGreeting RFQMessageKind = "GREETING"
	RFQMessageQuestion RFQMessageKind = "QUESTION"
	RFQMessageOther    RFQMessageKind = "OTHER"
)

// RFQExtractor turns informal RFQ text into the lines a quote draft is built from. It is a
// feature port: its adapter owns the prompt and the schema and reaches the model through
// StructuredGenerator, never a provider SDK.
type RFQExtractor interface {
	Extract(ctx context.Context, raw string) (*RFQExtraction, error)
}

// RFQContentExtractor reads an order that arrived as a file. A photographed list and a PDF go
// to the model as they are, so they are blocks rather than text: flattening them first would
// throw away the layout the model reads a materials table from.
type RFQContentExtractor interface {
	ExtractFromContent(ctx context.Context, blocks []Content,
		examples []RFQInterpretationExample) (*RFQExtraction, error)
}

// TextRFQDraftInput is one plain-text order to run through the RFQ pipeline.
type TextRFQDraftInput struct {
	ChannelID   uuid.UUID
	ClientID    *uuid.UUID
	ClientLabel *string
	RawText     string
	WorkType    *string
}

// InboundTextRFQDraftInput is the text payload an external channel delivered after routing.
type InboundTextRFQDraftInput struct {
	ClientID    *uuid.UUID
	ClientLabel *string
	RawText     string
	WorkType    *string
}

// FileRFQDraftInput is one order that arrived as a file — a photo of a handwritten list, a PDF,
// a spreadsheet, a voice note — to run through the RFQ pipeline.
type FileRFQDraftInput struct {
	ChannelID   uuid.UUID
	ClientID    *uuid.UUID
	ClientLabel *string
	WorkType    *string
	// Note is what the seller typed alongside the file, and is optional. It reaches the model
	// as context, never as a replacement for what the file says.
	Note *string
	File AttachmentUpload
	// Filename is the client's own, kept because the transcriber reads its extension to pick
	// a decoder and the spreadsheet reader to pick a parser.
	Filename string
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
	// NonOrder is set, and RFQ is empty, when a channel message turned out not to be an order.
	NonOrder *InboundChannelMessage
}

// RfqDetail is the full detail view projection of one RFQ plus its associated
// quote data, items, and alternatives. This is what the detail endpoint returns.
type RfqDetail struct {
	Rfq           RfqListItem
	PublicContact *PublicRFQContact
	// OriginalText is the order exactly as it arrived, on every channel.
	OriginalText      *string
	PublicAttachments []AttachmentLink
	// InboundWhatsAppPhone is the sender that opened this WhatsApp RFQ, when available.
	InboundWhatsAppPhone *string
	InboundEmail         *string
	Quote                *Quote
	Version              *QuoteVersion
	Items                []QuoteItem
	Alternatives         map[uuid.UUID][]QuoteItemAlternative
	RFQStatusChanges     []RFQStatusChange
	QuoteStatusChanges   []QuoteStatusChange
	Deliveries           []QuoteSend
	// Discounts are the current version's discount applications; empty when the version
	// has none or the discount surface is not wired.
	Discounts []QuoteDiscount
	// ChangesRequested is the frozen-vs-draft comparison for a CHANGE_REQUESTED quote;
	// nil until a frozen predecessor exists.
	ChangesRequested *ChangeRequestDiff
	// ClientActions are the customer responses recorded against the quote's versions,
	// oldest first; empty when none were recorded or the surface is not wired.
	ClientActions []ClientAction
	// Conversation is the customer's messages after a send and what the handler did with them;
	// nil when there are none.
	Conversation *Conversation
}

// AcceptedByReply reports whether the quote stands ACCEPTED on an acceptance read from a reply of
// its current cycle: one a reactivation reopened no longer counts.
func (d RfqDetail) AcceptedByReply() bool {
	if d.Quote == nil || d.Quote.CurrentStatus != QuoteStatusAccepted || d.Quote.CurrentVersionID == nil {
		return false
	}
	for _, action := range d.ClientActions {
		if action.Type == ClientActionAccept && action.VersionID == *d.Quote.CurrentVersionID &&
			action.MessageBatchID != nil && action.RevokedAt == nil && !d.reopenedAfter(action.CreatedAt) {
			return true
		}
	}
	return false
}

// reopenedAfter reports whether a reactivation moved the quote off an answer after at.
func (d RfqDetail) reopenedAfter(at time.Time) bool {
	for _, change := range d.QuoteStatusChanges {
		if change.PreviousStatus != nil && change.ChangedAt.After(at) &&
			(*change.PreviousStatus == QuoteStatusAccepted || *change.PreviousStatus == QuoteStatusRejected) &&
			(change.NewStatus == QuoteStatusQuoted || change.NewStatus == QuoteStatusChangeRequested) {
			return true
		}
	}
	return false
}
