package domain

import (
	"time"

	"github.com/google/uuid"
)

// ProductFamily is a global catalog family available to every account.
type ProductFamily struct {
	ID        uuid.UUID
	Name      string
	Subgroups []ProductSubgroup
}

// ProductSubgroup is an optional classification within one product family.
type ProductSubgroup struct {
	ID       uuid.UUID
	FamilyID uuid.UUID
	Name     string
}

// CatalogImportFile is the Spanish XLSX used for bulk catalog creation and editing.
type CatalogImportFile struct {
	Filename string
	Content  []byte
}

// CatalogExport is the account catalog together with values scoped to one branch.
type CatalogExport struct {
	BranchName string
	Rows       []CatalogExportRow
}

// CatalogExportRow is one product written to the bulk-edit workbook.
type CatalogExportRow struct {
	Code        string
	Name        string
	Description string
	Unit        string
	Family      string
	Subgroup    *string
	Price       *string
	MinPrice    *string
	IsActive    bool
}

// CatalogImportRow is one spreadsheet row prepared for human review.
type CatalogImportRow struct {
	RowNumber   int
	Code        string
	Name        string
	Description string
	Unit        string
	FamilyID    uuid.UUID
	Family      string
	SubgroupID  *uuid.UUID
	Subgroup    *string
	Price       string
	MinPrice    *string
	IsActive    bool
	Action      string
	Errors      []string
}

// CatalogImportInput is one reviewed catalog row sent for confirmation.
type CatalogImportInput struct {
	Code        string
	Name        string
	Description string
	Unit        string
	Family      string
	Subgroup    *string
	Price       string
	MinPrice    *string
	IsActive    bool
}

// CatalogImportPreview summarizes a catalog spreadsheet before confirmation.
type CatalogImportPreview struct {
	Rows        []CatalogImportRow
	ValidRows   int
	InvalidRows int
	CanConfirm  bool
	PreviewedAt time.Time
}

// CatalogImportResult reports the rows created, updated, and skipped after confirmation.
type CatalogImportResult struct {
	CreatedRows int
	UpdatedRows int
	SkippedRows int
}
