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
