package domain

import "time"

// CatalogImportFile is the Spanish XLSX template for an initial catalog load.
type CatalogImportFile struct {
	Filename string
	Content  []byte
}

// CatalogImportRow is one spreadsheet row prepared for human review.
type CatalogImportRow struct {
	RowNumber   int
	Code        string
	Name        string
	Description string
	Unit        string
	Category    *string
	Price       string
	MinPrice    *string
	Currency    string
	Conditions  *string
	Errors      []string
}

// CatalogImportInput is one reviewed catalog row sent for confirmation.
type CatalogImportInput struct {
	Code        string
	Name        string
	Description string
	Unit        string
	Category    *string
	Price       string
	MinPrice    *string
	Currency    string
	Conditions  *string
}

// CatalogImportPreview summarizes a catalog spreadsheet before confirmation.
type CatalogImportPreview struct {
	Rows        []CatalogImportRow
	ValidRows   int
	InvalidRows int
	CanConfirm  bool
	PreviewedAt time.Time
}

// CatalogImportResult reports the rows created and skipped after confirmation.
type CatalogImportResult struct {
	ImportedRows int
	SkippedRows  int
}
