package domain

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

// QuoteRepresentationValidationError lists all source defects that prevent approval.
type QuoteRepresentationValidationError struct {
	Issues []string
}

// Error describes the invalid representation source.
func (e *QuoteRepresentationValidationError) Error() string {
	return "invalid quote representation: " + strings.Join(e.Issues, "; ")
}

// Unwrap classifies source validation as invalid input.
func (e *QuoteRepresentationValidationError) Unwrap() error {
	return ErrInvalidInput
}

// BrandLogo is a validated raster image safe for the PDF renderer.
type BrandLogo struct {
	Bytes       []byte
	ContentType string
}

// BrandLogoLoader fetches and validates one untrusted account logo.
type BrandLogoLoader interface {
	Load(ctx context.Context, rawURL string) (*BrandLogo, error)
}

// QuotePDFRenderer renders a canonical snapshot without reading external state.
type QuotePDFRenderer interface {
	Render(payload QuoteRepresentationPayload, logo *BrandLogo) ([]byte, error)
}

const (
	// QuoteRepresentationSchemaVersion is the canonical payload contract persisted with a PDF.
	QuoteRepresentationSchemaVersion = 1
	// QuotePublicURLPlaceholder is replaced only when a channel-specific token exists.
	QuotePublicURLPlaceholder = "{{public_url}}"
)

// QuoteRepresentation is one immutable client-facing bundle for a frozen quote version.
type QuoteRepresentation struct {
	ID               uuid.UUID
	AccountID        uuid.UUID
	BranchID         uuid.UUID
	QuoteID          uuid.UUID
	VersionID        uuid.UUID
	SchemaVersion    int
	Payload          QuoteRepresentationPayload
	Message          string
	PDFStorageKey    string
	PDFContentType   string
	PDFSizeBytes     int64
	PDFSHA256        string
	LogoFallbackUsed bool
	SelectionPricing *QuoteSelectionPricingSnapshot
	CreatedAt        time.Time
}

// NewQuoteRepresentation is the fully generated bundle ready for its immutable insert.
type NewQuoteRepresentation struct {
	ID               uuid.UUID
	BranchID         uuid.UUID
	QuoteID          uuid.UUID
	VersionID        uuid.UUID
	SchemaVersion    int
	Payload          QuoteRepresentationPayload
	Message          string
	PDFStorageKey    string
	PDFContentType   string
	PDFSizeBytes     int64
	PDFSHA256        string
	LogoFallbackUsed bool
	SelectionPricing *QuoteSelectionPricingSnapshot
}

// QuoteRepresentationPayload is the versioned, client-safe source shared by every format.
type QuoteRepresentationPayload struct {
	Reference     string                        `json:"reference"`
	VersionNumber int                           `json:"version_number"`
	ApprovedAt    time.Time                     `json:"approved_at"`
	Currency      string                        `json:"currency"`
	Supplier      QuoteRepresentationSupplier   `json:"supplier"`
	Branch        QuoteRepresentationBranch     `json:"branch"`
	Customer      QuoteRepresentationCustomer   `json:"customer"`
	Items         []QuoteRepresentationItem     `json:"items"`
	Discounts     []QuoteRepresentationDiscount `json:"discounts"`
	Total         string                        `json:"total"`
	ValidityNote  string                        `json:"validity_note"`
}

// QuoteRepresentationSupplier is the public identity frozen for the supplier.
type QuoteRepresentationSupplier struct {
	Name       string  `json:"name"`
	LegalName  *string `json:"legal_name,omitempty"`
	TaxID      *string `json:"tax_id,omitempty"`
	BrandColor string  `json:"brand_color"`
	// Absent on a snapshot frozen before the field existed, and on an account with no logo.
	LogoURL *string `json:"logo_url,omitempty"`
}

// QuoteRepresentationBranch is the public location identity frozen for the quote.
type QuoteRepresentationBranch struct {
	Name    string  `json:"name"`
	Address *string `json:"address,omitempty"`
}

// QuoteRepresentationCustomer is the minimal public customer identity.
type QuoteRepresentationCustomer struct {
	Address *string `json:"address,omitempty"`
	Name    *string `json:"name,omitempty"`
}

// QuoteRepresentationItem is one fully priced client-facing line.
type QuoteRepresentationItem struct {
	ID                   uuid.UUID                        `json:"id"`
	RequestedDescription string                           `json:"requested_description"`
	ProductCode          *string                          `json:"product_code,omitempty"`
	ProductName          string                           `json:"product_name"`
	Quantity             string                           `json:"quantity"`
	Unit                 *string                          `json:"unit,omitempty"`
	UnitPrice            string                           `json:"unit_price"`
	Subtotal             string                           `json:"subtotal"`
	Alternatives         []QuoteRepresentationAlternative `json:"alternatives"`
}

// QuoteRepresentationAlternative is one seller-approved priced option.
type QuoteRepresentationAlternative struct {
	ID                 uuid.UUID               `json:"id"`
	Code               *string                 `json:"code,omitempty"`
	Name               string                  `json:"name"`
	Unit               *string                 `json:"unit,omitempty"`
	UnitPrice          string                  `json:"unit_price"`
	RecommendationType *ProductAlternativeType `json:"recommendation_type,omitempty"`
	Selectable         bool                    `json:"selectable"`
}

// QuoteSelectionPricingSnapshot preserves the commercial rules available when a selectable offer
// was sent. It stays separate from the public payload because floors and rule definitions are not
// customer-facing data.
type QuoteSelectionPricingSnapshot struct {
	CapturedAt    time.Time                    `json:"captured_at"`
	Promotions    []Promotion                  `json:"promotions"`
	ProductGroups []QuoteSelectionProductGroup `json:"product_groups"`
}

// QuoteSelectionProductGroup freezes the taxonomy the promotion engine uses for one offered item.
type QuoteSelectionProductGroup struct {
	ProductID uuid.UUID    `json:"product_id"`
	Group     ProductGroup `json:"group"`
}

// QuoteRepresentationDiscount is one persisted discount the seller did not suppress.
type QuoteRepresentationDiscount struct {
	Description string `json:"description"`
	Amount      string `json:"amount"`
}

// QuoteRepresentationSource is the persisted data used once to build the canonical snapshot.
type QuoteRepresentationSource struct {
	Account      Account
	Branch       Branch
	Quote        Quote
	Version      QuoteVersion
	CustomerName *string
	Items        []QuoteItem
	Alternatives map[uuid.UUID][]QuoteItemAlternative
	Discounts    []QuoteRepresentationDiscount
}

// QuoteRepresentationResult decorates a stored bundle with a short-lived PDF URL.
type QuoteRepresentationResult struct {
	Representation QuoteRepresentation
	PDFURL         string
	Replay         bool
}

// PublicQuoteStatusUnavailable answers a live link to a quote the seller archived: the offer is
// withdrawn, so nothing of it is shown and no answer is taken.
const PublicQuoteStatusUnavailable = "UNAVAILABLE"

// PublicQuoteRepresentation is the active token response assembled without live commercial data.
type PublicQuoteRepresentation struct {
	Status    string
	ExpiresAt time.Time
	Payload   *QuoteRepresentationPayload
	Message   *string
	PDFURL    *string
	// CustomerStatus is the answer this send already received, absent while the customer
	// has not responded through the token.
	CustomerStatus *ClientActionType
}
