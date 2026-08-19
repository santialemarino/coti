package dto

import (
	"time"

	"github.com/google/uuid"
)

// PricedQuoteResponse is returned by POST /v1/quotes/{quoteId}/accept-materials. Money and
// quantities are decimal strings; a line with nothing to price keeps all three of them null.
type PricedQuoteResponse struct {
	Quote   QuoteResponse        `json:"quote"`
	Version QuoteVersionResponse `json:"version"`
	Items   []QuoteItemResponse  `json:"items"`
}

// QuoteResponse represents the quote created from one RFQ.
type QuoteResponse struct {
	ID                uuid.UUID  `json:"id"`
	BranchID          uuid.UUID  `json:"branch_id"`
	ClientID          *uuid.UUID `json:"client_id"`
	RFQID             uuid.UUID  `json:"rfq_id"`
	SellerID          *uuid.UUID `json:"seller_id"`
	CurrentVersionID  *uuid.UUID `json:"current_version_id"`
	CurrentStatus     string     `json:"current_status"`
	ExpiresAt         *time.Time `json:"expires_at"`
	ArchivedAt        *time.Time `json:"archived_at"`
	NeedsFollowup     bool       `json:"needs_followup"`
	FollowupFlaggedAt *time.Time `json:"followup_flagged_at"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// QuoteVersionResponse represents a reviewable quote version.
type QuoteVersionResponse struct {
	ID            uuid.UUID  `json:"id"`
	QuoteID       uuid.UUID  `json:"quote_id"`
	AuthorID      *uuid.UUID `json:"author_id"`
	VersionNumber int        `json:"version_number"`
	Total         string     `json:"total"`
	IsImmutable   bool       `json:"is_immutable"`
	Comment       *string    `json:"comment"`
	CreatedAt     time.Time  `json:"created_at"`
}

// QuoteItemResponse represents one material line in a quote version.
type QuoteItemResponse struct {
	ID                   uuid.UUID  `json:"id"`
	VersionID            uuid.UUID  `json:"version_id"`
	ProductID            *uuid.UUID `json:"product_id"`
	RequestedDescription string     `json:"requested_description"`
	Quantity             string     `json:"quantity"`
	Unit                 *string    `json:"unit"`
	UnitPriceSnapshot    *string    `json:"unit_price_snapshot"`
	MinPriceSnapshot     *string    `json:"min_price_snapshot"`
	Subtotal             *string    `json:"subtotal"`
	ConfidenceScore      *string    `json:"confidence_score"`
	MatchStatus          string     `json:"match_status"`
	QuantityRationale    *string    `json:"quantity_rationale"`
	CreatedAt            time.Time  `json:"created_at"`
}
