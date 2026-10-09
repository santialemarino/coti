package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// SellerReportService is the individual seller reporting surface the handler needs.
type SellerReportService interface {
	Get(ctx context.Context, tenant domain.Tenant, filter domain.SellerReportFilter) (*domain.SellerReport, error)
}

// SellerReportHandler serves authenticated seller reports.
type SellerReportHandler struct {
	reports SellerReportService
}

// NewSellerReportHandler builds a SellerReportHandler.
func NewSellerReportHandler(reports SellerReportService) *SellerReportHandler {
	return &SellerReportHandler{reports: reports}
}

// Get returns report metrics for the authenticated seller.
//
//	@Summary	Get seller reports
//	@Tags		reports
//	@Produce	json
//	@Security	BearerAuth
//	@Param		date_from	query		string	false	"First RFQ receipt date (YYYY-MM-DD)"
//	@Param		date_to		query		string	false	"Last RFQ receipt date (YYYY-MM-DD)"
//	@Success	200			{object}	dto.SellerReportResponse
//	@Failure	400			{object}	dto.ErrorResponse
//	@Failure	401			{object}	dto.ErrorResponse
//	@Failure	403			{object}	dto.ErrorResponse
//	@Failure	422			{object}	dto.ErrorResponse
//	@Router		/v1/reports [get]
func (h *SellerReportHandler) Get(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	var query dto.SellerReportQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		RespondBindError(c, err)
		return
	}
	filter, err := sellerReportFilter(query)
	if err != nil {
		RespondBindError(c, err)
		return
	}
	report, err := h.reports.Get(c.Request.Context(), tenant, filter)
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toSellerReportResponse(report))
}

func sellerReportFilter(query dto.SellerReportQuery) (domain.SellerReportFilter, error) {
	var filter domain.SellerReportFilter
	if query.DateFrom != "" {
		date, err := time.Parse(time.DateOnly, query.DateFrom)
		if err != nil {
			return filter, err
		}
		filter.DateFrom = &date
	}
	if query.DateTo != "" {
		date, err := time.Parse(time.DateOnly, query.DateTo)
		if err != nil {
			return filter, err
		}
		filter.DateTo = &date
	}
	return filter, nil
}

func toSellerReportResponse(report *domain.SellerReport) dto.SellerReportResponse {
	resp := dto.SellerReportResponse{
		OrdersReceived:          report.OrdersReceived,
		QuotesSent:              report.QuotesSent,
		QuotesAccepted:          report.QuotesAccepted,
		AverageQuoteTimeSeconds: report.AverageQuoteTimeSeconds,
		Statuses:                make([]dto.SellerReportStatusResponse, len(report.Statuses)),
		TopMaterials:            make([]dto.SellerReportMaterialResponse, len(report.TopMaterials)),
		TopClients:              make([]dto.SellerReportClientResponse, len(report.TopClients)),
	}
	for index, item := range report.Statuses {
		resp.Statuses[index] = dto.SellerReportStatusResponse{Status: item.Status, Count: item.Count}
	}
	for index, item := range report.TopMaterials {
		resp.TopMaterials[index] = dto.SellerReportMaterialResponse{
			Name: item.Name, OrderCount: item.OrderCount,
		}
	}
	for index, item := range report.TopClients {
		resp.TopClients[index] = dto.SellerReportClientResponse{
			Name: item.Name, OrderCount: item.OrderCount,
		}
	}
	return resp
}
