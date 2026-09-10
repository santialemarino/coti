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
	QuoteNumber   *int64     `json:"quote_number"`
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
// list item projection plus the full quote, version, items, alternatives, discounts,
// and the change-request diff when one applies.
type RfqDetailResponse struct {
	Rfq              RfqListItemResponse                       `json:"rfq"`
	Quote            *QuoteResponse                            `json:"quote"`
	Version          *QuoteVersionResponse                     `json:"version"`
	Items            []QuoteItemResponse                       `json:"items"`
	Alternatives     map[string][]QuoteItemAlternativeResponse `json:"alternatives"`
	RFQHistory       []RFQStatusChangeResponse                 `json:"rfq_status_history"`
	QuoteHistory     []QuoteStatusChangeResponse               `json:"quote_status_history"`
	Deliveries       []QuoteSendTrackingResponse               `json:"deliveries"`
	Discounts        []QuoteDiscountResponse                   `json:"discounts"`
	ChangesRequested *ChangeRequestDiffResponse                `json:"changes_requested,omitempty"`
}

// RFQStatusChangeResponse is one recorded transition on the RFQ status cache.
type RFQStatusChangeResponse struct {
	ID             uuid.UUID  `json:"id"`
	RFQID          uuid.UUID  `json:"rfq_id"`
	PreviousStatus *string    `json:"previous_status"`
	NewStatus      string     `json:"new_status"`
	UserID         *uuid.UUID `json:"user_id"`
	ChangedAt      time.Time  `json:"changed_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

// QuoteStatusChangeResponse is one recorded transition on quote.current_status.
type QuoteStatusChangeResponse struct {
	ID             uuid.UUID  `json:"id"`
	QuoteID        uuid.UUID  `json:"quote_id"`
	PreviousStatus *string    `json:"previous_status"`
	NewStatus      string     `json:"new_status"`
	UserID         *uuid.UUID `json:"user_id"`
	ChangedAt      time.Time  `json:"changed_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

// QuoteSendTrackingResponse is one delivery attempt shown in the RFQ detail tracking panel.
type QuoteSendTrackingResponse struct {
	ID             uuid.UUID  `json:"id"`
	VersionID      uuid.UUID  `json:"version_id"`
	Channel        string     `json:"channel"`
	Destination    string     `json:"destination"`
	Format         string     `json:"format"`
	TrackingStatus string     `json:"tracking_status"`
	SentAt         *time.Time `json:"sent_at"`
	ExpiresAt      *time.Time `json:"expires_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

// ChangeRequestDiffResponse is the frozen-vs-draft comparison the detail screen
// renders for a CHANGE_REQUESTED quote with a frozen predecessor.
type ChangeRequestDiffResponse struct {
	Reason    *string                   `json:"reason"`
	Original  ChangeRequestSideResponse `json:"original"`
	Requested ChangeRequestSideResponse `json:"requested"`
}

// ChangeRequestSideResponse is one column (original or requested) of the diff.
type ChangeRequestSideResponse struct {
	Items     []DiffLineItemResponse     `json:"items"`
	Discounts []DiffDiscountLineResponse `json:"discounts"`
	Total     string                     `json:"total"` // decimal string, never float.
}

// DiffLineItemResponse is one aligned line of the diff. Quantity and UnitPrice are
// decimal strings; ChangeType is set only on the flagged (requested) side.
type DiffLineItemResponse struct {
	Description string  `json:"description"`
	Quantity    string  `json:"quantity"`
	Unit        *string `json:"unit"`
	UnitPrice   *string `json:"unit_price"`
	Changed     bool    `json:"changed"`
	ChangeType  *string `json:"change_type,omitempty"`
}

// DiffDiscountLineResponse is one aligned discount row of the diff.
type DiffDiscountLineResponse struct {
	Name    string `json:"name"`
	Amount  string `json:"amount"`
	Changed bool   `json:"changed"`
}

// QuoteDiscountResponse is one discount application on the quote's current version.
// ActionType and ActionValue surface the raw seller-typed rule: a MANUAL_SELLER percentage
// row reports its rate, so the UI can render how the amount was produced. ItemIDs are the
// covered lines for an ITEM/ITEM_SET discount, filled on the detail response only.
type QuoteDiscountResponse struct {
	ID                 uuid.UUID   `json:"id"`
	QuoteVersionID     uuid.UUID   `json:"quote_version_id"`
	PromotionID        *uuid.UUID  `json:"promotion_id"`
	PromotionName      *string     `json:"promotion_name"`
	ConditionType      string      `json:"condition_type"`
	Scope              string      `json:"scope"`
	Origin             string      `json:"origin"`
	Amount             string      `json:"amount"`
	ActionType         string      `json:"action_type"`
	ActionValue        *string     `json:"action_value"`
	ItemIDs            []uuid.UUID `json:"item_ids"`
	Description        *string     `json:"description"`
	SuppressedBySeller bool        `json:"suppressed_by_seller"`
	CreatedAt          time.Time   `json:"created_at"`
}

// CreateDiscountRequest is the body for POST /v1/quotes/{quoteId}/discounts. Only
// seller-typed discounts are accepted here; the engine's own rows are computed and written
// by the sweep, not by a seller. Value is a fixed amount or a percentage depending on
// action_type; item_ids scope the discount to lines (empty for a TOTAL scope).
type CreateDiscountRequest struct {
	Description string   `json:"description" binding:"required,min=1,max=512"`
	ActionType  string   `json:"action_type" binding:"required,oneof=FIXED_AMOUNT PERCENTAGE"`
	Value       string   `json:"value" binding:"required,numeric"`
	Scope       string   `json:"scope" binding:"required,oneof=TOTAL ITEM ITEM_SET"`
	ItemIDs     []string `json:"item_ids"`
}

// UpdateDiscountRequest is the body for PATCH /v1/quotes/{quoteId}/discounts/{discountId}.
// All fields are optional: only present fields are written.
type UpdateDiscountRequest struct {
	Value              *string  `json:"value" binding:"omitempty,numeric"`
	ActionType         *string  `json:"action_type" binding:"omitempty,oneof=FIXED_AMOUNT PERCENTAGE"`
	Scope              *string  `json:"scope" binding:"omitempty,oneof=TOTAL ITEM ITEM_SET"`
	ItemIDs            []string `json:"item_ids"`
	Description        *string  `json:"description" binding:"omitempty,min=1,max=512"`
	SuppressedBySeller *bool    `json:"suppressed_by_seller"`
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
