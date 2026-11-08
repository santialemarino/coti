package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// RecommendationRequestService discovers and adds explicitly selected recommendations.
type RecommendationRequestService interface {
	Search(context.Context, domain.Tenant, uuid.UUID) ([]domain.RecommendationProposal, error)
	AddIfVacant(context.Context, domain.Tenant, uuid.UUID, uuid.UUID, uuid.UUID, domain.ProductAlternativeType) (*domain.QuoteItemAlternative, error)
}

// RecommendationRequestHandler serves seller-requested recommendations.
type RecommendationRequestHandler struct {
	service RecommendationRequestService
}

// NewRecommendationRequestHandler builds a RecommendationRequestHandler.
func NewRecommendationRequestHandler(service RecommendationRequestService) *RecommendationRequestHandler {
	return &RecommendationRequestHandler{service: service}
}

// Search discovers available alternatives without modifying the quote.
//
// @Summary Search for quote recommendations
// @Tags quotes
// @Produce json
// @Security BearerAuth
// @Param X-Branch-Id header string true "Active branch"
// @Param quoteId path string true "Quote id"
// @Success 200 {object} dto.RecommendationSearchResponse
// @Failure 400,401,404,409 {object} dto.ErrorResponse
// @Router /v1/quotes/{quoteId}/recommendations/search [post]
func (h *RecommendationRequestHandler) Search(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	quoteID, ok := pathUUID(c, "quoteId")
	if !ok {
		return
	}
	proposals, err := h.service.Search(c.Request.Context(), tenant, quoteID)
	if err != nil {
		Respond(c, err)
		return
	}
	items := make([]dto.RecommendationProposalResponse, 0, len(proposals))
	for _, p := range proposals {
		items = append(items, dto.RecommendationProposalResponse{
			ItemID: p.ItemID, ProductID: p.ProductID, RecommendationType: string(p.RecommendationType),
			Name: p.Name, Code: p.Code, Unit: p.Unit, UnitPrice: p.UnitPrice.StringFixed(2), Currency: p.Currency,
		})
	}
	c.JSON(http.StatusOK, dto.RecommendationSearchResponse{Items: items})
}

// AddIfVacant adds a proposal without replacing an existing recommendation.
//
// @Summary Add a suggested quote recommendation
// @Tags quotes
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param X-Branch-Id header string true "Active branch"
// @Param quoteId path string true "Quote id"
// @Param itemId path string true "Item id"
// @Param recommendationType path string true "ECONOMY or PREMIUM"
// @Param request body dto.AddRecommendationRequest true "Suggested product"
// @Success 200 {object} dto.QuoteItemAlternativeResponse
// @Failure 400,401,404,409,422 {object} dto.ErrorResponse
// @Router /v1/quotes/{quoteId}/items/{itemId}/recommendations/{recommendationType}/add [post]
func (h *RecommendationRequestHandler) AddIfVacant(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	quoteID, ok := pathUUID(c, "quoteId")
	if !ok {
		return
	}
	itemID, ok := pathUUID(c, "itemId")
	if !ok {
		return
	}
	kind, err := parseQuoteRecommendationType(c.Param("recommendationType"))
	if err != nil {
		Respond(c, err)
		return
	}
	var request dto.AddRecommendationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		RespondBindError(c, err)
		return
	}
	alternative, err := h.service.AddIfVacant(c.Request.Context(), tenant, quoteID, itemID, request.ProductID, kind)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toQuoteItemAlternativeResponse(*alternative))
}
