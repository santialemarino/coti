package dto

import "github.com/google/uuid"

// PublicRFQDestinationResponse identifies the supplier receiving a public request.
type PublicRFQDestinationResponse struct {
	MaxTextCharacters    int      `json:"max_text_characters"`
	MaxFileSize          int64    `json:"max_file_size"`
	AcceptedContentTypes []string `json:"accepted_content_types"`
	SupplierName         string   `json:"supplier_name"`
	BranchName           string   `json:"branch_name"`
	LogoURL              *string  `json:"logo_url,omitempty"`
}

// CreatePublicRFQRequest is the body for POST /v1/public/rfq-channels/{channelId}/requests.
type CreatePublicRFQRequest struct {
	RequestID string `json:"request_id" form:"request_id" binding:"required,uuid4"`
	TaxID     string `json:"tax_id" form:"tax_id" binding:"required,max=32"`
	Name      string `json:"name" form:"name" binding:"required,max=255"`
	Email     string `json:"email" form:"email" binding:"omitempty,email,max=255"`
	Phone     string `json:"phone" form:"phone" binding:"max=64"`
	Text      string `json:"text" form:"text"`
}

// PublicRFQReceiptResponse confirms durable reception without exposing internal identifiers.
type PublicRFQReceiptResponse struct {
	Received bool `json:"received"`
}

// PublicRFQContactResponse contains declarations retained on the seller's request.
type PublicRFQContactResponse struct {
	TaxID string `json:"tax_id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone,omitempty"`
}

// PublicRFQSiteResponse contains a supplier's public branch choices.
type PublicRFQSiteResponse struct {
	MaxTextCharacters    int                           `json:"max_text_characters"`
	MaxFileSize          int64                         `json:"max_file_size"`
	AcceptedContentTypes []string                      `json:"accepted_content_types"`
	SupplierName         string                        `json:"supplier_name"`
	LogoURL              *string                       `json:"logo_url,omitempty"`
	Branches             []PublicRFQSiteBranchResponse `json:"branches"`
}

// PublicRFQSiteBranchResponse is one active public intake destination.
type PublicRFQSiteBranchResponse struct {
	Name      string    `json:"name"`
	ChannelID uuid.UUID `json:"channel_id"`
}
