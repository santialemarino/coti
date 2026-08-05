package domain

import (
	"time"

	"github.com/google/uuid"
)

// SynonymSource mirrors the native enum product_synonym_source.
//
// The endpoint accepts only MANUAL and LEARNED: IMPORTED is written by the bulk catalog
// import, which has its own path.
type SynonymSource string

const (
	// SynonymSourceManual is a term a person loaded in the backoffice.
	SynonymSourceManual SynonymSource = "MANUAL"
	// SynonymSourceLearned is a term the matching pipeline proposed from real RFQ text.
	SynonymSourceLearned SynonymSource = "LEARNED"
	// SynonymSourceImported is a term that arrived with a bulk catalog load.
	SynonymSourceImported SynonymSource = "IMPORTED"
)

// ProductAlternativeType is what the alternative is relative to its base product.
type ProductAlternativeType string

const (
	ProductAlternativeEquivalent ProductAlternativeType = "EQUIVALENT"
	ProductAlternativePremium    ProductAlternativeType = "PREMIUM"
	ProductAlternativeEconomy    ProductAlternativeType = "ECONOMY"
)

// AlternativeDirection is which end of product_alternative a read anchors on, so one
// method serves both readings of the relation.
type AlternativeDirection string

const (
	// AlternativeDirectionOutgoing lists what may be offered instead of the product.
	AlternativeDirectionOutgoing AlternativeDirection = "OUTGOING"
	// AlternativeDirectionIncoming lists the products this one is an alternative to.
	AlternativeDirectionIncoming AlternativeDirection = "INCOMING"
)

// Product is a catalog item owned by an account. What varies per branch lives in
// branch_product and product_price.
//
// The embedding column is deliberately absent: it is written by the vectorization
// pipeline, not by these endpoints.
type Product struct {
	ID            uuid.UUID
	AccountID     uuid.UUID
	Code          *string // nullable; unique per account when present.
	CanonicalName string
	Description   *string
	Unit          *string // nullable; free text (bolsa, m2, kg, ...).
	FamilyID      *uuid.UUID
	SubgroupID    *uuid.UUID
	IsActive      bool
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// NewProduct is the input for creating a catalog item.
type NewProduct struct {
	Code          *string
	CanonicalName string
	Description   *string
	Unit          *string
	FamilyID      uuid.UUID
	SubgroupID    *uuid.UUID
}

// ProductUpdate replaces a product's editable attributes: a nil nullable field clears
// the column. IsActive is the exception — nil leaves the flag untouched.
type ProductUpdate struct {
	Code          *string
	CanonicalName string
	Description   *string
	Unit          *string
	FamilyID      uuid.UUID
	SubgroupID    *uuid.UUID
	IsActive      *bool
}

// ProductFilter narrows a catalog listing. The service resolves Limit and Offset against
// the configured page size before they reach the repository.
type ProductFilter struct {
	Search          string
	FamilyID        *uuid.UUID
	SubgroupID      *uuid.UUID
	IncludeInactive bool
	Limit           int
	Offset          int
}

// ProductPage is one page of a catalog listing plus the total the filter matches.
type ProductPage struct {
	Items  []Product
	Total  int
	Limit  int
	Offset int
}

// ProductSynonym is a colloquial term that improves lexical catalog matching.
type ProductSynonym struct {
	ID        uuid.UUID
	AccountID uuid.UUID
	ProductID uuid.UUID
	Term      string
	Source    SynonymSource
	CreatedAt time.Time
}

// ProductAlternative links a base catalog item to another that can stand in for it.
type ProductAlternative struct {
	ID                   uuid.UUID
	AccountID            uuid.UUID
	BaseProductID        uuid.UUID
	AlternativeProductID uuid.UUID
	Type                 ProductAlternativeType
	CreatedAt            time.Time
}

// ProductAlternativeView is a link plus the product on the far end of it.
type ProductAlternativeView struct {
	Link    ProductAlternative
	Product Product
}
