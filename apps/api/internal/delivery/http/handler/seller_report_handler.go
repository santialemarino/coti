package handler

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// SellerReportService is the reporting surface the handler needs.
type SellerReportService interface {
	Get(context.Context, domain.Tenant, string, string) (*domain.SellerReport, error)
}

// SellerReportHandler serves the authenticated seller dashboard.
type SellerReportHandler struct {
	reports SellerReportService
}

// NewSellerReportHandler builds a SellerReportHandler.
func NewSellerReportHandler(reports SellerReportService) *SellerReportHandler {
	return &SellerReportHandler{reports: reports}
}

// Get returns the report for the requested period, defaulting to the last 30 days.
//
//	@Summary		Get seller report
//	@Description	Returns audited order and quote metrics for the caller's branch and seller reach.
//	@Tags			reports
//	@Produce		json
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header	string	false	"Active branch; omitted reports on every branch the caller reaches"
//	@Param			from		query	string	false	"First day, YYYY-MM-DD; defaults to 30 days ago"
//	@Param			to			query	string	false	"Last day, YYYY-MM-DD; defaults to today"
//	@Success		200			{object}	dto.SellerReportResponse
//	@Failure		400			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Router			/v1/reports [get]
func (h *SellerReportHandler) Get(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	report, err := h.reports.Get(c.Request.Context(), tenant, c.Query("from"), c.Query("to"))
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toSellerReportResponse(*report))
}

func toSellerReportResponse(report domain.SellerReport) dto.SellerReportResponse {
	response := dto.SellerReportResponse{
		Range: dto.SellerReportRangeResponse{
			From: report.Range.From,
			To:   report.Range.To,
		},
		Scope: dto.SellerReportScopeResponse{
			MultiBranch: report.Scope.MultiBranch,
			BranchID:    report.Scope.BranchID,
		},
		KPIs: dto.SellerReportKPIsResponse{
			RFQsReceived:   report.KPIs.RFQsReceived,
			QuotesSent:     report.KPIs.QuotesSent,
			QuotesAccepted: report.KPIs.QuotesAccepted,
			ConversionRate: decimalString(report.KPIs.ConversionRate, 4),
			AcceptedAmount: report.KPIs.AcceptedAmount.StringFixed(2),
			ResponseTime: dto.SellerReportResponseTimeResponse{
				AverageSeconds: report.KPIs.ResponseTime.AverageSeconds,
				MedianSeconds:  report.KPIs.ResponseTime.MedianSeconds,
				Sample:         report.KPIs.ResponseTime.Sample,
			},
		},
		Branches:       make([]dto.SellerReportBranchResponse, 0, len(report.Branches)),
		QuotesByStatus: make([]dto.SellerReportStatusResponse, 0, len(report.QuotesByStatus)),
		Timeline:       make([]dto.SellerReportTimelineResponse, 0, len(report.Timeline)),
		TopProducts:    make([]dto.SellerReportProductResponse, 0, len(report.TopProducts)),
		TopClients:     make([]dto.SellerReportClientResponse, 0, len(report.TopClients)),
		AIMatch: dto.SellerReportAIMatchResponse{
			Matched: report.AIMatch.Matched,
			Total:   report.AIMatch.Total,
			Rate:    decimalString(report.AIMatch.Rate, 4),
			Confidence: dto.SellerReportConfidenceResponse{
				Low:    report.AIMatch.Confidence.Low,
				Medium: report.AIMatch.Confidence.Medium,
				High:   report.AIMatch.Confidence.High,
			},
		},
	}
	for _, branch := range report.Branches {
		response.Branches = append(response.Branches, dto.SellerReportBranchResponse{
			BranchID: branch.BranchID, BranchName: branch.BranchName, RFQs: branch.RFQs,
		})
	}
	for _, status := range report.QuotesByStatus {
		response.QuotesByStatus = append(response.QuotesByStatus, dto.SellerReportStatusResponse{
			Status: string(status.Status), Count: status.Count,
		})
	}
	for _, point := range report.Timeline {
		response.Timeline = append(response.Timeline, dto.SellerReportTimelineResponse{
			Date: point.Date, Sent: point.Sent, Accepted: point.Accepted,
		})
	}
	for _, product := range report.TopProducts {
		response.TopProducts = append(response.TopProducts, dto.SellerReportProductResponse{
			ProductID: product.ProductID, Name: product.Name, Requests: product.Requests,
		})
	}
	for _, client := range report.TopClients {
		response.TopClients = append(response.TopClients, dto.SellerReportClientResponse{
			ClientID: client.ClientID, Name: client.Name,
			AcceptedAmount: client.AcceptedAmount.StringFixed(2),
		})
	}
	return response
}

func decimalString(value *decimal.Decimal, places int32) *string {
	if value == nil {
		return nil
	}
	out := value.StringFixed(places)
	return &out
}
