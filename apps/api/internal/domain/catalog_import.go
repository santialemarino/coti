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

// What confirming a catalog import row does: create the product, change it, or leave it as stored.
const (
	CatalogImportCreate    = "CREATE"
	CatalogImportUpdate    = "UPDATE"
	CatalogImportUnchanged = "UNCHANGED"
)

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
	// The valid rows by action; confirming writes only the ones to create or update.
	CreateRows    int
	UpdateRows    int
	UnchangedRows int
	CanConfirm    bool
	PreviewedAt   time.Time
}

// CatalogImportResult reports the rows created, updated, left as stored, and skipped after
// confirmation.
type CatalogImportResult struct {
	CreatedRows   int
	UpdatedRows   int
	UnchangedRows int
	SkippedRows   int
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

// CatalogSlots are the codes a product's ECONOMY and PREMIUM slots hold: nil when empty, "" when
// the slot holds a product with no code.
type CatalogSlots struct {
	Economy *string
	Premium *string
}

// CatalogProductState is a stored product as an import row is compared against it: its
// account-level attributes, plus its availability, price and slots at the import's branch.
type CatalogProductState struct {
	Name        string
	Description string
	Unit        string
	FamilyID    *uuid.UUID
	SubgroupID  *uuid.UUID
	IsActive    bool
	// Nil when the branch does not carry the product.
	BranchActive *bool
	// The price in force at the branch; nil when there is none.
	Price    *string
	MinPrice *string
	Slots    CatalogSlots
}
