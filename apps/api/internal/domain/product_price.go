package domain

import (
	"time"

	"github.com/google/uuid"
)

// ProductPriceLookup is a catalog product with its price currently in force for a branch.
type ProductPriceLookup struct {
	ProductID         uuid.UUID
	Code              string
	ProductName       string
	CurrentPrice      *string
	CurrentMinPrice   *string
	CurrentCurrency   *string
	CurrentConditions *string
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
	Currency    string
	Conditions  *string
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
	Conditions      *string
	Errors          []string
}

// ProductPriceUpdate is one validated price version to persist.
type ProductPriceUpdate struct {
	ProductID  uuid.UUID
	Price      string
	MinPrice   *string
	Currency   string
	Conditions *string
}

// ProductPriceImportInput is one client-confirmed spreadsheet row.
type ProductPriceImportInput struct {
	Code       string
	Price      string
	MinPrice   *string
	Currency   string
	Conditions *string
}

// ProductPriceImportPreview summarizes a spreadsheet before it can be confirmed.
type ProductPriceImportPreview struct {
	Rows        []ProductPriceImportRow
	ValidRows   int
	InvalidRows int
	CanConfirm  bool
	PreviewedAt time.Time
}
