package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateTextRFQDraftRequest is the body for POST /v1/rfqs/text-drafts.
type CreateTextRFQDraftRequest struct {
	ChannelID   uuid.UUID  `json:"channel_id" binding:"required"`
	ClientID    *uuid.UUID `json:"client_id"`
	ClientLabel *string    `json:"client_label" binding:"omitempty,max=255"`
	RawText     string     `json:"raw_text" binding:"required,min=1"`
	WorkType    *string    `json:"work_type" binding:"omitempty,max=255"`
}

// TextRFQDraftResponse is returned by POST /v1/rfqs/text-drafts.
type TextRFQDraftResponse struct {
	RFQ     RFQResponse          `json:"rfq"`
	Quote   QuoteResponse        `json:"quote"`
	Version QuoteVersionResponse `json:"version"`
	Items   []QuoteItemResponse  `json:"items"`
}

// RFQResponse represents the source request stored from client input.
type RFQResponse struct {
	ID          uuid.UUID  `json:"id"`
	BranchID    uuid.UUID  `json:"branch_id"`
	ClientID    *uuid.UUID `json:"client_id"`
	ChannelID   uuid.UUID  `json:"channel_id"`
	RawText     *string    `json:"raw_text"`
	Status      string     `json:"status"`
	WorkType    *string    `json:"work_type"`
	ClientLabel *string    `json:"client_label"`
	ReceivedAt  time.Time  `json:"received_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// QuoteResponse represents the quote shell created from one RFQ.
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
