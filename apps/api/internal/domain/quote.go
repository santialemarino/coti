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
	// ID is chosen by the caller so a line's candidates can name it before either is written.
	// Deriving that from the insert's row order instead would rest on an order Postgres does not
	// promise, and a line offering another line's candidates is a wrong answer nothing catches.
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
// seller needs to tell it apart. Code, CanonicalName and Unit are read from the catalog as it
// stands and are not frozen with the line, the same as the product the line itself matched.
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
	QuoteItemID uuid.UUID
	ProductID   *uuid.UUID
	ComboID     *uuid.UUID
	Type        QuoteItemAlternativeType
	Origin      QuoteItemAlternativeOrigin
	// Rank is the matcher's order, best first from one. Nothing else on the row records it:
	// created_at is the transaction's timestamp, shared by every row of one insert.
	Rank            int
	ConfidenceScore decimal.NullDecimal
	// PriceSnapshot stays empty on an AI candidate: nothing has been priced when matching runs,
	// and the price a seller would freeze is the one in force when they choose it.
	PriceSnapshot decimal.NullDecimal
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
	// UnpricedItemIDs are the lines that matched a product the branch cannot price. They are
	// neither NO_MATCH nor AMBIGUOUS — the catalog decided them — yet they reach QUOTED with an
	// empty valuation, so the seller has to be shown the gap rather than left to infer it.
	UnpricedItemIDs []uuid.UUID
	// Alternatives are the candidates each flagged line was decided from, keyed by line id.
	Alternatives map[uuid.UUID][]QuoteItemAlternative
}
