package domain

import (
	"time"

	"github.com/google/uuid"
)

// DataExportEntity is what an account can download for use in another system.
type DataExportEntity string

const (
	DataExportQuotes   DataExportEntity = "quotes"
	DataExportCatalog  DataExportEntity = "catalog"
	DataExportClients  DataExportEntity = "clients"
	DataExportInvoices DataExportEntity = "invoices"
)

// IsValidDataExportEntity reports whether entity is one the export knows.
func IsValidDataExportEntity(entity DataExportEntity) bool {
	switch entity {
	case DataExportQuotes, DataExportCatalog, DataExportClients, DataExportInvoices:
		return true
	}
	return false
}

// DataExportFormat is the file format of an export.
type DataExportFormat string

const (
	DataExportCSV DataExportFormat = "csv"
	DataExportXML DataExportFormat = "xml"
)

// IsValidDataExportFormat reports whether format is one the export writes.
func IsValidDataExportFormat(format DataExportFormat) bool {
	return format == DataExportCSV || format == DataExportXML
}

// DataExportRequest is what the caller asked for; dates are inclusive YYYY-MM-DD in Argentina.
type DataExportRequest struct {
	Entity   DataExportEntity
	Format   DataExportFormat
	BranchID string
	From     string
	To       string
}

// DataExportFilter is a validated request: the branch and the period it narrows to, when given.
type DataExportFilter struct {
	AccountID uuid.UUID
	BranchID  *uuid.UUID
	// StartAt and EndAt bound a timestamp as [StartAt, EndAt); FromDate and ToDate bound a date.
	StartAt  *time.Time
	EndAt    *time.Time
	FromDate *string
	ToDate   *string
}

// DataExportFile is an export ready for download.
type DataExportFile struct {
	Filename    string
	ContentType string
	Content     []byte
}

// DataExportQuote is a quote with its current version's lines.
type DataExportQuote struct {
	ID            uuid.UUID
	Number        int64
	Branch        string
	Status        QuoteStatus
	ClientID      *uuid.UUID
	Client        *string
	Seller        *string
	CreatedAt     time.Time
	ExpiresAt     *time.Time
	ArchivedAt    *time.Time
	PaidAt        *time.Time
	NeedsFollowUp bool
	VersionNumber *int
	Currency      *string
	ItemsSubtotal *string
	Total         *string
	Items         []DataExportQuoteItem
}

// DataExportQuoteItem is one line of a quote's current version.
type DataExportQuoteItem struct {
	RequestedDescription string
	ProductCode          *string
	ProductName          *string
	Quantity             string
	Unit                 *string
	UnitPrice            *string
	Subtotal             *string
	MatchStatus          ItemMatchStatus
}

// DataExportProduct is a catalog product with what each branch carries and charges for it.
type DataExportProduct struct {
	ID          uuid.UUID
	Code        *string
	Name        string
	Description *string
	Unit        *string
	Family      *string
	Subgroup    *string
	VATRate     VATRate
	IsActive    bool
	Branches    []DataExportProductBranch
}

// DataExportProductBranch is a product's availability, stock and current price at one branch.
type DataExportProductBranch struct {
	Branch   string
	IsActive bool
	Stock    *string
	Price    *string
	MinPrice *string
	Currency *string
}

// DataExportClient is a client with its tags and live contacts.
type DataExportClient struct {
	ID            uuid.UUID
	Name          *string
	LegalName     *string
	TaxID         *string
	DNI           *string
	IVACondition  *IVACondition
	FiscalAddress *string
	Origin        *ClientOrigin
	Notes         *string
	Tags          []string
	CreatedAt     time.Time
	Contacts      []DataExportClientContact
}

// DataExportClientContact is one of a client's contacts that has not been retired.
type DataExportClientContact struct {
	Kind     ChannelType
	Value    string
	IsMain   bool
	Verified bool
}

// DataExportInvoice is one invoice or credit note, whatever its status.
type DataExportInvoice struct {
	ID                   uuid.UUID
	Kind                 VoucherKind
	Type                 InvoiceType
	PointOfSale          int
	Number               *int64
	IssuedOn             string
	Status               InvoiceStatus
	CAE                  *string
	CAEExpiresOn         *string
	Environment          ARCAEnvironment
	Branch               string
	QuoteNumber          int64
	CreditedInvoiceID    *uuid.UUID
	ReceiverName         string
	ReceiverDocType      ReceiverDocType
	ReceiverDocNumber    *string
	ReceiverIVACondition IVACondition
	NetAmount            string
	ExemptAmount         string
	VATAmount            string
	Total                string
	Currency             string
}
