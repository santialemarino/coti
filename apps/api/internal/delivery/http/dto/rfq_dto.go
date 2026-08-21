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

// CreateRfqItemRequest is one structured line of a manual entry. Quantity is a decimal
// STRING, never a float, so NUMERIC(14,2) precision survives the round trip.
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

// QuoteResponse is a quote as returned by the creation endpoint.
type QuoteResponse struct {
	ID               uuid.UUID  `json:"id"`
	BranchID         uuid.UUID  `json:"branch_id"`
	RfqID            uuid.UUID  `json:"rfq_id"`
	SellerID         *uuid.UUID `json:"seller_id"`
	CurrentVersionID *uuid.UUID `json:"current_version_id"`
	CurrentStatus    string     `json:"current_status"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// CreateRfqResponse is returned by POST /v1/rfqs: the RFQ and the DRAFT quote born
// with it, already pointing at version v1.
type CreateRfqResponse struct {
	Rfq   RfqResponse   `json:"rfq"`
	Quote QuoteResponse `json:"quote"`
}

// RfqListItemResponse is one row of the RFQ list the Backoffice dashboard consumes.
type RfqListItemResponse struct {
	ID          uuid.UUID  `json:"id"`
	Client      *string    `json:"client"`
	CreatedAt   time.Time  `json:"created_at"`
	Channel     string     `json:"channel"`
	Seller      string     `json:"seller"`
	Branch      string     `json:"branch"`
	ItemCount   int        `json:"item_count"`
	Total       *string    `json:"total"`
	Status      string     `json:"status"`
	ArchivedAt  *time.Time `json:"archived_at"`
}
