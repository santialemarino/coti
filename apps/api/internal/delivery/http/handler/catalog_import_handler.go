package handler

import (
	"context"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// CatalogImportService is the bulk catalog editing surface the handler needs.
type CatalogImportService interface {
	Template(ctx context.Context, tenant domain.Tenant) (*domain.CatalogImportFile, error)
	Preview(ctx context.Context, tenant domain.Tenant, src io.Reader) (*domain.CatalogImportPreview, error)
	Confirm(ctx context.Context, tenant domain.Tenant, inputs []domain.CatalogImportInput) (*domain.CatalogImportResult, error)
	Taxonomy(ctx context.Context, tenant domain.Tenant) ([]domain.ProductFamily, error)
}

// CatalogImportHandler serves the reviewed bulk catalog editing flow.
type CatalogImportHandler struct {
	imports  CatalogImportService
	maxBytes int64
}

// NewCatalogImportHandler builds a CatalogImportHandler.
func NewCatalogImportHandler(imports CatalogImportService, maxBytes int64) *CatalogImportHandler {
	return &CatalogImportHandler{imports: imports, maxBytes: maxBytes}
}

// Export writes the Spanish XLSX populated with the branch's current catalog.
//
//	@Summary		Download the bulk catalog workbook
//	@Description	Returns a Spanish XLSX populated with current products, prices, and editing instructions.
//	@Tags			catalog
//	@Produce		application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header	string	true	"Active branch"
//	@Success		200			{file}	binary
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		422			{object}	dto.ErrorResponse
//	@Router			/v1/products/export [get]
func (h *CatalogImportHandler) Export(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	file, err := h.imports.Template(c.Request.Context(), tenant)
	if err != nil {
		Respond(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+file.Filename+`"`)
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", file.Content)
}

// Preview validates an uploaded catalog spreadsheet without changing the catalog.
//
//	@Summary		Preview bulk catalog changes
//	@Description	Parses the spreadsheet and reports every valid and invalid row without writing data.
//	@Tags			catalog
//	@Accept			multipart/form-data
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string	true	"Active branch"
//	@Param			file		formData	file	true	"Catalog spreadsheet to preview (.xlsx or .csv)"
//	@Success		200			{object}	dto.CatalogImportPreviewResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		413			{object}	dto.ErrorResponse
//	@Failure		422			{object}	dto.ErrorResponse
//	@Router			/v1/products/import/preview [post]
func (h *CatalogImportHandler) Preview(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	file, _, ok := openUpload(c, h.maxBytes)
	if !ok {
		return
	}
	defer file.Close()

	preview, err := h.imports.Preview(c.Request.Context(), tenant, file)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toCatalogImportPreviewResponse(preview))
}

// Confirm revalidates the reviewed rows and applies every valid catalog change.
//
//	@Summary		Confirm bulk catalog changes
//	@Description	Revalidates all rows, skips invalid ones, and atomically upserts products, availability, and changed prices.
//	@Tags			catalog
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string						true	"Active branch"
//	@Param			request		body		dto.ConfirmCatalogImportRequest	true	"Reviewed rows"
//	@Success		201			{object}	dto.ConfirmCatalogImportResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		409			{object}	dto.ErrorResponse
//	@Failure		422			{object}	dto.ErrorResponse
//	@Router			/v1/products/import/confirm [post]
func (h *CatalogImportHandler) Confirm(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	var body dto.ConfirmCatalogImportRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}
	inputs := make([]domain.CatalogImportInput, len(body.Rows))
	for i, row := range body.Rows {
		inputs[i] = domain.CatalogImportInput{
			Code: row.Code, Name: row.Name, Description: row.Description, Unit: row.Unit,
			Family: row.Family, Subgroup: row.Subgroup, Price: row.Price, MinPrice: row.MinPrice,
			IsActive: row.IsActive,
		}
	}
	result, err := h.imports.Confirm(c.Request.Context(), tenant, inputs)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, dto.ConfirmCatalogImportResponse{
		CreatedRows: result.CreatedRows, UpdatedRows: result.UpdatedRows, SkippedRows: result.SkippedRows,
	})
}

// Taxonomy returns the product families and subgroups used by catalog editors.
//
//	@Summary		List product taxonomy
//	@Tags			catalog
//	@Produce		json
//	@Security		BearerAuth
//	@Success		200	{object}	dto.ProductTaxonomyResponse
//	@Failure		401	{object}	dto.ErrorResponse
//	@Failure		403	{object}	dto.ErrorResponse
//	@Router			/v1/product-taxonomy [get]
func (h *CatalogImportHandler) Taxonomy(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	families, err := h.imports.Taxonomy(c.Request.Context(), tenant)
	if err != nil {
		Respond(c, err)
		return
	}
	items := make([]dto.ProductFamilyResponse, len(families))
	for i, family := range families {
		subgroups := make([]dto.ProductSubgroupResponse, len(family.Subgroups))
		for j, subgroup := range family.Subgroups {
			subgroups[j] = dto.ProductSubgroupResponse{ID: subgroup.ID.String(), Name: subgroup.Name}
		}
		items[i] = dto.ProductFamilyResponse{ID: family.ID.String(), Name: family.Name, Subgroups: subgroups}
	}
	c.JSON(http.StatusOK, dto.ProductTaxonomyResponse{Families: items})
}

func toCatalogImportPreviewResponse(preview *domain.CatalogImportPreview) dto.CatalogImportPreviewResponse {
	rows := make([]dto.CatalogImportRowResponse, len(preview.Rows))
	for i, row := range preview.Rows {
		rows[i] = dto.CatalogImportRowResponse{
			RowNumber: row.RowNumber, Code: row.Code, Name: row.Name,
			Description: row.Description, Unit: row.Unit, Family: row.Family,
			Subgroup: row.Subgroup, Price: row.Price, MinPrice: row.MinPrice,
			IsActive: row.IsActive, Action: row.Action, Errors: row.Errors,
		}
	}
	return dto.CatalogImportPreviewResponse{
		Rows: rows, ValidRows: preview.ValidRows, InvalidRows: preview.InvalidRows,
		CanConfirm: preview.CanConfirm, PreviewedAt: preview.PreviewedAt,
	}
}
