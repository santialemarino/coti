package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
)

// SignupInviteService is the invite surface the handler needs.
type SignupInviteService interface {
	Check(ctx context.Context, token string) error
}

// SignupInviteHandler lets the signup screen refuse a dead invite before the wizard is filled in.
type SignupInviteHandler struct {
	invites SignupInviteService
}

// NewSignupInviteHandler builds a SignupInviteHandler.
func NewSignupInviteHandler(invites SignupInviteService) *SignupInviteHandler {
	return &SignupInviteHandler{invites: invites}
}

// Check answers whether a sign-up invite can still register a corralón. Returns 204.
//
//	@Summary		Check a sign-up invite
//	@Description	Redeems nothing: registration checks the invite again and uses it up.
//	@Tags			accounts
//	@Accept			json
//	@Produce		json
//	@Param			body	body	dto.SignupInviteCheckRequest	true	"Invite token"
//	@Success		204		"The invite can register a corralón"
//	@Failure		400		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse	"SIGNUP_INVITE_INVALID, SIGNUP_INVITE_USED or SIGNUP_INVITE_EXPIRED"
//	@Router			/v1/public/signup-invites/check [post]
func (h *SignupInviteHandler) Check(c *gin.Context) {
	var body dto.SignupInviteCheckRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}
	if err := h.invites.Check(c.Request.Context(), body.Token); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
