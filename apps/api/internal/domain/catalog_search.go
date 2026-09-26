package domain

import (
	"github.com/google/uuid"
)

// CatalogSearch is one hybrid catalog lookup: several RFQ lines resolved in a single pass
// against the account's catalog, narrowed to what one branch carries.
type CatalogSearch struct {
	// Texts are the line descriptions to resolve, as they arrived from the client.
	Texts []string
	// Limit is how many candidates each text gets back. Zero takes the configured default.
	Limit int
}

// CatalogCandidate is one catalog item a search offered for a line, carrying both halves'
// evidence. Neither figure decides a match: the matching service reads them and assigns
// match_status, so a line with no acceptable candidate is flagged rather than dropped.
type CatalogCandidate struct {
	ProductID     uuid.UUID
	Code          *string
	CanonicalName string
	Description   *string
	Unit          *string
	// Synonyms are the account's trade terms for this product that the line contained, so a
	// match through "portland" reads as covered even though the name says "cemento".
	Synonyms []string
	// Distance is the cosine distance to the line's vector, and is nil when the product has no
	// vector yet.
	Distance *float64
	// LexicalScore ranks the full-text match, and is nil when only the semantic half found it.
	LexicalScore *float64
	// LearnedDistance is cosine distance to a seller-approved phrase for this product.
	LearnedDistance *float64
	// LearnedConfirmed is true when the seller taught this product for this very phrase and later
	// kept it on a line that was flagged: a choice made twice.
	LearnedConfirmed bool
	// Score is the fused rank the candidates are ordered by, best first.
	Score float64
}

// CatalogSearchResult is what one line of a search resolved to, best candidate first.
type CatalogSearchResult struct {
	Text       string
	Candidates []CatalogCandidate
}
