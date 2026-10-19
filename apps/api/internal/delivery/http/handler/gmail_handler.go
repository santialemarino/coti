package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// GmailAuthorization is the OAuth connection surface used by the HTTP handler.
type GmailAuthorization interface {
	Start(context.Context, domain.Tenant, uuid.UUID) (string, error)
	Complete(context.Context, string, string) error
}

// GmailHandler serves the branch mailbox authorization flow.
type GmailHandler struct {
	authorization GmailAuthorization
	backofficeURL string
}

// NewGmailHandler builds the Gmail connection handler.
func NewGmailHandler(authorization GmailAuthorization, backofficeURL string) *GmailHandler {
	return &GmailHandler{authorization: authorization, backofficeURL: strings.TrimRight(backofficeURL, "/")}
}

// Start begins Google authorization for an active email channel.
//
//	@Summary		Authorize a branch Gmail mailbox
//	@Tags			channels
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id header string true "Active branch"
//	@Param			channelId path string true "Email channel id"
//	@Success		200 {object} dto.GmailAuthorizationResponse
//	@Failure		403 {object} dto.ErrorResponse
//	@Failure		503 {object} dto.ErrorResponse
//	@Router			/v1/channels/{channelId}/gmail/authorization [post]
func (h *GmailHandler) Start(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	channelID, ok := pathUUID(c, "channelId")
	if !ok {
		return
	}
	url, err := h.authorization.Start(c.Request.Context(), tenant, channelID)
	if err != nil {
		Respond(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, dto.GmailAuthorizationResponse{AuthorizationURL: url})
}

// Callback completes the single-use OAuth flow and returns to the branch settings.
func (h *GmailHandler) Callback(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Referrer-Policy", "no-referrer")
	status := "failed"
	if c.Query("error") == "" && h.authorization.Complete(c.Request.Context(), c.Query("state"), c.Query("code")) == nil {
		status = "connected"
	}
	c.Redirect(http.StatusSeeOther, h.backofficeURL+"/settings/branches?gmail="+status)
}
