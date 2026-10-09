package dto

import (
	"time"

	"github.com/google/uuid"
)

// UpdateInvoicingSettingsRequest is the body for PUT /v1/invoicing/settings. Branches not listed
// keep their point of sale; a null point_of_sale clears it.
type UpdateInvoicingSettingsRequest struct {
	IVACondition     *string                  `json:"iva_condition" binding:"omitempty,oneof=REGISTERED MONOTRIBUTO EXEMPT FINAL_CONSUMER"`
	PricesIncludeVAT bool                     `json:"prices_include_vat"`
	Branches         []BranchPointOfSaleInput `json:"branches" binding:"dive"`
}

// BranchPointOfSaleInput sets one branch's ARCA point of sale.
type BranchPointOfSaleInput struct {
	BranchID    uuid.UUID `json:"branch_id" binding:"required"`
	PointOfSale *int      `json:"point_of_sale" binding:"omitempty,min=1,max=99998"`
}

// InvoicingSettingsResponse is returned by GET and PUT /v1/invoicing/settings.
type InvoicingSettingsResponse struct {
	Enabled          bool                        `json:"enabled"`
	Environment      string                      `json:"environment"`
	LegalName        *string                     `json:"legal_name"`
	TaxID            *string                     `json:"tax_id"`
	IVACondition     *string                     `json:"iva_condition"`
	PricesIncludeVAT bool                        `json:"prices_include_vat"`
	Branches         []BranchPointOfSaleResponse `json:"branches"`
	Credential       *ARCACredentialResponse     `json:"credential"`
}

// BranchPointOfSaleResponse is one branch and its point of sale.
type BranchPointOfSaleResponse struct {
	BranchID    uuid.UUID `json:"branch_id"`
	Name        string    `json:"name"`
	IsActive    bool      `json:"is_active"`
	PointOfSale *int      `json:"point_of_sale"`
}

// ARCACredentialResponse describes the uploaded certificate; the key never leaves the server.
type ARCACredentialResponse struct {
	CUIT      string    `json:"cuit"`
	Subject   string    `json:"subject"`
	ExpiresAt time.Time `json:"expires_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// IssueInvoiceRequest is the body for POST /v1/quotes/:quoteId/invoice: the previewed invoice the
// seller confirmed. Total is a decimal string.
type IssueInvoiceRequest struct {
	VersionID uuid.UUID `json:"version_id" binding:"required"`
	Type      string    `json:"type" binding:"required,oneof=A B C"`
	Total     string    `json:"total" binding:"required,numeric"`
}

// UpdateClientFiscalRequest is the body for PUT /v1/clients/:clientId/fiscal. Omitted fields clear.
type UpdateClientFiscalRequest struct {
	LegalName    *string `json:"legal_name" binding:"omitempty,max=255"`
	TaxID        *string `json:"tax_id" binding:"omitempty,max=32"`
	IVACondition *string `json:"iva_condition" binding:"omitempty,oneof=REGISTERED MONOTRIBUTO EXEMPT FINAL_CONSUMER"`
}

// ClientFiscalResponse is a client's fiscal identity.
type ClientFiscalResponse struct {
	ID           uuid.UUID `json:"id"`
	Name         *string   `json:"name"`
	LegalName    *string   `json:"legal_name"`
	TaxID        *string   `json:"tax_id"`
	IVACondition *string   `json:"iva_condition"`
}

// VATAmountResponse is one rate of an invoice. Money is a decimal string.
type VATAmountResponse struct {
	Rate   string `json:"rate"`
	Base   string `json:"base"`
	Amount string `json:"amount"`
}

// InvoiceReceiverResponse is the buyer as the invoice names them.
type InvoiceReceiverResponse struct {
	Name         string `json:"name"`
	DocType      string `json:"doc_type"`
	DocNumber    string `json:"doc_number"`
	IVACondition string `json:"iva_condition"`
}

// InvoiceAmountsResponse is an invoice's money, as decimal strings.
type InvoiceAmountsResponse struct {
	Net    string              `json:"net"`
	Exempt string              `json:"exempt"`
	VAT    string              `json:"vat"`
	Total  string              `json:"total"`
	ByRate []VATAmountResponse `json:"by_rate"`
}

// InvoiceResponse is an invoice issued, pending or refused for a quote.
type InvoiceResponse struct {
	ID           uuid.UUID               `json:"id"`
	Status       string                  `json:"status"`
	Type         string                  `json:"type"`
	PointOfSale  int                     `json:"point_of_sale"`
	Number       *int64                  `json:"number"`
	IssuedOn     string                  `json:"issued_on"`
	CAE          *string                 `json:"cae"`
	CAEExpiresOn *string                 `json:"cae_expires_on"`
	IssuerCUIT   string                  `json:"issuer_cuit"`
	Receiver     InvoiceReceiverResponse `json:"receiver"`
	Amounts      InvoiceAmountsResponse  `json:"amounts"`
	Currency     string                  `json:"currency"`
	Issues       []string                `json:"issues"`
	// QRURL is ARCA's verification link for an issued invoice (RG 4892), empty otherwise.
	QRURL     string    `json:"qr_url"`
	CreatedAt time.Time `json:"created_at"`
}

// InvoicePreviewResponse is returned by GET /v1/quotes/:quoteId/invoice: what would be issued now,
// every gap that stops it, and the invoice the quote already has, if any.
type InvoicePreviewResponse struct {
	VersionID   uuid.UUID               `json:"version_id"` // what POST confirms, with type and total.
	Type        string                  `json:"type"`
	PointOfSale *int                    `json:"point_of_sale"`
	Receiver    InvoiceReceiverResponse `json:"receiver"`
	Amounts     InvoiceAmountsResponse  `json:"amounts"`
	Currency    string                  `json:"currency"`
	Issues      []string                `json:"issues"`
	Invoice     *InvoiceResponse        `json:"invoice"`
}
