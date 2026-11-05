package handler

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// SellerReportService is the reporting surface the handler needs.
type SellerReportService interface {
	Get(context.Context, domain.Tenant, string, string,
		domain.SellerReportFilterInput) (*domain.SellerReport, error)
	RenderPDF(context.Context, domain.Tenant, string, string,
		domain.SellerReportFilterInput) ([]byte, domain.SellerReportRange, error)
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
//	@Param			status		query	[]string	false	"Quote statuses to include; repeat or comma-separate"
//	@Param			channel		query	string	false	"Intake channel type"
//	@Param			seller_id	query	string	false	"Seller UUID; administrators only"
//	@Param			client_id	query	string	false	"Client UUID"
//	@Param			branch_id	query	[]string	false	"Branch UUIDs inside the caller's reach; repeat or comma-separate"
//	@Param			tag_id		query	[]string	false	"Client tag UUIDs; an order matches when its client carries any of them"
//	@Param			granularity	query	string	false	"Trend bucket: day, week or month; omitted picks one from the period's length"
//	@Success		200			{object}	dto.SellerReportResponse
//	@Failure		422			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Router			/v1/reports [get]
func (h *SellerReportHandler) Get(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	query, ok := reportFilterQuery(c)
	if !ok {
		return
	}
	report, err := h.reports.Get(c.Request.Context(), tenant, query.From, query.To,
		reportFilterInput(query))
	if err != nil {
		Respond(c, err)
		return
	}
	c.JSON(http.StatusOK, toSellerReportResponse(*report))
}

// DownloadPDF returns the report for the requested period as an A4 PDF.
//
//	@Summary		Download seller report PDF
//	@Description	Renders the same scoped seller dashboard metrics as a printable PDF.
//	@Tags			reports
//	@Produce		application/pdf
//	@Security		BearerAuth
//	@Param			X-Branch-Id	header	string	false	"Active branch; omitted reports on every branch the caller reaches"
//	@Param			from		query	string	false	"First day, YYYY-MM-DD; defaults to 30 days ago"
//	@Param			to			query	string	false	"Last day, YYYY-MM-DD; defaults to today"
//	@Param			status		query	[]string	false	"Quote statuses to include; repeat or comma-separate"
//	@Param			channel		query	string	false	"Intake channel type"
//	@Param			seller_id	query	string	false	"Seller UUID; administrators only"
//	@Param			client_id	query	string	false	"Client UUID"
//	@Param			branch_id	query	[]string	false	"Branch UUIDs inside the caller's reach; repeat or comma-separate"
//	@Param			tag_id		query	[]string	false	"Client tag UUIDs; an order matches when its client carries any of them"
//	@Param			granularity	query	string	false	"Trend bucket: day, week or month; omitted picks one from the period's length"
//	@Success		200			{file}	binary
//	@Failure		422			{object}	dto.ErrorResponse
//	@Failure		401			{object}	dto.ErrorResponse
//	@Failure		403			{object}	dto.ErrorResponse
//	@Failure		500			{object}	dto.ErrorResponse
//	@Router			/v1/reports/pdf [get]
func (h *SellerReportHandler) DownloadPDF(c *gin.Context) {
	tenant, ok := tenantOf(c)
	if !ok {
		return
	}
	query, ok := reportFilterQuery(c)
	if !ok {
		return
	}
	content, period, err := h.reports.RenderPDF(c.Request.Context(), tenant, query.From, query.To,
		reportFilterInput(query))
	if err != nil {
		Respond(c, err)
		return
	}
	filename := fmt.Sprintf("reporte-%s-%s.pdf", period.From, period.To)
	c.Header("Cache-Control", "private, no-store")
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Data(http.StatusOK, "application/pdf", content)
}

func reportFilterQuery(c *gin.Context) (dto.SellerReportFilterQuery, bool) {
	var query dto.SellerReportFilterQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		Respond(c, fmt.Errorf("%w: invalid report filters", domain.ErrInvalidInput))
		return query, false
	}
	return query, true
}

func reportFilterInput(query dto.SellerReportFilterQuery) domain.SellerReportFilterInput {
	return domain.SellerReportFilterInput{
		Statuses:    query.Statuses,
		Channel:     query.Channel,
		SellerID:    query.SellerID,
		ClientID:    query.ClientID,
		BranchIDs:   query.BranchIDs,
		TagIDs:      query.TagIDs,
		Granularity: query.Granularity,
	}
}

func toSellerReportResponse(report domain.SellerReport) dto.SellerReportResponse {
	filters := dto.SellerReportFiltersResponse{
		Statuses:   make([]string, 0, len(report.Filters.Statuses)),
		SellerName: report.Filters.SellerName,
		ClientName: report.Filters.ClientName,
		SellerID:   report.Filters.SellerID,
		ClientID:   report.Filters.ClientID,
	}
	for _, status := range report.Filters.Statuses {
		filters.Statuses = append(filters.Statuses, string(status))
	}
	filters.Branches = reportOptions(report.Filters.Branches)
	filters.Tags = reportOptions(report.Filters.Tags)
	if report.Filters.Channel != nil {
		channel := string(*report.Filters.Channel)
		filters.Channel = &channel
	}
	response := dto.SellerReportResponse{
		Range: dto.SellerReportRangeResponse{
			From: report.Range.From,
			To:   report.Range.To,
		},
		Granularity: string(report.Granularity),
		Scope: dto.SellerReportScopeResponse{
			MultiBranch: report.Scope.MultiBranch,
			BranchID:    report.Scope.BranchID,
		},
		Filters: filters,
		KPIs: dto.SellerReportKPIsResponse{
			RFQsReceived:   report.KPIs.RFQsReceived,
			QuotesSent:     report.KPIs.QuotesSent,
			QuotesAccepted: report.KPIs.QuotesAccepted,
			SentAccepted:   report.KPIs.SentAccepted,
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
		TopRevenueProducts: make([]dto.SellerReportRevenueProductResponse, 0,
			len(report.TopRevenueProducts)),
		TopClients: make([]dto.SellerReportClientResponse, 0, len(report.TopClients)),
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
			Date: point.Date, RFQs: point.RFQs, Sent: point.Sent, Accepted: point.Accepted,
			SentAccepted:   point.SentAccepted,
			ConversionRate: decimalString(point.ConversionRate, 4),
		})
	}
	for _, product := range report.TopProducts {
		response.TopProducts = append(response.TopProducts, dto.SellerReportProductResponse{
			ProductID: product.ProductID, Name: product.Name, Requests: product.Requests,
		})
	}
	for _, product := range report.TopRevenueProducts {
		response.TopRevenueProducts = append(response.TopRevenueProducts,
			dto.SellerReportRevenueProductResponse{
				ProductID: product.ProductID,
				Name:      product.Name,
				Revenue:   product.Revenue.StringFixed(2),
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

func reportOptions(options []domain.SellerReportFilterOption) []dto.SellerReportOptionResponse {
	out := make([]dto.SellerReportOptionResponse, 0, len(options))
	for _, option := range options {
		out = append(out, dto.SellerReportOptionResponse{ID: option.ID, Name: option.Name})
	}
	return out
}
