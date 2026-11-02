package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// QuoteFollowupService drafts the message that chases a quote the client has not answered.
type QuoteFollowupService interface {
	Draft(ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID) (string, error)
}

// QuoteFollowupHandler serves the seller's follow-up drafts.
type QuoteFollowupHandler struct {
	followups QuoteFollowupService
}

// NewQuoteFollowupHandler builds a QuoteFollowupHandler.
func NewQuoteFollowupHandler(followups QuoteFollowupService) *QuoteFollowupHandler {
	return &QuoteFollowupHandler{followups: followups}
}

// Draft writes a fresh follow-up message for a sent quote; nothing is sent or stored.
//
//	@Summary		Draft a follow-up message
//	@Description	Drafts, from the quote as it stands, a message the seller can edit and send to a client who has not answered. It contains no prices and is never sent by Coti.
//	@Tags			quotes
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string	true	"Active branch"
//	@Param			quoteId		path		string	true	"Quote id"
//	@Success		200			{object}	dto.QuoteFollowupDraftResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse	"No such quote in the selected branch"
//	@Failure		409			{object}	dto.ErrorResponse	"QUOTE_ARCHIVED or QUOTE_NOT_SENT"
//	@Failure		422			{object}	dto.ErrorResponse	"No active branch"
//	@Failure		503			{object}	dto.ErrorResponse	"AI_UNAVAILABLE"
//	@Router			/v1/quotes/{quoteId}/followup-draft [post]
func (h *QuoteFollowupHandler) Draft(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	quoteID, ok := pathUUID(c, "quoteId")
	if !ok {
		return
	}

	message, err := h.followups.Draft(c.Request.Context(), tenant, quoteID)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.QuoteFollowupDraftResponse{Message: message})
}
