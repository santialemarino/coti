package dto

import (
	"time"

	"github.com/google/uuid"
)

// SignupRequest is the body for POST /v1/public/accounts. It registers the corralón, its first
// branch and the administrator who operates them in one call.
type SignupRequest struct {
	AccountName   string  `json:"account_name" binding:"required,min=1,max=255"`
	LegalName     *string `json:"legal_name" binding:"omitempty,max=255"`
	TaxID         *string `json:"tax_id" binding:"omitempty,max=255"`
	BranchName    string  `json:"branch_name" binding:"required,min=1,max=255"`
	BranchAddress *string `json:"branch_address" binding:"omitempty,max=255"`
	AdminName     string  `json:"admin_name" binding:"required,min=1,max=255"`
	AdminEmail    string  `json:"admin_email" binding:"required,email,max=255"`
	AdminPassword string  `json:"admin_password" binding:"required,max=72"`
}

// SignupResponse carries what registration created plus the session it opened, so the caller
// does not have to log in immediately afterwards.
type SignupResponse struct {
	Account AccountResponse `json:"account"`
	Branch  BranchResponse  `json:"branch"`
	Tokens  TokenResponse   `json:"tokens"`
}

// UpdateAccountRequest is the body for PUT /v1/account. It replaces the record, brand included.
type UpdateAccountRequest struct {
	Name         string  `json:"name" binding:"required,min=1,max=255"`
	LegalName    *string `json:"legal_name" binding:"omitempty,max=255"`
	TaxID        *string `json:"tax_id" binding:"omitempty,max=255"`
	BrandLogoURL *string `json:"brand_logo_url" binding:"omitempty,url,max=512"`
	BrandColor   *string `json:"brand_color" binding:"omitempty,max=32"`
}

// AccountResponse is the corralón record the backoffice reads and edits.
type AccountResponse struct {
	ID           uuid.UUID `json:"id"`
	Name         string    `json:"name"`
	LegalName    *string   `json:"legal_name"`
	TaxID        *string   `json:"tax_id"`
	BrandLogoURL *string   `json:"brand_logo_url"`
	BrandColor   *string   `json:"brand_color"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// MeResponse is the authenticated caller's own identity and reach.
type MeResponse struct {
	ID        uuid.UUID   `json:"id"`
	Name      string      `json:"name"`
	Email     string      `json:"email"`
	Role      string      `json:"role"`
	AccountID uuid.UUID   `json:"account_id"`
	BranchIDs []uuid.UUID `json:"branch_ids"`
}
