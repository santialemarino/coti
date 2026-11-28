package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// NonOrderService is the surface over channel messages that opened no order.
type NonOrderService interface {
	ListNonOrders(ctx context.Context, tenant domain.Tenant) ([]domain.InboundChannelMessage, error)
	CreateDraftFromNonOrder(ctx context.Context, tenant domain.Tenant, id uuid.UUID) (*domain.TextRFQDraft, error)
	DismissNonOrder(ctx context.Context, tenant domain.Tenant, id uuid.UUID) error
}

// InboundMessageHandler serves the messages that are not orders.
type InboundMessageHandler struct {
	messages       NonOrderService
	highConfidence decimal.Decimal
}

// NewInboundMessageHandler builds an InboundMessageHandler.
func NewInboundMessageHandler(messages NonOrderService, highConfidence decimal.Decimal) *InboundMessageHandler {
	return &InboundMessageHandler{messages: messages, highConfidence: highConfidence}
}

// ListNonOrders lists the branch's open messages that opened no order.
//
//	@Summary		List messages that are not orders
//	@Description	Greetings, questions and automated mail the active branch received and no seller has put away, newest first.
//	@Tags			inbound-messages
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string	true	"Active branch"
//	@Success		200			{object}	dto.NonOrderMessageListResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		422			{object}	dto.ErrorResponse
//	@Router			/v1/inbound-messages/non-orders [get]
func (h *InboundMessageHandler) ListNonOrders(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	messages, err := h.messages.ListNonOrders(c.Request.Context(), tenant)
	if err != nil {
		Respond(c, err)
		return
	}
	resp := dto.NonOrderMessageListResponse{Messages: make([]dto.NonOrderMessageResponse, 0, len(messages))}
	for _, message := range messages {
		resp.Messages = append(resp.Messages, toNonOrderMessageResponse(message))
	}
	c.JSON(http.StatusOK, resp)
}

// CreateOrder opens an order from a message that was not read as one.
//
//	@Summary		Open an order from a message that is not one
//	@Description	Reads the message through the order pipeline on the seller's behalf; one that still names no material becomes a failed order to load by hand.
//	@Tags			inbound-messages
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string	true	"Active branch"
//	@Param			messageId	path		string	true	"Message ID"
//	@Success		201			{object}	dto.TextRFQDraftResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Failure		422			{object}	dto.ErrorResponse
//	@Failure		429			{object}	dto.RateLimitResponse
//	@Failure		503			{object}	dto.ErrorResponse
//	@Router			/v1/inbound-messages/{messageId}/order [post]
func (h *InboundMessageHandler) CreateOrder(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "messageId")
	if !ok {
		return
	}
	draft, err := h.messages.CreateDraftFromNonOrder(c.Request.Context(), tenant, id)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, toTextRFQDraftResponse(*draft, h.highConfidence))
}

// Dismiss puts away a message that is not an order.
//
//	@Summary		Dismiss a message that is not an order
//	@Tags			inbound-messages
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header	string	true	"Active branch"
//	@Param			messageId	path	string	true	"Message ID"
//	@Success		204
//	@Failure		401	{object}	dto.ErrorResponse
//	@Failure		404	{object}	dto.ErrorResponse
//	@Router			/v1/inbound-messages/{messageId}/dismiss [post]
func (h *InboundMessageHandler) Dismiss(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "messageId")
	if !ok {
		return
	}
	if err := h.messages.DismissNonOrder(c.Request.Context(), tenant, id); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func toNonOrderMessageResponse(message domain.InboundChannelMessage) dto.NonOrderMessageResponse {
	resp := dto.NonOrderMessageResponse{
		ID: message.ID, ChannelID: message.ChannelID, SenderID: message.SenderID,
		SenderLabel: message.SenderLabel, Body: message.Body, ReceivedAt: message.ProviderReceivedAt,
	}
	if message.Kind != nil {
		resp.Kind = string(*message.Kind)
	}
	return resp
}
