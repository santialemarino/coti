package handler

import (
	"context"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// ProductPriceImportService is the price-import surface the handler needs.
type ProductPriceImportService interface {
	Export(ctx context.Context, tenant domain.Tenant) (*domain.ProductPriceExportFile, error)
	Preview(ctx context.Context, tenant domain.Tenant, filename string, src io.Reader) (*domain.ProductPriceImportPreview, error)
	Confirm(ctx context.Context, tenant domain.Tenant, inputs []domain.ProductPriceImportInput) (int, error)
}

// ProductPriceHandler serves the reviewed product-price import flow.
type ProductPriceHandler struct {
	imports  ProductPriceImportService
	maxBytes int64
}

// NewProductPriceHandler builds a ProductPriceHandler.
func NewProductPriceHandler(imports ProductPriceImportService, maxBytes int64) *ProductPriceHandler {
	return &ProductPriceHandler{imports: imports, maxBytes: maxBytes}
}

// Export writes the branch's current prices as an editable XLSX template.
//
//	@Summary		Export the branch price list
//	@Description	Returns an XLSX pre-filled with the prices in force for the active branch, ready to edit and re-import.
//	@Tags			catalog
//	@Produce		application/vnd.openxmlformats-officedocument.spreadsheetml.sheet
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header	string	true	"Active branch"
//	@Success		200			{file}		binary
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		422			{object}	dto.ErrorResponse
//	@Router			/v1/product-prices/export [get]
func (h *ProductPriceHandler) Export(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	file, err := h.imports.Export(c.Request.Context(), tenant)
	if err != nil {
		Respond(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+file.Filename+`"`)
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", file.Content)
}

// PreviewImport validates an uploaded spreadsheet without changing any prices.
//
//	@Summary		Preview a price import
//	@Description	Parses the uploaded spreadsheet and reports every row with its proposed value and errors. Writes nothing.
//	@Tags			catalog
//	@Accept			multipart/form-data
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string	true	"Active branch"
//	@Param			file		formData	file	true	"Spreadsheet to preview (.xlsx or .csv)"
//	@Success		200			{object}	dto.ProductPriceImportPreviewResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		413			{object}	dto.ErrorResponse
//	@Failure		422			{object}	dto.ErrorResponse
//	@Router			/v1/product-prices/import/preview [post]
func (h *ProductPriceHandler) PreviewImport(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	file, filename, ok := openSpreadsheetUpload(c, h.maxBytes)
	if !ok {
		return
	}
	defer file.Close()

	preview, err := h.imports.Preview(c.Request.Context(), tenant, filename, file)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toProductPriceImportPreviewResponse(preview))
}

// ConfirmImport creates price versions only after the reviewed preview is confirmed.
//
//	@Summary		Confirm a price import
//	@Description	Revalidates the reviewed rows and, in one transaction, closes the current price periods and inserts the replacements.
//	@Tags			catalog
//	@Accept			json
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header		string									true	"Active branch"
//	@Param			request		body		dto.ConfirmProductPriceImportRequest	true	"Reviewed rows"
//	@Success		201			{object}	dto.ConfirmProductPriceImportResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		404			{object}	dto.ErrorResponse
//	@Failure		422			{object}	dto.ErrorResponse
//	@Router			/v1/product-prices/import/confirm [post]
func (h *ProductPriceHandler) ConfirmImport(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	var body dto.ConfirmProductPriceImportRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		RespondBindError(c, err)
		return
	}
	inputs := make([]domain.ProductPriceImportInput, len(body.Rows))
	for i, row := range body.Rows {
		inputs[i] = domain.ProductPriceImportInput{Code: row.Code, Price: row.Price,
			MinPrice: row.MinPrice}
	}
	importedRows, err := h.imports.Confirm(c.Request.Context(), tenant, inputs)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusCreated, dto.ConfirmProductPriceImportResponse{ImportedRows: importedRows})
}

func toProductPriceImportPreviewResponse(preview *domain.ProductPriceImportPreview) dto.ProductPriceImportPreviewResponse {
	rows := make([]dto.ProductPriceImportRowResponse, len(preview.Rows))
	for i, row := range preview.Rows {
		rows[i] = dto.ProductPriceImportRowResponse{
			RowNumber: row.RowNumber, Code: row.Code, ProductName: row.ProductName,
			CurrentPrice: row.CurrentPrice, CurrentMinPrice: row.CurrentMinPrice,
			Price: row.Price, MinPrice: row.MinPrice, Currency: row.Currency,
			Errors: append([]string{}, row.Errors...),
		}
	}
	return dto.ProductPriceImportPreviewResponse{
		Rows: rows, ValidRows: preview.ValidRows, InvalidRows: preview.InvalidRows,
		CanConfirm: preview.CanConfirm, PreviewedAt: preview.PreviewedAt,
	}
}
