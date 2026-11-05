package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// CustomerPortalService is the administrator's portal configuration surface.
type CustomerPortalService interface {
	Get(context.Context, domain.Tenant) (*domain.CustomerPortalSettings, error)
	Available(context.Context, domain.Tenant, string) (bool, error)
	Update(context.Context, domain.Tenant, domain.CustomerPortalUpdate) (*domain.CustomerPortalSettings, error)
}

// CustomerPortalHandler serves account-scoped portal configuration.
type CustomerPortalHandler struct{ service CustomerPortalService }

// NewCustomerPortalHandler builds a CustomerPortalHandler.
func NewCustomerPortalHandler(service CustomerPortalService) *CustomerPortalHandler {
	return &CustomerPortalHandler{service: service}
}

// Get returns the current portal settings.
// @Summary Get customer portal settings
// @Tags accounts
// @Produce json
// @Security BearerAuth
// @Success 200 {object} dto.CustomerPortalResponse
// @Failure 403 {object} dto.ErrorResponse
// @Router /v1/account/portal [get]
func (h *CustomerPortalHandler) Get(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	settings, err := h.service.Get(c.Request.Context(), tenant)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toCustomerPortalResponse(settings))
}

// Available checks a name without reserving it.
// @Summary Check customer subdomain availability
// @Tags accounts
// @Produce json
// @Security BearerAuth
// @Param subdomain query string true "Subdomain label"
// @Success 200 {object} dto.CustomerPortalAvailabilityResponse
// @Failure 422 {object} dto.ErrorResponse
// @Router /v1/account/portal/availability [get]
func (h *CustomerPortalHandler) Available(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	available, err := h.service.Available(c.Request.Context(), tenant, c.Query("subdomain"))
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.CustomerPortalAvailabilityResponse{Available: available})
}

// Update saves a server-constructed hostname for the authenticated administrator.
// @Summary Update customer subdomain
// @Tags accounts
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.UpdateCustomerPortalRequest true "Portal configuration"
// @Success 200 {object} dto.CustomerPortalResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 409 {object} dto.ErrorResponse
// @Failure 422 {object} dto.ErrorResponse
// @Router /v1/account/portal [put]
func (h *CustomerPortalHandler) Update(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	var body dto.UpdateCustomerPortalRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}
	settings, err := h.service.Update(c.Request.Context(), tenant, domain.CustomerPortalUpdate{Subdomain: body.Subdomain, ExpectedHostname: body.ExpectedHostname, ConfirmChange: body.ConfirmChange})
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toCustomerPortalResponse(settings))
}

func toCustomerPortalResponse(settings *domain.CustomerPortalSettings) dto.CustomerPortalResponse {
	return dto.CustomerPortalResponse{BaseURL: settings.BaseURL, Hostname: settings.Hostname, URL: settings.URL, Subdomain: settings.Subdomain, Legacy: settings.Legacy}
}
