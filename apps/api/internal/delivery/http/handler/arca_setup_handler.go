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
	Create(context.Context, domain.Tenant, string) (*domain.ARCASetup, error)
	Upload(context.Context, domain.Tenant, string) error
	Delete(context.Context, domain.Tenant) error
	Verify(context.Context, domain.Tenant, int) (domain.ARCAConnectionResult, error)
}

// ARCASetupHandler serves administrator-only homologation setup.
type ARCASetupHandler struct{ setup ARCASetupService }

// NewARCASetupHandler builds an ARCASetupHandler.
func NewARCASetupHandler(setup ARCASetupService) *ARCASetupHandler {
	return &ARCASetupHandler{setup: setup}
}

// Get returns public setup metadata.
// @Summary Get ARCA homologation setup
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
	response := dto.ARCASetupResponse{Enabled: enabled}
	if setup != nil {
		response.TaxID = setup.TaxID
		response.CSR = setup.CSR
		response.HasCertificate = setup.Certificate != ""
		response.CertificateExpiresAt = setup.CertificateExpiresAt
		response.PointOfSale = setup.PointOfSale
		response.VerifiedAt = setup.VerifiedAt
	}
	c.JSON(http.StatusOK, response)
}

// Create generates the account's first certificate request.
// @Summary Create ARCA homologation certificate request
// @Tags arca
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body dto.CreateARCASetupRequest true "Issuer CUIT"
// @Success 201 {object} dto.ARCASetupResponse
// @Router /v1/arca/setup [post]
func (h *ARCASetupHandler) Create(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	var body dto.CreateARCASetupRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}
	setup, err := h.setup.Create(c.Request.Context(), tenant, body.TaxID)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, dto.ARCASetupResponse{Enabled: true, TaxID: setup.TaxID, CSR: setup.CSR})
}

// Upload installs the public ARCA certificate matching the generated key.
// @Summary Install ARCA homologation certificate
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

// Delete disconnects the account and every branch.
// @Summary Disconnect ARCA homologation
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

// Verify checks access through a read-only request for a test invoice's last number.
// @Summary Verify ARCA homologation access
// @Tags arca
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param X-Branch-Id header string true "Active branch"
// @Param request body dto.VerifyARCASetupRequest true "Test point of sale"
// @Success 200 {object} dto.ARCAConnectionResponse
// @Router /v1/arca/setup/verify [post]
func (h *ARCASetupHandler) Verify(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	var body dto.VerifyARCASetupRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}
	result, err := h.setup.Verify(c.Request.Context(), tenant, body.PointOfSale)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, dto.ARCAConnectionResponse{Verified: result.Failure == "", Failure: result.Failure, LastNumber: result.LastNumber})
}
