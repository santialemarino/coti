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

// CreateWhatsAppMockRFQDraftRequest is the body for POST /v1/dev/whatsapp/messages.
type CreateWhatsAppMockRFQDraftRequest struct {
	ChannelID   *uuid.UUID `json:"channel_id"`
	From        string     `json:"from" binding:"required,min=1,max=64"`
	ProfileName *string    `json:"profile_name" binding:"omitempty,max=160"`
	Text        string     `json:"text" binding:"required,min=1"`
}

// TextRFQDraftResponse is returned by POST /v1/rfqs/text-drafts. Quote, version and items are
// null or empty when the order named no material at all: the text is kept and the RFQ stays
// RECEIVED.
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

// RFQAttachmentResponse is one stored file on an RFQ, returned by the attachment routes. The
// link expires — expires_at says when — so it is fetched again rather than stored by a client.
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
