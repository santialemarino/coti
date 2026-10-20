package domain

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// QuoteStatus is the lifecycle state a quote carries once it exists.
type QuoteStatus string

const (
	QuoteStatusDraft           QuoteStatus = "DRAFT"
	QuoteStatusQuoted          QuoteStatus = "QUOTED"
	QuoteStatusSent            QuoteStatus = "SENT"
	QuoteStatusChangeRequested QuoteStatus = "CHANGE_REQUESTED"
	QuoteStatusAccepted        QuoteStatus = "ACCEPTED"
	QuoteStatusRejected        QuoteStatus = "REJECTED"
)

// QuoteReactivationMode selects whether a closed quote reuses its frozen version or opens a new draft.
type QuoteReactivationMode string

const (
	QuoteReactivationResend QuoteReactivationMode = "RESEND"
	QuoteReactivationEdit   QuoteReactivationMode = "EDIT"
)

// Quote is the seller-facing quote created from one RFQ. current_status is a
// backend-exclusive derived cache, recomputed on each transition, never set by a
// human or the AI.
type Quote struct {
	ID                uuid.UUID
	AccountID         uuid.UUID
	Number            int64
	BranchID          uuid.UUID
	ClientID          *uuid.UUID
	RFQID             uuid.UUID
	SellerID          *uuid.UUID
	CurrentVersionID  *uuid.UUID
	CurrentStatus     QuoteStatus
	ExpiresAt         *time.Time
	ArchivedAt        *time.Time
	NeedsFollowup     bool
	FollowupFlaggedAt *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
	// PaidAt and PaidByUserID record «Cobrado» on an accepted sale: a fact, not a status.
	PaidAt       *time.Time
	PaidByUserID *uuid.UUID
	// InvoiceWaivedAt and InvoiceWaivedByUserID record «No requiere factura».
	InvoiceWaivedAt       *time.Time
	InvoiceWaivedByUserID *uuid.UUID
}

// SaleClosure is what has closed an accepted sale so far, with who did it. It is derived, never
// stored as a status.
type SaleClosure struct {
	PaidAt          *time.Time
	PaidBy          *string // the user's name.
	InvoiceWaivedAt *time.Time
	InvoiceWaivedBy *string
	// Invoiced is whether an issued invoice no credit note cancelled closes the sale here.
	Invoiced bool
	// InvoicePending is whether a voucher waits for ARCA's answer, in any environment.
	InvoicePending bool
	// InvoicingEnabled is the account switch; an account that does not invoice needs no invoice.
	InvoicingEnabled bool
}

// SaleComplete reports whether a sale in status is fully closed: accepted, paid, and invoiced or
// waived, or with invoicing off for the account. A voucher still waiting for ARCA keeps it open.
func SaleComplete(status QuoteStatus, closure SaleClosure) bool {
	if status != QuoteStatusAccepted || closure.PaidAt == nil || closure.InvoicePending {
		return false
	}
	return closure.Invoiced || closure.InvoiceWaivedAt != nil || !closure.InvoicingEnabled
}

// AutoArchiveStatuses are the statuses the inactivity sweep archives: the stalls follow-up chases,
// waiting on the seller (DRAFT, QUOTED) or on the client (SENT, only once its offer has lapsed). A
// change request is a live demand from the client, and a closed quote is a record of the outcome,
// so neither is put away unasked.
var AutoArchiveStatuses = []QuoteStatus{QuoteStatusDraft, QuoteStatusQuoted, QuoteStatusSent}

// ArchivedQuote is one quote the inactivity sweep archived, with the activity it went quiet after.
type ArchivedQuote struct {
	AccountID      uuid.UUID
	BranchID       uuid.UUID
	QuoteID        uuid.UUID
	Number         int64
	Status         QuoteStatus
	LastActivityAt time.Time
}

// NewQuote is the input for creating a quote shell.
type NewQuote struct {
	BranchID      uuid.UUID
	ClientID      *uuid.UUID
	RFQID         uuid.UUID
	SellerID      *uuid.UUID
	CurrentStatus QuoteStatus
	ExpiresAt     *time.Time
}

// QuoteVersion is one reviewable snapshot of a quote. total = Σ item subtotals −
// Σ discounts. Append-only: it has no updated_at.
type QuoteVersion struct {
	ID            uuid.UUID
	AccountID     uuid.UUID
	QuoteID       uuid.UUID
	AuthorID      *uuid.UUID
	VersionNumber int
	Currency      string
	Total         decimal.Decimal // NUMERIC(14,2).
	IsImmutable   bool            // draft = false; frozen = true.
	FrozenAt      *time.Time
	Comment       *string
	CreatedAt     time.Time
}

// NewQuoteVersion is the input for creating a quote version.
type NewQuoteVersion struct {
	QuoteID       uuid.UUID
	AuthorID      *uuid.UUID
	VersionNumber int
	Currency      string
	Total         decimal.Decimal
	IsImmutable   bool
	Comment       *string
}

// QuoteItem is one material line inside a quote version. The item does not carry
// its discount; a discount is its own entity. price snapshots are NULL until the
// pricing step runs. ProductCode, ProductName, and ProductUnit are populated only
// when the item is loaded with a product JOIN (ListItemsWithProduct); bare
// ListItems leaves them nil.
type QuoteItem struct {
	ID                   uuid.UUID
	AccountID            uuid.UUID
	VersionID            uuid.UUID
	ProductID            *uuid.UUID
	RequestedDescription string
	Quantity             decimal.Decimal // NUMERIC(14,2).
	Unit                 *string
	UnitPriceSnapshot    decimal.NullDecimal // NUMERIC(14,2).
	MinPriceSnapshot     decimal.NullDecimal // discount-engine floor, snapshotted.
	Subtotal             decimal.NullDecimal // NUMERIC(14,2).
	ConfidenceScore      decimal.NullDecimal
	MatchStatus          ItemMatchStatus
	QuantityRationale    *string
	ProductCode          *string
	ProductName          *string
	ProductUnit          *string
	CreatedAt            time.Time
	// SourceItemID is the sent line a draft line was copied from; nil for one added to the draft.
	SourceItemID *uuid.UUID
}

// NewQuoteItem is the input for creating a quote item.
type NewQuoteItem struct {
	// ID is chosen by the caller so a line's candidates can name it before either is written.
	ID                   uuid.UUID
	ProductID            *uuid.UUID
	RequestedDescription string
	Quantity             decimal.Decimal
	Unit                 *string
	UnitPriceSnapshot    decimal.NullDecimal
	MinPriceSnapshot     decimal.NullDecimal
	Subtotal             decimal.NullDecimal
	ConfidenceScore      decimal.NullDecimal
	MatchStatus          ItemMatchStatus
	QuantityRationale    *string
	SourceItemID         *uuid.UUID
}

// QuoteItemAlternativeType is what an alternative offers in place of a line's product.
type QuoteItemAlternativeType string

const (
	QuoteItemAlternativeTypeProduct QuoteItemAlternativeType = "PRODUCT"
	QuoteItemAlternativeTypeCombo   QuoteItemAlternativeType = "COMBO"
)

// QuoteItemAlternativeOrigin is who offered an alternative.
type QuoteItemAlternativeOrigin string

const (
	QuoteItemAlternativeOriginAI     QuoteItemAlternativeOrigin = "AI"
	QuoteItemAlternativeOriginSeller QuoteItemAlternativeOrigin = "SELLER"
)

// QuoteItemAlternative is one candidate offered for a quote line, carrying the catalog identity a
// seller needs to tell it apart.
type QuoteItemAlternative struct {
	ID               uuid.UUID
	AccountID        uuid.UUID
	QuoteItemID      uuid.UUID
	ProductID        *uuid.UUID
	ComboID          *uuid.UUID
	Type             QuoteItemAlternativeType
	Origin           QuoteItemAlternativeOrigin
	Rank             int
	ConfidenceScore  decimal.NullDecimal
	PriceSnapshot    decimal.NullDecimal
	ApprovedBySeller bool
	ChosenByClient   bool
	Code             *string
	CanonicalName    *string
	Unit             *string
	CreatedAt        time.Time
}

// NewQuoteItemAlternative is the input for creating a quote item alternative.
type NewQuoteItemAlternative struct {
	QuoteItemID     uuid.UUID
	ProductID       *uuid.UUID
	ComboID         *uuid.UUID
	Type            QuoteItemAlternativeType
	Origin          QuoteItemAlternativeOrigin
	Rank            int
	ConfidenceScore decimal.NullDecimal
	PriceSnapshot   decimal.NullDecimal
}

// QuoteItemAlternativePricing is the frozen catalog price for one offered product.
type QuoteItemAlternativePricing struct {
	AlternativeID uuid.UUID
	PriceSnapshot decimal.NullDecimal
}

// QuoteItemUpdate is the mutable surface of a quote item. All fields are optional:
// only present fields are written.
type QuoteItemUpdate struct {
	ProductID            *uuid.UUID
	RequestedDescription *string
	Quantity             *decimal.Decimal
	Unit                 *string
	UnitPriceSnapshot    *decimal.Decimal
	// MinPriceSnapshot is the floor that comes with a branch price; nil leaves the line without one.
	MinPriceSnapshot *decimal.Decimal
	// ConfirmMatch settles a flagged line on the product it already carries, keeping its price.
	ConfirmMatch bool
}

// IsEditableStatus returns true when the quote status allows item mutations.
func IsEditableStatus(status QuoteStatus) bool {
	switch status {
	case QuoteStatusDraft, QuoteStatusQuoted, QuoteStatusChangeRequested:
		return true
	default:
		return false
	}
}

// QuoteItemCreate is the input for adding a new line to a draft quote version.
type QuoteItemCreate struct {
	ProductID            *uuid.UUID
	RequestedDescription string
	Quantity             decimal.Decimal
	Unit                 *string
}

// QuoteItemPricing is one line's frozen valuation, written when the seller accepts the
// materials.
type QuoteItemPricing struct {
	ItemID            uuid.UUID
	UnitPriceSnapshot decimal.NullDecimal
	MinPriceSnapshot  decimal.NullDecimal
	Subtotal          decimal.NullDecimal
}

// QuoteStatusChange records a quote lifecycle transition.
type QuoteStatusChange struct {
	ID             uuid.UUID
	AccountID      uuid.UUID
	QuoteID        uuid.UUID
	PreviousStatus *QuoteStatus
	NewStatus      QuoteStatus
	UserID         *uuid.UUID
	ChangedAt      time.Time
	CreatedAt      time.Time
}

// PricedQuote is the result of the DRAFT to QUOTED transition.
type PricedQuote struct {
	Quote           Quote
	Version         QuoteVersion
	Items           []QuoteItem
	UnpricedItemIDs []uuid.UUID
	Alternatives    map[uuid.UUID][]QuoteItemAlternative
}

// QuoteReference is how a quote is named to anyone outside the product — on the customer's page,
// in the seller's mail, in the subject line. One place, because it appears on several surfaces and
// a quote called COT-000042 in one and COT-42 in another is two quotes to whoever reads them.
func QuoteReference(number int64) string {
	return fmt.Sprintf("COT-%06d", number)
}
