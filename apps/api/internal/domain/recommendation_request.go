package domain

import (
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// RecommendationProposal is an available catalog alternative for a quote item.
type RecommendationProposal struct {
	ItemID             uuid.UUID
	ProductID          uuid.UUID
	RecommendationType ProductAlternativeType
	Name               string
	Code               *string
	Unit               *string
	UnitPrice          decimal.Decimal
	Currency           string
}

// FilledRecommendation is a catalog alternative added to an empty slot of a quote item.
type FilledRecommendation struct {
	ItemID      uuid.UUID
	Alternative QuoteItemAlternative
}
