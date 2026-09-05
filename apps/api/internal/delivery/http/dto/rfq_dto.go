package dto

import (
	"time"

	"github.com/google/uuid"
)

// CreateRfqRequest is the body for POST /v1/rfqs. raw_text and items are both
// optional, but at least one must be present.
type CreateRfqRequest struct {
	RawText     *string                `json:"raw_text"`
	WorkType    *string                `json:"work_type"`
	ClientLabel *string                `json:"client_label"`
	Items       []CreateRfqItemRequest `json:"items" binding:"omitempty,dive"`
}

// CreateRfqItemRequest is one structured line of a manual entry.
type CreateRfqItemRequest struct {
	ProductID            *uuid.UUID `json:"product_id"`
	RequestedDescription string     `json:"requested_description" binding:"required,min=1,max=512"`
	Quantity             string     `json:"quantity" binding:"required,numeric"`
	Unit                 *string    `json:"unit" binding:"omitempty,max=64"`
}

// RfqResponse is one RFQ as returned by the creation endpoint.
type RfqResponse struct {
	ID          uuid.UUID `json:"id"`
	BranchID    uuid.UUID `json:"branch_id"`
	ClientLabel *string   `json:"client_label"`
	ChannelID   uuid.UUID `json:"channel_id"`
	RawText     *string   `json:"raw_text"`
	Status      string    `json:"status"`
	WorkType    *string   `json:"work_type"`
	ReceivedAt  time.Time `json:"received_at"`
	CreatedAt   time.Time `json:"created_at"`
}

// CreateRfqResponse is returned by POST /v1/rfqs.
type CreateRfqResponse struct {
	Rfq   RfqResponse   `json:"rfq"`
	Quote QuoteResponse `json:"quote"`
}

// RfqListItemResponse is one row of the RFQ list the Backoffice dashboard consumes.
type RfqListItemResponse struct {
	ID            uuid.UUID  `json:"id"`
	ClientID      *uuid.UUID `json:"client_id"`
	Client        *string    `json:"client"`
	CreatedAt     time.Time  `json:"created_at"`
	Channel       string     `json:"channel"`
	SellerID      *uuid.UUID `json:"seller_id"`
	Seller        string     `json:"seller"`
	BranchID      uuid.UUID  `json:"branch_id"`
	Branch        string     `json:"branch"`
	ItemCount     int        `json:"item_count"`
	Total         *string    `json:"total"`
	Status        string     `json:"status"`
	ArchivedAt    *time.Time `json:"archived_at"`
	NeedsFollowup bool       `json:"needs_followup"`
}

// CreateTextRFQDraftRequest is the body for POST /v1/rfqs/text-drafts.
type CreateTextRFQDraftRequest struct {
	ChannelID   uuid.UUID  `json:"channel_id" binding:"required"`
	ClientID    *uuid.UUID `json:"client_id"`
	ClientLabel *string    `json:"client_label" binding:"omitempty,max=255"`
	RawText     string     `json:"raw_text" binding:"required,min=1"`
	WorkType    *string    `json:"work_type" binding:"omitempty,max=255"`
}

// CreateWhatsAppMockRFQDraftRequest is the body for POST /v1/dev/whatsapp/messages.
type CreateWhatsAppMockRFQDraftRequest struct {
	ChannelID   *uuid.UUID `json:"channel_id"`
	From        string     `json:"from" binding:"required,min=1,max=64"`
	ProfileName *string    `json:"profile_name" binding:"omitempty,max=160"`
	Text        string     `json:"text" binding:"required,min=1"`
}

// TextRFQDraftResponse is returned by POST /v1/rfqs/text-drafts.
type TextRFQDraftResponse struct {
	RFQ     RFQResponse           `json:"rfq"`
	Quote   *QuoteResponse        `json:"quote"`
	Version *QuoteVersionResponse `json:"version"`
	Items   []QuoteItemResponse   `json:"items"`
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

// RFQAttachmentResponse is one stored file on an RFQ.
type RFQAttachmentResponse struct {
	ID               uuid.UUID `json:"id"`
	RFQID            uuid.UUID `json:"rfq_id"`
	Type             string    `json:"type"`
	ProcessingStatus string    `json:"processing_status"`
	URL              string    `json:"url"`
	ExpiresAt        time.Time `json:"expires_at"`
	CreatedAt        time.Time `json:"created_at"`
}

// RFQAttachmentListResponse is returned by GET /v1/rfqs/{rfqId}/attachments.
type RFQAttachmentListResponse struct {
	Attachments []RFQAttachmentResponse `json:"attachments"`
}

// RfqDetailResponse is returned by GET /v1/rfqs/{rfqId}. It includes the RFQ
// list item projection plus the full quote, version, items, and alternatives.
type RfqDetailResponse struct {
	Rfq          RfqListItemResponse                       `json:"rfq"`
	Quote        *QuoteResponse                            `json:"quote"`
	Version      *QuoteVersionResponse                     `json:"version"`
	Items        []QuoteItemResponse                       `json:"items"`
	Alternatives map[string][]QuoteItemAlternativeResponse `json:"alternatives"`
}

// UpdateQuoteItemRequest is the body for PATCH /v1/quotes/{quoteId}/items/{itemId}. All fields
// are optional: only present fields are written.
type UpdateQuoteItemRequest struct {
	ProductID            *uuid.UUID `json:"product_id"`
	RequestedDescription *string    `json:"requested_description" binding:"omitempty,max=512"`
	Quantity             *string    `json:"quantity" binding:"omitempty,numeric"`
	Unit                 *string    `json:"unit" binding:"omitempty,max=64"`
	UnitPriceSnapshot    *string    `json:"unit_price_snapshot" binding:"omitempty,numeric"`
}

// AddQuoteItemRequest is the body for POST /v1/quotes/{quoteId}/items.
type AddQuoteItemRequest struct {
	ProductID            *uuid.UUID `json:"product_id"`
	RequestedDescription string     `json:"requested_description" binding:"required,min=1,max=512"`
	Quantity             string     `json:"quantity" binding:"required,numeric"`
	Unit                 *string    `json:"unit" binding:"omitempty,max=64"`
}
