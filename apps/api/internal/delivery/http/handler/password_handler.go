package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// PasswordService is the credential-lifecycle surface the handler needs.
type PasswordService interface {
	ChangeOwn(ctx context.Context, tenant domain.Tenant, current, next string) (*domain.TokenPair, error)
	Forgot(ctx context.Context, email string) error
	Reset(ctx context.Context, rawToken, next string) error
	AdminReset(ctx context.Context, tenant domain.Tenant, userID uuid.UUID) error
}

// PasswordHandler serves the three ways a password changes after the user exists.
type PasswordHandler struct {
	passwords PasswordService
}

// NewPasswordHandler builds a PasswordHandler.
func NewPasswordHandler(passwords PasswordService) *PasswordHandler {
	return &PasswordHandler{passwords: passwords}
}

// Change replaces the caller's own password. Returns 401 when the current one is wrong.
//
//	@Summary		Change your own password
//	@Description	Requires the current password. Ends every other session and returns a fresh token pair, so the caller stays logged in and nobody else does.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body		dto.ChangePasswordRequest	true	"Current and new password"
//	@Success		200		{object}	dto.TokenResponse
//	@Failure		400		{object}	dto.ErrorResponse
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		422		{object}	dto.ErrorResponse	"New password below the configured minimum length"
//	@Router			/v1/auth/change-password [post]
func (h *PasswordHandler) Change(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}

	var body dto.ChangePasswordRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}

	pair, err := h.passwords.ChangeOwn(c.Request.Context(), tenant, body.CurrentPassword, body.NewPassword)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toTokenResponse(pair))
}

// Forgot mails a single-use recovery link. Returns 202 whether or not the address exists.
//
//	@Summary		Request a password recovery link
//	@Description	Always answers 202, registered address or not, so the response reveals nothing about which addresses exist. Requesting a new link retires the previous one.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body	dto.ForgotPasswordRequest	true	"Address to send the link to"
//	@Success		202		"Accepted"
//	@Failure		400		{object}	dto.ErrorResponse
//	@Router			/v1/public/auth/forgot-password [post]
func (h *PasswordHandler) Forgot(c *gin.Context) {
	var body dto.ForgotPasswordRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}

	if err := h.passwords.Forgot(c.Request.Context(), body.Email); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusAccepted)
}

// Reset redeems a recovery link and sets the new password. Returns 204.
//
//	@Summary		Reset a password with a recovery link
//	@Description	The link works once and expires. Unknown, expired and already-used tokens all answer 401 alike. Every session the user had is ended.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body	dto.ResetPasswordRequest	true	"Recovery token and new password"
//	@Success		204		"Password changed"
//	@Failure		400		{object}	dto.ErrorResponse
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		422		{object}	dto.ErrorResponse	"New password below the configured minimum length"
//	@Router			/v1/public/auth/reset-password [post]
func (h *PasswordHandler) Reset(c *gin.Context) {
	var body dto.ResetPasswordRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}

	if err := h.passwords.Reset(c.Request.Context(), body.Token, body.NewPassword); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// AdminReset mails a recovery link to another user of the caller's account. Returns 204.
//
//	@Summary		Trigger a password reset for a user
//	@Description	Admin only, and only within the caller's own account. Sends the user the same single-use link they would request themselves, so the administrator never sees a password.
//	@Tags			users
//	@Produce		json
//	@Security		BearerAuth
//	@Param			userId	path	string	true	"User id"
//	@Success		204		"Recovery link sent"
//	@Failure		400		{object}	dto.ErrorResponse
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse
//	@Failure		404		{object}	dto.ErrorResponse
//	@Failure		422		{object}	dto.ErrorResponse	"The user is deactivated"
//	@Router			/v1/users/{userId}/password-reset [post]
func (h *PasswordHandler) AdminReset(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	userID, ok := pathUUID(c, "userId")
	if !ok {
		return
	}

	if err := h.passwords.AdminReset(c.Request.Context(), tenant, userID); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
