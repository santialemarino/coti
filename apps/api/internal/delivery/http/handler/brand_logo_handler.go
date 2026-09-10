package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// BrandLogoService is the account-logo surface the handler needs.
type BrandLogoService interface {
	DownloadLogo(ctx context.Context, accountID, logoID uuid.UUID) (*domain.StoredObject, error)
	UploadLogo(ctx context.Context, tenant domain.Tenant, file domain.AccountLogoUpload) (*domain.AccountLogo, error)
}

// BrandLogoHandler uploads and publicly serves account logos.
type BrandLogoHandler struct {
	logos    BrandLogoService
	maxBytes int64
}

// NewBrandLogoHandler builds a BrandLogoHandler.
func NewBrandLogoHandler(logos BrandLogoService, maxBytes int64) *BrandLogoHandler {
	return &BrandLogoHandler{logos: logos, maxBytes: maxBytes}
}

// Get streams one public account logo.
//
//	@Summary		Get an account logo
//	@Description	Serves one account logo by its public, unguessable identifier.
//	@Tags			accounts
//	@Produce		png
//	@Param			accountId	path		string	true	"Account id"
//	@Param			logoId		path		string	true	"Logo id"
//	@Success		200			{file}		binary
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Router			/v1/public/account-logos/{accountId}/{logoId} [get]
func (h *BrandLogoHandler) Get(c *gin.Context) {
	accountID, ok := pathUUID(c, "accountId")
	if !ok {
		return
	}
	logoID, ok := pathUUID(c, "logoId")
	if !ok {
		return
	}
	logo, err := h.logos.DownloadLogo(c.Request.Context(), accountID, logoID)
	if err != nil {
		Respond(c, err)
		return
	}
	defer logo.Body.Close()

	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Disposition", "inline")
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	c.DataFromReader(http.StatusOK, logo.Size, logo.ContentType, logo.Body, nil)
}

// Upload stores one logo for the caller's account.
//
//	@Summary		Upload an account logo
//	@Description	Stores one PNG or JPEG logo and returns its permanent public path.
//	@Tags			accounts
//	@Accept			multipart/form-data
//	@Produce		json
//	@Security		BearerAuth
//	@Param			file	formData	file	true	"PNG or JPEG logo"
//	@Success		201		{object}	dto.BrandLogoResponse
//	@Failure		400		{object}	dto.ErrorResponse
//	@Failure		401		{object}	dto.ErrorResponse
//	@Failure		403		{object}	dto.ErrorResponse
//	@Failure		413		{object}	dto.ErrorResponse	"FILE_TOO_LARGE"
//	@Failure		422		{object}	dto.ErrorResponse	"UNSUPPORTED_FILE_TYPE"
//	@Router			/v1/account/logo [post]
func (h *BrandLogoHandler) Upload(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	file, header, ok := openUpload(c, h.maxBytes)
	if !ok {
		return
	}
	defer file.Close()

	logo, err := h.logos.UploadLogo(c.Request.Context(), tenant, domain.AccountLogoUpload{
		ContentType: header.Header.Get("Content-Type"),
		Size:        header.Size,
		Content:     file,
	})
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, dto.BrandLogoResponse{Path: fmt.Sprintf(
		"/v1/public/account-logos/%s/%s", logo.AccountID, logo.ID)})
}
