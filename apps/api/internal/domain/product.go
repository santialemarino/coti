package domain

import (
	"time"

	"github.com/google/uuid"
)

// SynonymSource is where a product synonym came from. The column is free text in the
// schema, so the closed set the API accepts is defined here.
type SynonymSource string

const (
	// SynonymSourceManual is a term a person loaded in the backoffice.
	SynonymSourceManual SynonymSource = "MANUAL"
	// SynonymSourceLearned is a term the matching pipeline proposed from real RFQ text.
	SynonymSourceLearned SynonymSource = "LEARNED"
)

// ProductAlternativeType is what the alternative is relative to its base product.
type ProductAlternativeType string

const (
	ProductAlternativeEquivalent ProductAlternativeType = "EQUIVALENT"
	ProductAlternativePremium    ProductAlternativeType = "PREMIUM"
	ProductAlternativeEconomy    ProductAlternativeType = "ECONOMY"
)

// AlternativeDirection is which end of product_alternative a read anchors on.
//
// It exists so one service method serves both readings of the relation: the
// recommendation engine asks what may be offered instead of a product, and the
// upsell path asks which products this one stands in for. Same table, same query,
// opposite anchor.
type AlternativeDirection string

const (
	// AlternativeDirectionOutgoing lists what may be offered instead of the product.
	AlternativeDirectionOutgoing AlternativeDirection = "OUTGOING"
	// AlternativeDirectionIncoming lists the products this one is an alternative to.
	AlternativeDirectionIncoming AlternativeDirection = "INCOMING"
)

// Product is a catalog item owned by an account.
//
// The catalog is account-scoped: one row, one embedding, and one set of synonyms and
// alternatives per account. What varies per branch lives in branch_product
// (availability and stock) and product_price (price and min_price).
//
// The embedding column is deliberately absent: it is 1536 floats that no catalog
// screen renders, and it is written by the vectorization pipeline, not by these
// endpoints.
type Product struct {
	ID            uuid.UUID
	AccountID     uuid.UUID
	Code          *string // nullable; unique per account when present.
	CanonicalName string
	Description   *string
	Unit          *string // nullable; free text (bolsa, m2, kg, ...).
	Category      *string
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
	Category      *string
}

// ProductUpdate replaces a product's editable attributes.
//
// A nil nullable field clears the column, so the caller sends the item as it should
// end up. IsActive is the exception: nil leaves the flag untouched, which keeps
// deactivation an explicit soft delete instead of a side effect of an edit form.
type ProductUpdate struct {
	Code          *string
	CanonicalName string
	Description   *string
	Unit          *string
	Category      *string
	IsActive      *bool
}

// ProductFilter narrows a catalog listing. Limit and Offset are resolved against the
// configured page size in the service before they reach the repository.
type ProductFilter struct {
	Search          string
	Category        string
	IncludeInactive bool
	Limit           int
	Offset          int
}

// ProductPage is one page of a catalog listing plus the total the filter matches, so a
// caller can render pagination without a second request.
type ProductPage struct {
	Items  []Product
	Total  int
	Limit  int
	Offset int
}

// ProductSynonym is a colloquial term that improves lexical catalog matching — the
// trade vocabulary a better model does not resolve on its own.
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

// ProductAlternativeView is a link plus the product on the far end of it, so listing
// alternatives is one query instead of one per row.
type ProductAlternativeView struct {
	Link    ProductAlternative
	Product Product
}
