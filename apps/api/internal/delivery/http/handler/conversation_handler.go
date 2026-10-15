package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// ConversationReviewService is the seller's surface on the conversation handler's work.
type ConversationReviewService interface {
	DiscardAIChanges(ctx context.Context, tenant domain.Tenant,
		quoteID uuid.UUID) (*domain.QuoteVersion, error)
	RetryBatch(ctx context.Context, tenant domain.Tenant, quoteID,
		batchID uuid.UUID) (*domain.MessageBatch, error)
}

// ConversationHandler serves the seller's actions on a quote's conversation.
type ConversationHandler struct {
	reviews ConversationReviewService
}

// NewConversationHandler builds a ConversationHandler.
func NewConversationHandler(reviews ConversationReviewService) *ConversationHandler {
	return &ConversationHandler{reviews: reviews}
}

// DiscardAIChanges undoes what the AI changed on a change-requested draft.
//
//	@Summary		Discard the AI's changes to a draft
//	@Description	Resets the draft to the version the customer last received and records the pending AI changes as rejected.
//	@Tags			quotes
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string	true	"Active branch"
//	@Param			quoteId		path		string	true	"Quote id"
//	@Success		200			{object}	dto.QuoteVersionResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse	"No such quote in the selected branch"
//	@Failure		409			{object}	dto.ErrorResponse	"QUOTE_ARCHIVED or NO_AI_CHANGES"
//	@Failure		422			{object}	dto.ErrorResponse	"No active branch"
//	@Router			/v1/quotes/{quoteId}/discard-ai-changes [post]
func (h *ConversationHandler) DiscardAIChanges(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	quoteID, ok := pathUUID(c, "quoteId")
	if !ok {
		return
	}

	version, err := h.reviews.DiscardAIChanges(c.Request.Context(), tenant, quoteID)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toQuoteVersionResponse(*version))
}

// RetryBatch sends a batch that ran out of attempts back to the handler's queue.
//
//	@Summary		Retry a failed conversation batch
//	@Description	Requeues a batch that ran out of attempts while the handler may still act on its quote.
//	@Tags			quotes
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string	true	"Active branch"
//	@Param			quoteId		path		string	true	"Quote id"
//	@Param			batchId		path		string	true	"Batch id"
//	@Success		200			{object}	dto.ConversationBatchResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse	"No such quote or batch in the selected branch"
//	@Failure		409			{object}	dto.ErrorResponse	"BATCH_NOT_RETRYABLE"
//	@Failure		422			{object}	dto.ErrorResponse	"No active branch"
//	@Router			/v1/quotes/{quoteId}/message-batches/{batchId}/retry [post]
func (h *ConversationHandler) RetryBatch(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	quoteID, ok := pathUUID(c, "quoteId")
	if !ok {
		return
	}
	batchID, ok := pathUUID(c, "batchId")
	if !ok {
		return
	}

	batch, err := h.reviews.RetryBatch(c.Request.Context(), tenant, quoteID, batchID)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toConversationBatchResponse(*batch))
}
