package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/middleware"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// CustomerAuthService is the customer credential lifecycle used by the HTTP boundary.
type CustomerAuthService interface {
	GetDeliveryHostname(context.Context, string) (string, error)
	Register(context.Context, string, domain.CustomerRegistration) (*domain.CustomerLogin, error)
	Login(context.Context, string, domain.CustomerCredentials) (*domain.CustomerLogin, error)
	Logout(context.Context, domain.CustomerPrincipal) error
	GoogleOptions(context.Context, string) (*domain.CustomerAuthOptions, error)
	GoogleExchange(context.Context, string, string, string, string) (*domain.CustomerGoogleResult, error)
	CompleteGoogle(context.Context, string, domain.CustomerGoogleCompletion, *domain.CustomerPrincipal) (*domain.CustomerLogin, error)
	RecoveryOptions(context.Context, string, string) ([]domain.RecoveryContact, error)
	RequestRecoveryCode(context.Context, string, string, uuid.UUID) (*domain.ContactChallenge, error)
	ResetWithRecoveryCode(context.Context, string, uuid.UUID, string, string) (*domain.CustomerLogin, error)
}

// CustomerAuthHandler serves credentials independently of the staff authentication surface.
type CustomerAuthHandler struct {
	auth CustomerAuthService
}

// NewCustomerAuthHandler builds the customer authentication handler.
func NewCustomerAuthHandler(auth CustomerAuthService) *CustomerAuthHandler {
	return &CustomerAuthHandler{auth: auth}
}

// Me returns the customer scope verified for the current supplier.
//
// @Summary Get the authenticated customer
// @Tags customer-auth
// @Produce json
// @Security BearerAuth
// @Param hostname path string true "Registered supplier hostname"
// @Success 200 {object} dto.CustomerIdentityResponse
// @Failure 401 {object} dto.ErrorResponse
// @Router /v1/customer/sites/{hostname}/me [get]
func (h *CustomerAuthHandler) Me(c *gin.Context) {
	principal, ok := middleware.CustomerFrom(c)
	if !ok {
		Respond(c, domain.ErrUnauthenticated)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, toCustomerIdentityResponse(principal))
}

// Register creates CUIT/CUIL credentials and automatically starts the first session.
//
// @Summary Register a customer
// @Tags customer-auth
// @Accept json
// @Produce json
// @Param hostname path string true "Registered supplier hostname"
// @Param body body dto.CustomerRegistrationRequest true "CUIT/CUIL, credentials, full name and contacts"
// @Success 201 {object} dto.CustomerSessionResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 409 {object} dto.ErrorResponse
// @Failure 429 {object} dto.ErrorResponse
// @Router /v1/customer/sites/{hostname}/auth/register [post]
func (h *CustomerAuthHandler) Register(c *gin.Context) {
	var body dto.CustomerRegistrationRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}
	contacts := make([]domain.ClientContact, 0, len(body.Contacts))
	for _, contact := range body.Contacts {
		contacts = append(contacts, domain.ClientContact{Kind: domain.ChannelType(contact.Kind), Value: contact.Value, Primary: contact.Primary})
	}
	login, err := h.auth.Register(c.Request.Context(), c.Param("hostname"), domain.CustomerRegistration{TaxID: body.TaxID, Password: body.Password, Name: body.Name, Contacts: contacts})
	if err != nil {
		Respond(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusCreated, dto.CustomerSessionResponse{SessionToken: login.SessionToken, Customer: toCustomerIdentityResponse(login.Principal)})
}

// Login exchanges customer credentials for a supplier-scoped opaque session.
//
// @Summary Log in as a customer
// @Tags customer-auth
// @Accept json
// @Produce json
// @Param hostname path string true "Registered supplier hostname"
// @Param body body dto.CustomerCredentialsRequest true "CUIT/CUIL and password"
// @Success 200 {object} dto.CustomerSessionResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 429 {object} dto.ErrorResponse
// @Router /v1/customer/sites/{hostname}/auth/login [post]
func (h *CustomerAuthHandler) Login(c *gin.Context) {
	var body dto.CustomerCredentialsRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}
	login, err := h.auth.Login(c.Request.Context(), c.Param("hostname"), domain.CustomerCredentials{TaxID: body.TaxID, Password: body.Password})
	if err != nil {
		Respond(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, dto.CustomerSessionResponse{SessionToken: login.SessionToken, Customer: toCustomerIdentityResponse(login.Principal)})
}

// Logout revokes the current customer session in persistent storage.
//
// @Summary Log out as a customer
// @Tags customer-auth
// @Security BearerAuth
// @Param hostname path string true "Registered supplier hostname"
// @Success 204 "Logged out"
// @Failure 401 {object} dto.ErrorResponse
// @Router /v1/customer/sites/{hostname}/auth/logout [post]
func (h *CustomerAuthHandler) Logout(c *gin.Context) {
	principal, ok := middleware.CustomerFrom(c)
	if !ok {
		Respond(c, domain.ErrUnauthenticated)
		return
	}
	if err := h.auth.Logout(c.Request.Context(), principal); err != nil {
		Respond(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusNoContent)
}

func toCustomerIdentityResponse(principal domain.CustomerPrincipal) dto.CustomerIdentityResponse {
	return dto.CustomerIdentityResponse{ID: principal.CustomerID.String(), TaxID: principal.TaxID, Name: principal.Name, SessionExpiresAt: principal.SessionExpiresAt}
}
