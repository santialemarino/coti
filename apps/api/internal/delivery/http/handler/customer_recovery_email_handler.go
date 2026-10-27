package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/middleware"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// GetRecoveryEmail returns the current customer's optional recovery email.
// @Summary Get customer recovery email
// @Tags customer-auth
// @Produce json
// @Security BearerAuth
// @Param hostname path string true "Registered supplier hostname"
// @Success 200 {object} dto.CustomerRecoveryEmailResponse
// @Failure 401 {object} dto.ErrorResponse
// @Router /v1/customer/sites/{hostname}/auth/recovery-email [get]
func (h *CustomerAuthHandler) GetRecoveryEmail(c *gin.Context) {
	principal, ok := middleware.CustomerFrom(c)
	if !ok {
		Respond(c, domain.ErrUnauthenticated)
		return
	}
	result, err := h.auth.GetRecoveryEmail(c.Request.Context(), principal)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.CustomerRecoveryEmailResponse{Email: result.Email, Verified: result.Verified})
}

// UpdateRecoveryEmail saves the current customer's destination and requests its verification.
// @Summary Update customer recovery email
// @Tags customer-auth
// @Accept json
// @Security BearerAuth
// @Param hostname path string true "Registered supplier hostname"
// @Param body body dto.CustomerRecoveryEmailRequest true "Optional recovery email"
// @Success 204 "Recovery email saved"
// @Failure 400,401,429 {object} dto.ErrorResponse
// @Router /v1/customer/sites/{hostname}/auth/recovery-email [put]
func (h *CustomerAuthHandler) UpdateRecoveryEmail(c *gin.Context) {
	principal, ok := middleware.CustomerFrom(c)
	if !ok {
		Respond(c, domain.ErrUnauthenticated)
		return
	}
	var in dto.CustomerRecoveryEmailRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		RespondBindError(c, err)
		return
	}
	if err := h.auth.UpdateRecoveryEmail(c.Request.Context(), c.Param("hostname"), principal, in.Email); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
