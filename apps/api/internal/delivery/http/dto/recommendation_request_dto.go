package dto

import "github.com/google/uuid"

// RecommendationSearchResponse contains the seller-requested catalog suggestions.
type RecommendationSearchResponse struct {
	Items []RecommendationProposalResponse `json:"items"`
}

// RecommendationProposalResponse describes a candidate for a quote item.
type RecommendationProposalResponse struct {
	ItemID             uuid.UUID `json:"item_id"`
	ProductID          uuid.UUID `json:"product_id"`
	RecommendationType string    `json:"recommendation_type" enums:"ECONOMY,PREMIUM"`
	Name               string    `json:"name"`
	Code               *string   `json:"code"`
	Unit               *string   `json:"unit"`
	UnitPrice          string    `json:"unit_price"`
	Currency           string    `json:"currency"`
}

// AddRecommendationRequest identifies a suggested catalog product.
type AddRecommendationRequest struct {
	ProductID uuid.UUID `json:"product_id" binding:"required"`
}

// RecommendationFillResponse lists the catalog alternatives added to empty slots, by quote item.
type RecommendationFillResponse struct {
	Items []FilledRecommendationResponse `json:"items"`
}

// FilledRecommendationResponse is one alternative added to an item's empty slot.
type FilledRecommendationResponse struct {
	ItemID      uuid.UUID                    `json:"item_id"`
	Alternative QuoteItemAlternativeResponse `json:"alternative"`
}
