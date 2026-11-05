package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// DataExportService is the account data export surface the handler needs.
type DataExportService interface {
	Export(ctx context.Context, tenant domain.Tenant, request domain.DataExportRequest) (*domain.DataExportFile, error)
}

// DataExportHandler serves account data as CSV or XML for another system.
type DataExportHandler struct {
	exports DataExportService
}

// NewDataExportHandler builds a DataExportHandler.
func NewDataExportHandler(exports DataExportService) *DataExportHandler {
	return &DataExportHandler{exports: exports}
}

// Export writes one entity of the account as a CSV or XML download.
//
//	@Summary		Export account data
//	@Description	Returns quotes, catalog, clients or invoices across the account, with Spanish field names.
//	@Tags			exports
//	@Produce		text/csv
//	@Produce		application/xml
//	@Security		BearerAuth
//	@Param			entity		path		string	true	"What to export"	Enums(quotes, catalog, clients, invoices)
//	@Param			format		query		string	true	"File format"		Enums(csv, xml)
//	@Param			branch_id	query		string	false	"Branch to narrow quotes, catalog prices and invoices to"
//	@Param			from		query		string	false	"First day of quotes or invoices, YYYY-MM-DD"
//	@Param			to			query		string	false	"Last day of quotes or invoices, YYYY-MM-DD"
//	@Success		200			{file}		binary
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		422			{object}	dto.ErrorResponse
//	@Router			/v1/exports/{entity} [get]
func (h *DataExportHandler) Export(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	file, err := h.exports.Export(c.Request.Context(), tenant, domain.DataExportRequest{
		Entity:   domain.DataExportEntity(c.Param("entity")),
		Format:   domain.DataExportFormat(c.Query("format")),
		BranchID: c.Query("branch_id"),
		From:     c.Query("from"),
		To:       c.Query("to"),
	})
	if err != nil {
		Respond(c, err)
		return
	}
	c.Header("Content-Disposition", `attachment; filename="`+file.Filename+`"`)
	c.Header("Cache-Control", "private, no-store")
	c.Data(http.StatusOK, file.ContentType, file.Content)
}
