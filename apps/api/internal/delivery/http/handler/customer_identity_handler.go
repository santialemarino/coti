package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/middleware"
	"github.com/santialemarino/coti/apps/api/internal/domain"
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
	response := dto.CustomerGoogleExchangeResponse{SignupToken: result.SignupToken, SignupEmail: result.SignupEmail, SignupEmailProven: result.SignupEmailProven}
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
// @Param body body dto.CustomerGoogleCompleteRequest true "Verified Google signup token, CUIT/CUIL and contacts"
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
	contacts := make([]domain.ClientContact, 0, len(in.Contacts))
	for _, contact := range in.Contacts {
		contacts = append(contacts, domain.ClientContact{Kind: domain.ChannelType(contact.Kind), Value: contact.Value, Primary: contact.Primary})
	}
	login, err := h.auth.CompleteGoogle(c.Request.Context(), c.Param("hostname"), domain.CustomerGoogleCompletion{Token: in.Token, TaxID: in.TaxID, Name: in.Name, Contacts: contacts, UseGoogleEmail: in.UseGoogleEmail}, principal)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, dto.CustomerSessionResponse{SessionToken: login.SessionToken, Customer: toCustomerIdentityResponse(login.Principal)})
}

// RecoveryOptions lists the masked verified contacts a recovery code can go to.
// @Summary List account recovery contacts
// @Tags customer-auth
// @Accept json
// @Produce json
// @Param hostname path string true "Registered supplier hostname"
// @Param body body dto.CustomerRecoveryOptionsRequest true "CUIT/CUIL of the account"
// @Success 200 {object} dto.CustomerRecoveryOptionsResponse
// @Failure 400,404,422 {object} dto.ErrorResponse
// @Failure 429 {object} dto.RateLimitResponse "Rate limit spent; retry_after_seconds says when"
// @Router /v1/customer/sites/{hostname}/auth/recovery/options [post]
func (h *CustomerAuthHandler) RecoveryOptions(c *gin.Context) {
	var in dto.CustomerRecoveryOptionsRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		RespondBindError(c, err)
		return
	}
	contacts, err := h.auth.RecoveryOptions(c.Request.Context(), c.Param("hostname"), in.TaxID)
	if err != nil {
		Respond(c, err)
		return
	}
	response := dto.CustomerRecoveryOptionsResponse{Contacts: make([]dto.CustomerRecoveryContactResponse, 0, len(contacts))}
	for _, contact := range contacts {
		response.Contacts = append(response.Contacts, dto.CustomerRecoveryContactResponse{ID: contact.ID, Kind: string(contact.Kind), Masked: contact.Masked, Main: contact.Main, Available: contact.Available, Preselected: contact.Preselected})
	}
	c.JSON(http.StatusOK, response)
}

// RecoveryCode sends a one-use recovery code to one verified contact of the account.
// @Summary Send an account recovery code
// @Tags customer-auth
// @Accept json
// @Produce json
// @Param hostname path string true "Registered supplier hostname"
// @Param body body dto.CustomerRecoveryCodeRequest true "CUIT/CUIL and the chosen contact"
// @Success 201 {object} dto.CustomerRecoveryCodeResponse
// @Failure 400,404,422,503 {object} dto.ErrorResponse
// @Failure 429 {object} dto.RateLimitResponse "RATE_LIMITED with retry_after_seconds, or ACCOUNT_LOCKED for a cooldown, destination cap or lockout"
// @Router /v1/customer/sites/{hostname}/auth/recovery/code [post]
func (h *CustomerAuthHandler) RecoveryCode(c *gin.Context) {
	var in dto.CustomerRecoveryCodeRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		RespondBindError(c, err)
		return
	}
	challenge, err := h.auth.RequestRecoveryCode(c.Request.Context(), c.Param("hostname"), in.TaxID, in.ContactID)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, dto.CustomerRecoveryCodeResponse{ID: challenge.ID, ExpiresAt: challenge.ExpiresAt})
}

// RecoveryReset redeems a recovery code, sets a new password and signs the customer in.
// @Summary Reset a customer password with a recovery code
// @Tags customer-auth
// @Accept json
// @Produce json
// @Param hostname path string true "Registered supplier hostname"
// @Param body body dto.CustomerRecoveryResetRequest true "Code and new password"
// @Success 201 {object} dto.CustomerSessionResponse
// @Failure 400,401,422 {object} dto.ErrorResponse
// @Failure 429 {object} dto.RateLimitResponse "RATE_LIMITED with retry_after_seconds, or ACCOUNT_LOCKED for a destination lockout"
// @Router /v1/customer/sites/{hostname}/auth/recovery/reset [post]
func (h *CustomerAuthHandler) RecoveryReset(c *gin.Context) {
	var in dto.CustomerRecoveryResetRequest
	if err := c.ShouldBindJSON(&in); err != nil {
		RespondBindError(c, err)
		return
	}
	login, err := h.auth.ResetWithRecoveryCode(c.Request.Context(), c.Param("hostname"), in.ID, in.Code, in.Password)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, dto.CustomerSessionResponse{SessionToken: login.SessionToken, Customer: toCustomerIdentityResponse(login.Principal)})
}
