package dto

import (
	"time"

	"github.com/google/uuid"
)

// CustomerRegistrationRequest requires a fiscal identity, full name and commercial contact.
type CustomerRegistrationRequest struct {
	CustomerCredentialsRequest
	Name     string               `json:"name" binding:"required,max=255"`
	Contacts []ClientContactInput `json:"contacts" binding:"required,min=1,dive"`
}

// CustomerDeliverySiteResponse exposes public supplier routing for a legacy quotation link.
type CustomerDeliverySiteResponse struct {
	Hostname string `json:"hostname"`
}

// CustomerCredentialsRequest carries customer credentials within a supplier hostname.
type CustomerCredentialsRequest struct {
	TaxID    string `json:"tax_id" binding:"required,max=20"`
	Password string `json:"password" binding:"required,max=72"`
}

// CustomerSessionResponse returns an opaque session only to the trusted webapp server.
type CustomerSessionResponse struct {
	SessionToken string                   `json:"session_token"`
	Customer     CustomerIdentityResponse `json:"customer"`
}

// CustomerIdentityResponse exposes the customer's session without staff permissions.
type CustomerIdentityResponse struct {
	ID               string    `json:"id"`
	TaxID            string    `json:"tax_id"`
	Name             *string   `json:"name"`
	SessionExpiresAt time.Time `json:"session_expires_at"`
}

// CustomerGoogleOptionsResponse configures the trusted webapp's authorization-code redirect.
type CustomerGoogleOptionsResponse struct {
	SupplierName      string  `json:"supplier_name"`
	LogoURL           *string `json:"logo_url,omitempty"`
	PasswordMinLength int     `json:"password_min_length"`
	ClientID          string  `json:"client_id"`
	RedirectURI       string  `json:"redirect_uri"`
}

// CustomerGoogleExchangeRequest exchanges a PKCE-bound authorization code.
type CustomerGoogleExchangeRequest struct {
	Code     string `json:"code" binding:"required,max=4096"`
	Verifier string `json:"verifier" binding:"required,min=43,max=128"`
	Nonce    string `json:"nonce" binding:"required,len=43"`
}

// CustomerGoogleExchangeResponse either authenticates a linked account or requests its CUIT/CUIL.
type CustomerGoogleExchangeResponse struct {
	Session     *CustomerSessionResponse `json:"session,omitempty"`
	SignupToken string                   `json:"signup_token,omitempty"`
	// SignupEmail is the Google address the customer may choose as a contact, sent only when Google verified it.
	SignupEmail string `json:"signup_email,omitempty"`
	// SignupEmailProven is true when choosing SignupEmail verifies it; otherwise it needs a code.
	SignupEmailProven bool `json:"signup_email_proven,omitempty"`
}

// CustomerGoogleCompleteRequest supplies CUIT/CUIL and the contacts after a verified Google exchange.
type CustomerGoogleCompleteRequest struct {
	Name     string               `json:"name" binding:"max=255"`
	Token    string               `json:"token" binding:"required,len=43"`
	TaxID    string               `json:"tax_id" binding:"required,max=20"`
	Contacts []ClientContactInput `json:"contacts" binding:"omitempty,dive"`
	// UseGoogleEmail adds the Google address as a verified contact; a new registration needs it or a contact.
	UseGoogleEmail bool `json:"use_google_email"`
}

// CustomerRecoveryOptionsRequest is the body for POST /v1/customer/sites/{hostname}/auth/recovery/options.
type CustomerRecoveryOptionsRequest struct {
	TaxID string `json:"tax_id" binding:"required,max=20"`
}

// CustomerRecoveryContactResponse is one verified contact a recovery code can be sent to, masked.
type CustomerRecoveryContactResponse struct {
	ID     uuid.UUID `json:"id"`
	Kind   string    `json:"kind"`
	Masked string    `json:"masked"`
	Main   bool      `json:"main"`
	// Available is false while no code can reach the contact's channel.
	Available   bool `json:"available"`
	Preselected bool `json:"preselected"`
}

// CustomerRecoveryOptionsResponse lists the masked contacts; it is empty for an unknown CUIT/CUIL.
type CustomerRecoveryOptionsResponse struct {
	Contacts []CustomerRecoveryContactResponse `json:"contacts"`
}

// CustomerRecoveryCodeRequest is the body for POST /v1/customer/sites/{hostname}/auth/recovery/code.
type CustomerRecoveryCodeRequest struct {
	TaxID     string    `json:"tax_id" binding:"required,max=20"`
	ContactID uuid.UUID `json:"contact_id" binding:"required"`
}

// CustomerRecoveryCodeResponse names the code that was sent, without the code.
type CustomerRecoveryCodeResponse struct {
	ID        uuid.UUID `json:"id"`
	ExpiresAt time.Time `json:"expires_at"`
}

// CustomerRecoveryResetRequest is the body for POST /v1/customer/sites/{hostname}/auth/recovery/reset.
type CustomerRecoveryResetRequest struct {
	ID       uuid.UUID `json:"id" binding:"required"`
	Code     string    `json:"code" binding:"required,max=16"`
	Password string    `json:"password" binding:"required,max=72"`
}
