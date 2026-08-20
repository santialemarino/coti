package domain

import (
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

// Quote is the seller-facing quote created from one RFQ.
type Quote struct {
	ID                uuid.UUID
	AccountID         uuid.UUID
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

// QuoteVersion is one reviewable snapshot of a quote.
type QuoteVersion struct {
	ID            uuid.UUID
	AccountID     uuid.UUID
	QuoteID       uuid.UUID
	AuthorID      *uuid.UUID
	VersionNumber int
	Total         decimal.Decimal
	IsImmutable   bool
	Comment       *string
	CreatedAt     time.Time
}

// NewQuoteVersion is the input for creating a quote version.
type NewQuoteVersion struct {
	QuoteID       uuid.UUID
	AuthorID      *uuid.UUID
	VersionNumber int
	Total         decimal.Decimal
	IsImmutable   bool
	Comment       *string
}

// QuoteItem is one material line inside a quote version.
type QuoteItem struct {
	ID                   uuid.UUID
	AccountID            uuid.UUID
	VersionID            uuid.UUID
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
	CreatedAt            time.Time
}

// NewQuoteItem is the input for creating a quote item.
type NewQuoteItem struct {
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
}

// QuoteItemPricing is one line's frozen valuation, written when the seller accepts the
// materials. All three are null together: a line with no product, or whose product the branch
// has no price in force for, keeps its valuation empty rather than valued at zero.
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

// PricedQuote is the result of the DRAFT to QUOTED transition: the quote at its new status, the
// version whose total was just summed, and the lines carrying the prices that were frozen.
type PricedQuote struct {
	Quote   Quote
	Version QuoteVersion
	Items   []QuoteItem
}
