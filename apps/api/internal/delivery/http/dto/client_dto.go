package dto

import (
	"time"

	"github.com/google/uuid"
)

// ClientResponse represents one account-scoped client profile.
type ClientResponse struct {
	ID            uuid.UUID `json:"id"`
	Name          *string   `json:"name"`
	Phone         *string   `json:"phone"`
	Email         *string   `json:"email"`
	OriginChannel *string   `json:"origin_channel"`
	Notes         *string   `json:"notes"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ClientTagResponse represents one reusable account tag.
type ClientTagResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

// ClientSummaryResponse represents one client row and accepted-sale activity.
type ClientSummaryResponse struct {
	ClientResponse
	Tags               []ClientTagResponse `json:"tags"`
	AcceptedQuoteCount int                 `json:"accepted_quote_count"`
	LastAcceptedAt     *time.Time          `json:"last_accepted_at"`
}

// ClientListResponse is returned by GET /v1/clients.
type ClientListResponse struct {
	Items []ClientSummaryResponse `json:"items"`
}

// ClientQuoteResponse represents one quote in a client's history, at any status.
type ClientQuoteResponse struct {
	QuoteID    uuid.UUID `json:"quote_id"`
	RFQID      uuid.UUID `json:"rfq_id"`
	Number     int64     `json:"quote_number"`
	BranchID   uuid.UUID `json:"branch_id"`
	BranchName string    `json:"branch_name"`
	Status     string    `json:"status"`
	Archived   bool      `json:"archived"`
	// Total is a decimal string, or null while no version carries a price.
	Total *string `json:"total"`
	// SellerName is null on a quote nobody has claimed.
	SellerName *string   `json:"seller_name"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// ClientActivitySummaryResponse is the derived roll-up of a client's visible quote history.
type ClientActivitySummaryResponse struct {
	TotalQuotes       int        `json:"total_quotes"`
	AcceptedQuotes    int        `json:"accepted_quotes"`
	RejectedQuotes    int        `json:"rejected_quotes"`
	PendingQuotes     int        `json:"pending_quotes"`
	LastInteractionAt *time.Time `json:"last_interaction_at"`
}

// ClientProfileResponse is returned by GET /v1/clients/:clientId.
type ClientProfileResponse struct {
	Client  ClientResponse                `json:"client"`
	Tags    []ClientTagResponse           `json:"tags"`
	Summary ClientActivitySummaryResponse `json:"summary"`
	Quotes  []ClientQuoteResponse         `json:"quotes"`
}

// ClientTagListResponse is returned by tag list and replacement endpoints.
type ClientTagListResponse struct {
	Items []ClientTagResponse `json:"items"`
}

// CreateClientTagRequest is the body for POST /v1/tags.
type CreateClientTagRequest struct {
	Name string `json:"name" binding:"required,max=128"`
}

// ReplaceClientTagsRequest is the body for PUT /v1/clients/:clientId/tags.
type ReplaceClientTagsRequest struct {
	TagIDs []uuid.UUID `json:"tag_ids"`
}

// CreateClientRequest is the new profile option in an accepted-sale association.
type CreateClientRequest struct {
	Name  *string `json:"name" binding:"omitempty,max=255"`
	Phone *string `json:"phone" binding:"omitempty,max=64"`
	Email *string `json:"email" binding:"omitempty,max=255"`
}

// AssociateQuoteClientRequest is the body for PUT /v1/quotes/:quoteId/client-association.
type AssociateQuoteClientRequest struct {
	ClientID  *uuid.UUID           `json:"client_id"`
	NewClient *CreateClientRequest `json:"new_client"`
	TagIDs    []uuid.UUID          `json:"tag_ids"`
}

// ClientMatchResponse is one seller-reviewable contact match.
type ClientMatchResponse struct {
	Client ClientResponse      `json:"client"`
	Tags   []ClientTagResponse `json:"tags"`
}

// ClientContactHintsResponse carries the last quote destinations as creation defaults.
type ClientContactHintsResponse struct {
	Phone *string `json:"phone"`
	Email *string `json:"email"`
}

// QuoteClientAssociationResponse is the context for confirming an accepted sale's client.
type QuoteClientAssociationResponse struct {
	CurrentClient *ClientMatchResponse       `json:"current_client"`
	Suggestions   []ClientMatchResponse      `json:"suggestions"`
	ContactHints  ClientContactHintsResponse `json:"contact_hints"`
	AvailableTags []ClientTagResponse        `json:"available_tags"`
}
