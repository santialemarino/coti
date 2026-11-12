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
	// The codes in the product's ECONOMY and PREMIUM recommendation slots, when the slot is filled.
	EconomyAlternative *string
	PremiumAlternative *string
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
	// A recommendation slot change: nil keeps the slot, an empty code clears it, a code sets it.
	EconomyAlternative *string
	PremiumAlternative *string
	Action             string
	Errors             []string
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
	// nil keeps the slot, an empty code clears it, a code sets it.
	EconomyAlternative *string
	PremiumAlternative *string
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

// CatalogProductRef is what an alternative check needs to know about a product named by code.
type CatalogProductRef struct {
	FamilyID   *uuid.UUID
	SubgroupID *uuid.UUID
	Unit       string
	IsActive   bool
}

// CatalogSlotChange sets or clears one recommendation slot of a product, both named by code.
type CatalogSlotChange struct {
	BaseCode string
	Type     ProductAlternativeType
	// Empty clears the slot.
	AlternativeCode string
}
