package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// ARCASetupService is the setup surface required by the handler.
type ARCASetupService interface {
	Get(context.Context, domain.Tenant) (*domain.ARCASetup, bool, error)
	Create(context.Context, domain.Tenant) (*domain.ARCASetup, error)
	Upload(context.Context, domain.Tenant, string) error
	Delete(context.Context, domain.Tenant) error
	Environment() domain.ARCAEnvironment
}

// ARCASetupHandler serves the administrator-only ARCA connection setup.
type ARCASetupHandler struct{ setup ARCASetupService }

// NewARCASetupHandler builds an ARCASetupHandler.
func NewARCASetupHandler(setup ARCASetupService) *ARCASetupHandler {
	return &ARCASetupHandler{setup: setup}
}

// Get returns the account's public setup material.
// @Summary Get ARCA setup
// @Tags arca
// @Produce json
// @Security BearerAuth
// @Success 200 {object} dto.ARCASetupResponse
// @Router /v1/arca/setup [get]
func (h *ARCASetupHandler) Get(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	setup, enabled, err := h.setup.Get(c.Request.Context(), tenant)
	if err != nil {
		Respond(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	env := h.setup.Environment()
	response := dto.ARCASetupResponse{Enabled: enabled, Environment: string(env)}
	if setup != nil {
		setupEnv := string(setup.Environment)
		response.SetupEnvironment = &setupEnv
		response.ReconnectRequired = !setup.UsableIn(env)
		response.TaxID = setup.TaxID
		response.CSR = setup.CSR
		response.HasCertificate = setup.Certificate != ""
		response.Certificate = setup.Certificate
		response.CertificateExpiresAt = setup.CertificateExpiresAt
	}
	c.JSON(http.StatusOK, response)
}

// Create generates the account's first certificate request for the account's own CUIT.
// @Summary Create ARCA certificate request
// @Description Generates the key and the certificate request for the CUIT on the account; 422 ACCOUNT_TAX_ID_REQUIRED when the account has none.
// @Tags arca
// @Produce json
// @Security BearerAuth
// @Success 201 {object} dto.ARCASetupResponse
// @Failure 422 {object} dto.ErrorResponse
// @Router /v1/arca/setup [post]
func (h *ARCASetupHandler) Create(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	setup, err := h.setup.Create(c.Request.Context(), tenant)
	if err != nil {
		Respond(c, err)
		return
	}
	setupEnv := string(setup.Environment)
	c.JSON(http.StatusCreated, dto.ARCASetupResponse{Enabled: true, Environment: string(h.setup.Environment()),
		SetupEnvironment: &setupEnv, TaxID: setup.TaxID, CSR: setup.CSR})
}

// Upload installs the public ARCA certificate matching the generated key.
// @Summary Install ARCA certificate
// @Tags arca
// @Accept json
// @Security BearerAuth
// @Param request body dto.UploadARCACertificateRequest true "Public PEM certificate"
// @Success 204
// @Router /v1/arca/setup/certificate [put]
func (h *ARCASetupHandler) Upload(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	var body dto.UploadARCACertificateRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}
	if err := h.setup.Upload(c.Request.Context(), tenant, body.Certificate); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// Delete removes the account's ARCA credential, finished or in progress.
// @Summary Disconnect ARCA
// @Tags arca
// @Security BearerAuth
// @Success 204
// @Router /v1/arca/setup [delete]
func (h *ARCASetupHandler) Delete(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	if err := h.setup.Delete(c.Request.Context(), tenant); err != nil {
		Respond(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
