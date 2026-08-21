package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// VerificationService is the address-confirmation surface the handler needs.
type VerificationService interface {
	Confirm(ctx context.Context, rawToken string) error
	Resend(ctx context.Context, email string) error
	ChangeOwnEmail(ctx context.Context, tenant domain.Tenant, currentPassword, newEmail string) error
}

// VerificationHandler serves address confirmation.
type VerificationHandler struct {
	verification VerificationService
	mailTarget   *MailTargetLimiter
}

// NewVerificationHandler builds a VerificationHandler.
func NewVerificationHandler(verification VerificationService, mailTarget *MailTargetLimiter) *VerificationHandler {
	return &VerificationHandler{verification: verification, mailTarget: mailTarget}
}

// Confirm redeems a verification link. Returns 204.
//
//	@Summary		Confirm an email address
//	@Description	The link works once and expires. Unknown, expired and already-used tokens all answer 401 alike. Confirming an address that is already verified succeeds.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body	dto.ConfirmEmailRequest	true	"Verification token"
//	@Success		204		"Address confirmed"
//	@Failure		400		{object}	dto.ErrorResponse
//	@Failure		401		{object}	dto.ErrorResponse
//	@Router			/v1/public/auth/verify-email [post]
func (h *VerificationHandler) Confirm(c *gin.Context) {
	var body dto.ConfirmEmailRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}

	if err := h.verification.Confirm(c.Request.Context(), body.Token); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Resend mails a fresh confirmation link. Returns 202 whether or not the address exists.
//
//	@Summary		Request a new confirmation link
//	@Description	Always answers 202, so the response reveals nothing about which addresses exist or which are already confirmed. Requesting a new link retires the previous one.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			body	body	dto.ResendVerificationRequest	true	"Address to send the link to"
//	@Success		202		"Accepted"
//	@Failure		400		{object}	dto.ErrorResponse
//	@Router			/v1/public/auth/resend-verification [post]
func (h *VerificationHandler) Resend(c *gin.Context) {
	var body dto.ResendVerificationRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}

	// Past the per-address cap the answer stays 202 and nothing is sent, for the same reason
	// every other outcome here answers 202: a distinct code would reveal the address.
	if !h.mailTarget.Allow(c.Request.Context(), body.Email) {
		c.Status(http.StatusAccepted)
		return
	}

	if err := h.verification.Resend(c.Request.Context(), body.Email); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusAccepted)
}

// ChangeEmail replaces the caller's own address. Returns 204, or 401 on a wrong password.
//
//	@Summary		Change your own email address
//	@Description	Requires the current password. Drops the confirmation, mails the new address a link, and retires any outstanding recovery link sent to the old one. Exempt from the confirmed-address requirement, so a mistyped address at signup is still correctable.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			body	body	dto.ChangeEmailRequest	true	"New address and the current password"
//	@Success		204		"Address changed"
//	@Failure		400		{object}	dto.ErrorResponse
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		409		{object}	dto.ErrorResponse	"The address is already in use, the caller's own included"
//	@Router			/v1/auth/change-email [post]
func (h *VerificationHandler) ChangeEmail(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}

	var body dto.ChangeEmailRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}

	if err := h.verification.ChangeOwnEmail(c.Request.Context(), tenant,
		body.CurrentPassword, body.NewEmail); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
