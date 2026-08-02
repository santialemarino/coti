package handler

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/middleware"
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
func (h *ProductPriceHandler) Export(c *gin.Context) {
	tenant, ok := middleware.TenantFrom(c)
	if !ok {
		Respond(c, domain.ErrUnauthenticated)
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
func (h *ProductPriceHandler) PreviewImport(c *gin.Context) {
	tenant, ok := middleware.TenantFrom(c)
	if !ok {
		Respond(c, domain.ErrUnauthenticated)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, h.maxBytes)
	fileHeader, err := c.FormFile("file")
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "file too large"})
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
	c.JSON(http.StatusOK, toProductPriceImportPreviewResponse(preview))
}

// ConfirmImport creates price versions only after the reviewed preview is confirmed.
func (h *ProductPriceHandler) ConfirmImport(c *gin.Context) {
	tenant, ok := middleware.TenantFrom(c)
	if !ok {
		Respond(c, domain.ErrUnauthenticated)
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
			MinPrice: row.MinPrice, Currency: row.Currency, Conditions: row.Conditions}
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
			Conditions: row.Conditions, Errors: row.Errors,
		}
	}
	return dto.ProductPriceImportPreviewResponse{
		Rows: rows, ValidRows: preview.ValidRows, InvalidRows: preview.InvalidRows,
		CanConfirm: preview.CanConfirm, PreviewedAt: preview.PreviewedAt,
	}
}
