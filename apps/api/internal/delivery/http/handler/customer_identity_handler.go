package handler

import (
	"github.com/gin-gonic/gin"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/middleware"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"net/http"
)

// GetDeliverySite discovers public supplier routing without disclosing the linked order.
// @Summary Get supplier routing for a legacy delivery
// @Tags customer-auth
// @Produce json
// @Param token path string true "Delivery token"
// @Success 200 {object} dto.CustomerDeliverySiteResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /v1/public/quote-sends/{token}/site [get]
func (h *CustomerAuthHandler) GetDeliverySite(c *gin.Context) {
	hostname, err := h.auth.GetDeliveryHostname(c.Request.Context(), c.Param("token"))
	if err != nil {
		Respond(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, dto.CustomerDeliverySiteResponse{Hostname: hostname})
}

// LinkGoogle attaches a verified Google identity only after customer session authentication.
// @Summary Link Google to an authenticated customer
// @Tags customer-auth
// @Accept json
// @Security BearerAuth
// @Param hostname path string true "Registered supplier hostname"
// @Param body body dto.CustomerGoogleCompleteRequest true "Verified Google signup token and CUIT/CUIL"
// @Success 201 {object} dto.CustomerSessionResponse
// @Failure 400,401,409 {object} dto.ErrorResponse
// @Router /v1/customer/sites/{hostname}/auth/google/link [post]
func (h *CustomerAuthHandler) LinkGoogle(c *gin.Context) { h.CompleteGoogle(c) }

// GoogleOptions returns public Google configuration for a registered supplier.
// @Summary Get customer Google sign-in configuration
// @Tags customer-auth
// @Produce json
// @Param hostname path string true "Registered supplier hostname"
// @Success 200 {object} dto.CustomerGoogleOptionsResponse
// @Router /v1/customer/sites/{hostname}/auth/google/options [get]
func (h *CustomerAuthHandler) GoogleOptions(c *gin.Context) {
	options, err := h.auth.GoogleOptions(c.Request.Context(), c.Param("hostname"))
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.CustomerGoogleOptionsResponse{ClientID: options.ClientID, RedirectURI: options.RedirectURI, PasswordMinLength: options.PasswordMinLength, SupplierName: options.SupplierName, LogoURL: options.LogoURL})
}

// GoogleExchange verifies the provider's code before returning any customer session.
// @Summary Exchange a Google authorization code
// @Tags customer-auth
// @Accept json
// @Produce json
// @Param hostname path string true "Registered supplier hostname"
// @Param body body dto.CustomerGoogleExchangeRequest true "Authorization code and PKCE verifier"
// @Success 200 {object} dto.CustomerGoogleExchangeResponse
// @Failure 400,401,429 {object} dto.ErrorResponse
// @Router /v1/customer/sites/{hostname}/auth/google/exchange [post]
func (h *CustomerAuthHandler) GoogleExchange(c *gin.Context) {
	var in dto.CustomerGoogleExchangeRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		RespondBindError(c, err)
		return
	}
	result, err := h.auth.GoogleExchange(c.Request.Context(), c.Param("hostname"), in.Code, in.Verifier, in.Nonce)
	if err != nil {
		Respond(c, err)
		return
	}
	response := dto.CustomerGoogleExchangeResponse{SignupToken: result.SignupToken}
	if result.Login != nil {
		response.Session = &dto.CustomerSessionResponse{SessionToken: result.Login.SessionToken, Customer: toCustomerIdentityResponse(result.Login.Principal)}
	}
	c.JSON(http.StatusOK, response)
}

// CompleteGoogle registers new CUIT/CUIL credentials or explicitly links the authenticated account.
// @Summary Complete Google customer registration
// @Tags customer-auth
// @Accept json
// @Produce json
// @Param hostname path string true "Registered supplier hostname"
// @Param body body dto.CustomerGoogleCompleteRequest true "Verified Google signup token and CUIT/CUIL"
// @Success 201 {object} dto.CustomerSessionResponse
// @Failure 400,401,409 {object} dto.ErrorResponse
// @Router /v1/customer/sites/{hostname}/auth/google/complete [post]
func (h *CustomerAuthHandler) CompleteGoogle(c *gin.Context) {
	var in dto.CustomerGoogleCompleteRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		RespondBindError(c, err)
		return
	}
	var principal *domain.CustomerPrincipal
	if p, ok := middleware.CustomerFrom(c); ok {
		principal = &p
	}
	login, err := h.auth.CompleteGoogle(c.Request.Context(), c.Param("hostname"), in.Token, in.TaxID, principal)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, dto.CustomerSessionResponse{SessionToken: login.SessionToken, Customer: toCustomerIdentityResponse(login.Principal)})
}

// Forgot sends a recovery link only when the CUIT/CUIL and verified email match.
// @Summary Request customer password recovery
// @Tags customer-auth
// @Accept json
// @Param hostname path string true "Registered supplier hostname"
// @Param body body dto.CustomerRecoveryRequest true "CUIT/CUIL and recovery email"
// @Success 204 "Recovery request received"
// @Router /v1/customer/sites/{hostname}/auth/forgot-password [post]
func (h *CustomerAuthHandler) Forgot(c *gin.Context) {
	var in dto.CustomerRecoveryRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		RespondBindError(c, err)
		return
	}
	if err := h.auth.ForgotCustomerPassword(c.Request.Context(), c.Param("hostname"), in.TaxID, in.Email); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// VerifyEmail confirms an optional recovery address using its one-use capability.
// @Summary Verify a customer recovery email
// @Tags customer-auth
// @Accept json
// @Param hostname path string true "Registered supplier hostname"
// @Param body body dto.CustomerTokenRequest true "Verification token"
// @Success 204 "Email verified"
// @Router /v1/customer/sites/{hostname}/auth/verify-email [post]
func (h *CustomerAuthHandler) VerifyEmail(c *gin.Context) {
	var in dto.CustomerTokenRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		RespondBindError(c, err)
		return
	}
	if err := h.auth.VerifyCustomerEmail(c.Request.Context(), c.Param("hostname"), in.Token); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Reset replaces a recovered customer's password and revokes previous sessions.
// @Summary Reset a customer password
// @Tags customer-auth
// @Accept json
// @Param hostname path string true "Registered supplier hostname"
// @Param body body dto.CustomerPasswordResetRequest true "Recovery token and new password"
// @Success 204 "Password reset"
// @Router /v1/customer/sites/{hostname}/auth/reset-password [post]
func (h *CustomerAuthHandler) Reset(c *gin.Context) {
	var in dto.CustomerPasswordResetRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		RespondBindError(c, err)
		return
	}
	if err := h.auth.ResetCustomerPassword(c.Request.Context(), c.Param("hostname"), in.Token, in.Password); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
