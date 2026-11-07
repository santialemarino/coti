package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
)

// DeleteRecommendationAlternative removes a commercial recommendation from a mutable quote.
//
// @Summary Remove a quote item recommendation
// @Tags quotes
// @Security BearerAuth
// @Param X-Branch-Id header string true "Active branch"
// @Param quoteId path string true "Quote id"
// @Param itemId path string true "Item id"
// @Param alternativeId path string true "Alternative id"
// @Success 200 {object} dto.SuccessResponse
// @Failure 400,401,404,409 {object} dto.ErrorResponse
// @Router /v1/quotes/{quoteId}/items/{itemId}/alternatives/{alternativeId} [delete]
func (h *QuoteHandler) DeleteRecommendationAlternative(c *gin.Context) {
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
	alternativeID, ok := pathUUID(c, "alternativeId")
	if !ok {
		return
	}
	if err := h.quotes.DeleteRecommendationAlternative(c.Request.Context(), tenant, quoteID, itemID, alternativeID); err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.SuccessResponse{OK: true})
}
