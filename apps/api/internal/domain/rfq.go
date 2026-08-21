package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// RFQStatus is the pre-quote lifecycle state, living on the rfq entity. Manual
// entry skips RECEIVED: the seller types the request and it is GENERATED at once.
type RFQStatus string

const (
	RFQStatusReceived  RFQStatus = "RECEIVED"
	RFQStatusGenerated RFQStatus = "GENERATED"
)

// Rfq is what the client asked for. Separate from quote: the UI stepper is a
// projection over both, not a third entity.
type Rfq struct {
	ID          uuid.UUID
	AccountID   uuid.UUID
	BranchID    uuid.UUID
	ClientID    *uuid.UUID
	ClientLabel *string // loose name on manual entry, when there is no client record.
	ChannelID   uuid.UUID
	RawText     *string
	Status      RFQStatus
	WorkType    *string
	ReceivedAt  time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
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
	Rfq     Rfq
	Quote   Quote
	Version QuoteVersion
}

// RfqListItem is a denormalized projection of rfq + quote + branch + seller for the
// Backoffice list view. The display status merges rfq.status and quote.current_status
// into a single timeline the UI can render.
type RfqListItem struct {
	ID          uuid.UUID
	ClientLabel *string
	CreatedAt   time.Time
	Channel     string // lowercase channel_type: whatsapp, email, webapp, manual_entry.
	SellerName  string
	BranchName  string
	ItemCount   int
	Total       *string // decimal string from quote_version.total; NULL when no priced version.
	Status      string // merged: rfq.status when no quote, otherwise quote.current_status.
	ArchivedAt  *time.Time
}
