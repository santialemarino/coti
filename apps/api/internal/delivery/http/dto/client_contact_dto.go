package dto

import (
	"time"

	"github.com/google/uuid"
)

// ContactVerificationRequest names a destination already saved in the customer's own profile.
type ContactVerificationRequest struct {
	Kind  string `json:"kind" binding:"required,oneof=EMAIL WHATSAPP"`
	Value string `json:"value" binding:"required,max=255"`
}

// CustomerMainContactRequest is the body for PUT /v1/customer/sites/{hostname}/profile/main-contact.
type CustomerMainContactRequest struct {
	Kind  string `json:"kind" binding:"required,oneof=EMAIL WHATSAPP"`
	Value string `json:"value" binding:"required,max=255"`
}

// ContactRegistrationLinkRequest selects the customer's primary contact channel.
type ContactRegistrationLinkRequest struct {
	Kind string `json:"kind" binding:"required,oneof=EMAIL WHATSAPP"`
}

// ContactVerificationResponse exposes the challenge identifier without its secret.
type ContactVerificationResponse struct {
	ID        uuid.UUID `json:"id"`
	ExpiresAt string    `json:"expires_at"`
}

// ContactVerificationConfirmation redeems a one-use email token or WhatsApp code.
type ContactVerificationConfirmation struct {
	ID    uuid.UUID `json:"id" binding:"required"`
	Token string    `json:"token" binding:"required,max=128"`
}

// ClientContactInput declares a destination without granting verification authority.
type ClientContactInput struct {
	Kind    string `json:"kind" binding:"required,oneof=WHATSAPP EMAIL"`
	Value   string `json:"value" binding:"required,max=255"`
	Primary bool   `json:"primary"`
}

// ClientContactResponse is one commercial destination in a customer profile.
type ClientContactResponse struct {
	Kind     string `json:"kind" binding:"required,oneof=WHATSAPP EMAIL"`
	Value    string `json:"value" binding:"required,max=255"`
	Primary  bool   `json:"primary"`
	Verified bool   `json:"verified"`
	// Main marks the contact account recovery preselects; it is always a verified one.
	Main bool `json:"main"`
}

// CustomerProfileResponse is returned by GET /v1/customer/sites/{hostname}/profile.
type CustomerProfileResponse struct {
	ReviewPending bool `json:"review_pending"`
	// OwnershipUnproved means an earlier profile with this CUIT stays hidden until the customer
	// verifies a contact it already had; only what the customer entered is returned.
	OwnershipUnproved bool                    `json:"ownership_unproved"`
	ClientID          *uuid.UUID              `json:"client_id"`
	TaxID             string                  `json:"tax_id"`
	Name              string                  `json:"name"`
	Contacts          []ClientContactResponse `json:"contacts"`
	LegalName         *string                 `json:"legal_name"`
	IVACondition      *string                 `json:"iva_condition"`
	Address           *string                 `json:"fiscal_address"`
}

// UpdateCustomerProfileRequest replaces the authenticated customer's commercial data.
type UpdateCustomerProfileRequest struct {
	Name         string               `json:"name" binding:"required,max=255"`
	Contacts     []ClientContactInput `json:"contacts" binding:"required,dive"`
	LegalName    *string              `json:"legal_name" binding:"omitempty,max=255"`
	IVACondition *string              `json:"iva_condition" binding:"omitempty,oneof=REGISTERED MONOTRIBUTO EXEMPT FINAL_CONSUMER"`
	Address      *string              `json:"fiscal_address" binding:"omitempty,max=255"`
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
