package handler

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// CatalogImportService is the initial-catalog import surface the handler needs.
type CatalogImportService interface {
	Template(ctx context.Context, tenant domain.Tenant) (*domain.CatalogImportFile, error)
	Preview(ctx context.Context, tenant domain.Tenant, filename string, src io.Reader) (*domain.CatalogImportPreview, error)
	Confirm(ctx context.Context, tenant domain.Tenant, inputs []domain.CatalogImportInput) (*domain.CatalogImportResult, error)
}

// CatalogImportHandler serves the reviewed initial-catalog import flow.
type CatalogImportHandler struct {
	imports  CatalogImportService
	maxBytes int64
}

// NewCatalogImportHandler builds a CatalogImportHandler.
func NewCatalogImportHandler(imports CatalogImportService, maxBytes int64) *CatalogImportHandler {
	return &CatalogImportHandler{imports: imports, maxBytes: maxBytes}
}

// Export writes the Spanish XLSX template for an initial catalog load.
//
//	@Summary		Download the initial catalog template
//	@Description	Returns a Spanish XLSX with the catalog columns and a second sheet of instructions.
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
//	@Summary		Preview an initial catalog import
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
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxBytes)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			c.JSON(http.StatusRequestEntityTooLarge, dto.ErrorResponse{Error: "file too large"})
			return
		}
		RespondBindError(c, err)
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		Respond(c, err)
		return
	}
	defer file.Close()

	preview, err := h.imports.Preview(c.Request.Context(), tenant, fileHeader.Filename, file)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toCatalogImportPreviewResponse(preview))
}

// Confirm revalidates the reviewed rows and creates every valid catalog entry.
//
//	@Summary		Confirm an initial catalog import
//	@Description	Revalidates all rows, skips invalid ones, and creates valid products, availability, and prices atomically.
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
			Category: row.Category, Price: row.Price, MinPrice: row.MinPrice,
			Currency: row.Currency, Conditions: row.Conditions,
		}
	}
	result, err := h.imports.Confirm(c.Request.Context(), tenant, inputs)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, dto.ConfirmCatalogImportResponse{
		ImportedRows: result.ImportedRows, SkippedRows: result.SkippedRows,
	})
}

func toCatalogImportPreviewResponse(preview *domain.CatalogImportPreview) dto.CatalogImportPreviewResponse {
	rows := make([]dto.CatalogImportRowResponse, len(preview.Rows))
	for i, row := range preview.Rows {
		rows[i] = dto.CatalogImportRowResponse{
			RowNumber: row.RowNumber, Code: row.Code, Name: row.Name,
			Description: row.Description, Unit: row.Unit, Category: row.Category,
			Price: row.Price, MinPrice: row.MinPrice, Currency: row.Currency,
			Conditions: row.Conditions, Errors: row.Errors,
		}
	}
	return dto.CatalogImportPreviewResponse{
		Rows: rows, ValidRows: preview.ValidRows, InvalidRows: preview.InvalidRows,
		CanConfirm: preview.CanConfirm, PreviewedAt: preview.PreviewedAt,
	}
}
