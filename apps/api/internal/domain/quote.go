package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// QuoteStatus is the lifecycle state a quote carries once it exists. DRAFT is the
// state while the RFQ is GENERATED: materials matched, no accepted prices yet.
type QuoteStatus string

const (
	QuoteStatusDraft           QuoteStatus = "DRAFT"
	QuoteStatusQuoted          QuoteStatus = "QUOTED"
	QuoteStatusSent            QuoteStatus = "SENT"
	QuoteStatusChangeRequested QuoteStatus = "CHANGE_REQUESTED"
	QuoteStatusAccepted        QuoteStatus = "ACCEPTED"
	QuoteStatusRejected        QuoteStatus = "REJECTED"
)

// ItemMatchStatus is the catalog-match outcome for a quote line. NO_MATCH lines are
// flagged (product_id NULL), never discarded.
type ItemMatchStatus string

const (
	ItemMatchStatusMatched   ItemMatchStatus = "MATCHED"
	ItemMatchStatusAmbiguous ItemMatchStatus = "AMBIGUOUS"
	ItemMatchStatusNoMatch   ItemMatchStatus = "NO_MATCH"
)

// Quote is the review-ready answer to an RFQ. One RFQ has exactly one quote; the
// quote is born when the RFQ reaches GENERATED. current_status is a backend-exclusive
// derived cache, recomputed on each transition, never set by a human or the AI.
type Quote struct {
	ID                uuid.UUID
	AccountID         uuid.UUID
	BranchID          uuid.UUID
	ClientID          *uuid.UUID
	RfqID             uuid.UUID
	SellerID          *uuid.UUID // null until someone claims the RFQ from the inbox.
	CurrentVersionID  *uuid.UUID
	CurrentStatus     QuoteStatus
	ExpiresAt         *time.Time
	ArchivedAt        *time.Time
	NeedsFollowup     bool
	FollowupFlaggedAt *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// QuoteVersion is an immutable snapshot of a quote. total = Σ item subtotals −
// Σ discounts. Append-only: it has no updated_at.
type QuoteVersion struct {
	ID            uuid.UUID
	QuoteID       uuid.UUID
	AuthorID      *uuid.UUID
	VersionNumber int
	Total         decimal.Decimal // NUMERIC(14,2).
	IsImmutable   bool            // draft = false; frozen = true.
	Comment       *string
	CreatedAt     time.Time
}

// QuoteItem is one line of a quote version. The item does not carry its discount; a
// discount is its own entity. price snapshots are NULL until the pricing step runs.
type QuoteItem struct {
	ID                   uuid.UUID
	VersionID            uuid.UUID
	ProductID            *uuid.UUID // NULL on a NO_MATCH line.
	RequestedDescription string
	Quantity             decimal.Decimal // NUMERIC(14,2).
	Unit                 *string
	UnitPriceSnapshot    decimal.NullDecimal // NUMERIC(14,2).
	MinPriceSnapshot     decimal.NullDecimal // discount-engine floor, snapshotted.
	Subtotal             decimal.NullDecimal // NUMERIC(14,2).
	ConfidenceScore      *float64            // NUMERIC(5,4), 0..1.
	MatchStatus          ItemMatchStatus
	QuantityRationale    *string
	CreatedAt            time.Time
}
