package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

type whatsAppSignupService interface {
	GetReceipt(context.Context, domain.Tenant, uuid.UUID) (bool, error)
	Connect(context.Context, domain.Tenant, uuid.UUID, string, string, string) error
}

// WhatsAppSignupHandler holds the publisher's public SDK settings.
type WhatsAppSignupHandler struct {
	service whatsAppSignupService
	sdk     dto.WhatsAppSignupResponse
}

// WithWhatsAppSignup enables discovery and completion of Embedded Signup.
func (h *ChannelHandler) WithWhatsAppSignup(service whatsAppSignupService, enabled bool, appID, configID, version string) *ChannelHandler {
	h.signup = &WhatsAppSignupHandler{service: service}
	if enabled {
		h.signup.sdk = dto.WhatsAppSignupResponse{AppID: appID, ConfigurationID: configID, GraphVersion: version}
	}
	return h
}

// GetWhatsAppSignup returns SDK settings and evidence of receipt for one branch.
//
// @Summary Get WhatsApp signup settings
// @Tags channels
// @Security BearerAuth
// @Param X-Branch-Id header string true "Active branch"
// @Param channelId path string true "Channel id"
// @Success 200 {object} dto.WhatsAppSignupResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /v1/channels/{channelId}/whatsapp/signup [get]
func (h *ChannelHandler) GetWhatsAppSignup(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "channelId")
	if !ok {
		return
	}
	if h.signup == nil {
		Respond(c, domain.ErrNotConfigured)
		return
	}
	received, err := h.signup.service.GetReceipt(c.Request.Context(), tenant, id)
	if err != nil {
		Respond(c, err)
		return
	}
	response := h.signup.sdk
	response.Received = received
	c.JSON(http.StatusOK, response)
}

// ConnectWhatsAppSignup completes a server-side WhatsApp Embedded Signup.
//
// @Summary Connect a WhatsApp number from Meta
// @Tags channels
// @Accept json
// @Security BearerAuth
// @Param X-Branch-Id header string true "Active branch"
// @Param channelId path string true "Channel id"
// @Param body body dto.ConnectWhatsAppSignupRequest true "Meta signup result"
// @Success 204
// @Failure 400 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 409 {object} dto.ErrorResponse
// @Failure 422 {object} dto.ErrorResponse
// @Failure 503 {object} dto.ErrorResponse
// @Router /v1/channels/{channelId}/whatsapp/signup [post]
func (h *ChannelHandler) ConnectWhatsAppSignup(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	id, ok := pathUUID(c, "channelId")
	if !ok {
		return
	}
	if h.signup == nil || h.signup.sdk.AppID == "" {
		Respond(c, domain.ErrNotConfigured)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
	var body dto.ConnectWhatsAppSignupRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}
	if err := h.signup.service.Connect(c.Request.Context(), tenant, id, body.Code, body.BusinessAccountID, body.PhoneNumberID); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
