package dto

import (
	"github.com/google/uuid"
	"time"
)

// ClientContactResponse is one commercial destination in a customer profile.
type ClientContactResponse struct {
	Kind    string `json:"kind" binding:"required,oneof=WHATSAPP EMAIL"`
	Value   string `json:"value" binding:"required,max=255"`
	Primary bool   `json:"primary"`
}

// CustomerProfileResponse is returned by GET /v1/customer/sites/{hostname}/profile.
type CustomerProfileResponse struct {
	ReviewPending bool                    `json:"review_pending"`
	ClientID      *uuid.UUID              `json:"client_id"`
	TaxID         string                  `json:"tax_id"`
	Name          string                  `json:"name"`
	Contacts      []ClientContactResponse `json:"contacts"`
}

// UpdateCustomerProfileRequest replaces the authenticated customer's commercial data.
type UpdateCustomerProfileRequest struct {
	Name     string                  `json:"name" binding:"max=255"`
	Contacts []ClientContactResponse `json:"contacts" binding:"required,dive"`
}

// ContactSuggestionResponse is one branch-visible inbound association case.
type ContactSuggestionResponse struct {
	ID                uuid.UUID  `json:"id"`
	SourceClientID    *uuid.UUID `json:"source_client_id"`
	CandidateClientID *uuid.UUID `json:"candidate_client_id"`
	CandidateName     *string    `json:"candidate_name"`
	TaxID             *string    `json:"tax_id"`
	Kind              string     `json:"kind"`
	Value             string     `json:"value"`
	DeclaredName      *string    `json:"declared_name"`
	OriginalText      string     `json:"original_text"`
	CreatedAt         time.Time  `json:"created_at"`
}

// ResolveContactSuggestionRequest records a seller's explicit commercial association decision.
type ResolveContactSuggestionRequest struct {
	Action string `json:"action" binding:"required,oneof=ASSOCIATE SEPARATE MERGE"`
}
