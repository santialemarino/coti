package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"
)

// ProductEmbeddingInput is a catalog item waiting for a vector, carrying the fields its text
// is composed from.
type ProductEmbeddingInput struct {
	ProductID     uuid.UUID
	CanonicalName string
	Description   *string
	// UpdatedAt is the row as it was read. Embedding a product takes a provider call, and an
	// edit landing during it would otherwise be overwritten by a vector of the older text.
	UpdatedAt time.Time
}

// EmbeddingText is what represents the product to the embedding model: the name the catalog
// gives it, plus whatever the description adds.
func (p ProductEmbeddingInput) EmbeddingText() string {
	name := strings.TrimSpace(p.CanonicalName)
	if p.Description == nil {
		return name
	}
	description := strings.TrimSpace(*p.Description)
	if description == "" {
		return name
	}
	return name + " " + description
}

// ProductEmbedding is one catalog item's computed vector, ready to be stored against the row
// version it was computed from.
type ProductEmbedding struct {
	ProductID uuid.UUID
	Vector    pgvector.Vector
	UpdatedAt time.Time
}

// CatalogEmbeddingReport is what one backfill run did, so the operator sees whether the
// catalog is fully vectorized without querying for it.
type CatalogEmbeddingReport struct {
	Embedded int
	Rounds   int
}
