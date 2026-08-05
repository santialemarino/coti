package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// DefaultCurrency is the currency a price takes when the caller does not name one.
const DefaultCurrency = "ARS"

// MoneyScale is how many decimals the NUMERIC(14,2) money and quantity columns keep. It
// fixes both what the API accepts and the scale amounts are rendered at.
const MoneyScale = 2

// ProductPrice is one validity period of a product's price at one branch. The table is
// append-only: a price is never overwritten, so it carries no updated_at.
type ProductPrice struct {
	ID        uuid.UUID
	AccountID uuid.UUID
	BranchID  uuid.UUID
	ProductID uuid.UUID
	UserID    *uuid.UUID      // who set it; nullable, since an import has no author.
	Price     decimal.Decimal // NUMERIC(14,2).
	Currency  string
	MinPrice  decimal.NullDecimal // NUMERIC(14,2); the floor the discount engine may not cross.
	ValidFrom time.Time
	ValidTo   *time.Time // NULL on the open period — the price in force.
	CreatedAt time.Time
}

// NewProductPrice is the input for opening a price period. An empty Currency resolves to
// DefaultCurrency and a zero ValidFrom to now, both in the service.
type NewProductPrice struct {
	Price     decimal.Decimal
	Currency  string
	MinPrice  decimal.NullDecimal
	ValidFrom time.Time
}

// ProductPriceLookup is a catalog product with its price currently in force for a branch.
type ProductPriceLookup struct {
	ProductID       uuid.UUID
	Code            string
	ProductName     string
	CurrentPrice    *string
	CurrentMinPrice *string
	CurrentCurrency *string
}

// ProductPriceExport is the current price list exported for one branch.
type ProductPriceExport struct {
	BranchName string
	Rows       []ProductPriceExportRow
}

// ProductPriceExportFile is an XLSX price list ready for download.
type ProductPriceExportFile struct {
	Filename string
	Content  []byte
}

// ProductPriceExportRow is one current branch price written to the XLSX template.
type ProductPriceExportRow struct {
	Code        string
	ProductName string
	Price       string
	MinPrice    *string
}

// ProductPriceImportRow is one spreadsheet row prepared for human review.
type ProductPriceImportRow struct {
	RowNumber       int
	Code            string
	ProductID       uuid.UUID
	ProductName     string
	CurrentPrice    *string
	CurrentMinPrice *string
	Price           string
	MinPrice        *string
	Currency        string
	Errors          []string
}

// ProductPriceUpdate is one validated price version to persist.
type ProductPriceUpdate struct {
	ProductID uuid.UUID
	Price     string
	MinPrice  *string
	Currency  string
}

// ProductPriceImportInput is one client-confirmed spreadsheet row.
type ProductPriceImportInput struct {
	Code     string
	Price    string
	MinPrice *string
}

// ProductPriceImportPreview summarizes a spreadsheet before it can be confirmed.
type ProductPriceImportPreview struct {
	Rows        []ProductPriceImportRow
	ValidRows   int
	InvalidRows int
	CanConfirm  bool
	PreviewedAt time.Time
}
