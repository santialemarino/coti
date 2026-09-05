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
	// Alternatives are the candidates a flagged line was decided from, best first. A decided line
	// carries none: there is nothing for the seller to choose between.
	Alternatives []QuoteItemAlternativeResponse `json:"alternatives"`
	// PricingUnavailable names a line that matched a product the branch cannot price. It is null
	// until the materials are accepted, because until then no line has been priced at all.
	PricingUnavailable *bool     `json:"pricing_unavailable"`
	QuantityRationale  *string   `json:"quantity_rationale"`
	CreatedAt          time.Time `json:"created_at"`
}

// QuoteItemAlternativeResponse is one candidate offered for a flagged line. rank is its place in
// the matcher's ranking, so a line that kept the leading candidate offers none at rank one. The
// catalog identity is read as it stands now rather than frozen with the line.
type QuoteItemAlternativeResponse struct {
	ID               uuid.UUID  `json:"id"`
	ProductID        *uuid.UUID `json:"product_id"`
	ComboID          *uuid.UUID `json:"combo_id"`
	Type             string     `json:"type"`
	Origin           string     `json:"origin"`
	Rank             int        `json:"rank"`
	ConfidenceScore  *string    `json:"confidence_score"`
	PriceSnapshot    *string    `json:"price_snapshot"`
	ApprovedBySeller bool       `json:"approved_by_seller"`
	ChosenByClient   bool       `json:"chosen_by_client"`
	Code             *string    `json:"code"`
	CanonicalName    *string    `json:"canonical_name"`
	Unit             *string    `json:"unit"`
}

// QuoteSendRequest selects the optional email copy and validity override. WhatsApp and the
// public webapp link are always included by the backend.
type QuoteSendRequest struct {
	RecipientPhone string                     `json:"recipient_phone" binding:"required"`
	EmailDelivery  *QuoteEmailDeliveryRequest `json:"email_delivery"`
	ExpiryDays     *int                       `json:"expiry_days"`
}

// QuoteEmailDeliveryRequest is the optional independent email destination.
type QuoteEmailDeliveryRequest struct {
	Address string `json:"address" binding:"required"`
}

// QuoteSendResponse reports each independent channel outcome after the database commit.
type QuoteSendResponse struct {
	QuoteID       uuid.UUID               `json:"quote_id"`
	VersionID     uuid.UUID               `json:"version_id"`
	CurrentStatus string                  `json:"current_status"`
	ExpiresAt     *time.Time              `json:"expires_at"`
	Deliveries    []QuoteDeliveryResponse `json:"deliveries"`
}

// QuoteDeliveryResponse reports one channel-specific send.
type QuoteDeliveryResponse struct {
	ID             uuid.UUID  `json:"id"`
	Channel        string     `json:"channel"`
	Destination    string     `json:"destination"`
	TrackingStatus string     `json:"tracking_status"`
	PublicURL      string     `json:"public_url"`
	SentAt         *time.Time `json:"sent_at"`
}

// PublicQuoteSendResponse is the minimal sessionless token state for the future webapp.
type PublicQuoteSendResponse struct {
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
}
