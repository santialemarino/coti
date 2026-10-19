package dto

import "time"

// CustomerDeliverySiteResponse exposes public supplier routing for a legacy quotation link.
type CustomerDeliverySiteResponse struct {
	Hostname string `json:"hostname"`
}

// CustomerCredentialsRequest carries customer credentials within a supplier hostname.
type CustomerCredentialsRequest struct {
	RecoveryEmail string `json:"recovery_email" binding:"omitempty,email,max=255"`
	TaxID         string `json:"tax_id" binding:"required,max=20"`
	Password      string `json:"password" binding:"required,max=72"`
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
}

// CustomerGoogleCompleteRequest supplies CUIT/CUIL after a verified Google exchange.
type CustomerGoogleCompleteRequest struct {
	Token string `json:"token" binding:"required,len=43"`
	TaxID string `json:"tax_id" binding:"required,max=20"`
}

// CustomerRecoveryRequest names both the account and its previously verified recovery address.
type CustomerRecoveryRequest struct {
	TaxID string `json:"tax_id" binding:"required,max=20"`
	Email string `json:"email" binding:"required,email,max=255"`
}

// CustomerTokenRequest redeems a single-use email verification capability.
type CustomerTokenRequest struct {
	Token string `json:"token" binding:"required,len=43"`
}

// CustomerPasswordResetRequest replaces a password using a single-use recovery capability.
type CustomerPasswordResetRequest struct {
	Token    string `json:"token" binding:"required,len=43"`
	Password string `json:"password" binding:"required,max=72"`
}
