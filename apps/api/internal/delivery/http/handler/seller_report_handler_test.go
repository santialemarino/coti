package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/middleware"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

type sellerReportServiceStub struct {
	from    string
	to      string
	filters domain.SellerReportFilterInput
	report  *domain.SellerReport
	content []byte
	err     error
}

func (s *sellerReportServiceStub) Get(
	_ context.Context, _ domain.Tenant, from, to string, filters domain.SellerReportFilterInput,
) (*domain.SellerReport, error) {
	s.from = from
	s.to = to
	s.filters = filters
	return s.report, s.err
}

func (s *sellerReportServiceStub) RenderPDF(
	_ context.Context, _ domain.Tenant, from, to string, filters domain.SellerReportFilterInput,
) ([]byte, error) {
	s.from = from
	s.to = to
	s.filters = filters
	return s.content, s.err
}

func TestSellerReportHandlerDownloadPDFStreamsTheDocument(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &sellerReportServiceStub{content: []byte("%PDF-1.4 report")}
	handler := NewSellerReportHandler(service)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet,
		"/v1/reports/pdf?from=2026-10-01&to=2026-10-02&status=SENT&channel=WHATSAPP", nil)
	middleware.SetTenant(c, domain.Tenant{AccountID: uuid.New()})

	handler.DownloadPDF(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if service.from != "2026-10-01" || service.to != "2026-10-02" {
		t.Fatalf("dates = %s..%s, want the query values", service.from, service.to)
	}
	if len(service.filters.Statuses) != 1 || service.filters.Statuses[0] != "SENT" ||
		service.filters.Channel != "WHATSAPP" {
		t.Fatalf("filters = %+v, want the PDF query values", service.filters)
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/pdf" {
		t.Fatalf("content type = %q, want application/pdf", got)
	}
	if got := recorder.Header().Get("Content-Disposition"); got != `attachment; filename="reporte-2026-10-01-2026-10-02.pdf"` {
		t.Fatalf("content disposition = %q", got)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "private, no-store" {
		t.Fatalf("cache control = %q", got)
	}
	if recorder.Body.String() != "%PDF-1.4 report" {
		t.Fatalf("body = %q", recorder.Body.String())
	}
}

func TestSellerReportHandlerDownloadPDFUsesTheDefaultFilename(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &sellerReportServiceStub{content: []byte("%PDF-1.4 report")}
	handler := NewSellerReportHandler(service)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/reports/pdf", nil)
	middleware.SetTenant(c, domain.Tenant{AccountID: uuid.New()})

	handler.DownloadPDF(c)

	if got := recorder.Header().Get("Content-Disposition"); got != `attachment; filename="reporte.pdf"` {
		t.Fatalf("content disposition = %q", got)
	}
}

func TestSellerReportHandlerGetPassesDatesAndFormatsDecimals(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rate := decimal.RequireFromString("0.3333")
	aiRate := decimal.RequireFromString("0.5")
	sellerID, clientID := uuid.New(), uuid.New()
	channel := domain.ChannelTypeEmail
	service := &sellerReportServiceStub{report: &domain.SellerReport{
		Range: domain.SellerReportRange{From: "2026-10-01", To: "2026-10-02"},
		Filters: domain.SellerReportFilters{
			Statuses:   []domain.QuoteStatus{domain.QuoteStatusSent, domain.QuoteStatusAccepted},
			Channel:    &channel,
			SellerID:   &sellerID,
			SellerName: "Ana",
			ClientID:   &clientID,
			ClientName: "Obra Norte",
		},
		KPIs: domain.SellerReportKPIs{
			ConversionRate: &rate,
			AcceptedAmount: decimal.RequireFromString("1250.5"),
		},
		TopRevenueProducts: []domain.SellerReportRevenueProduct{{
			ProductID: uuid.New(),
			Name:      "Cemento",
			Revenue:   decimal.RequireFromString("1250.5"),
		}},
		AIMatch: domain.SellerReportAIMatch{Rate: &aiRate},
	}}
	handler := NewSellerReportHandler(service)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet,
		"/v1/reports?from=2026-10-01&to=2026-10-02&status=SENT&status=ACCEPTED"+
			"&channel=EMAIL&seller_id=seller-1&client_id=client-1", nil)
	middleware.SetTenant(c, domain.Tenant{AccountID: uuid.New()})

	handler.Get(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if service.from != "2026-10-01" || service.to != "2026-10-02" {
		t.Fatalf("dates = %s..%s, want the query values", service.from, service.to)
	}
	if len(service.filters.Statuses) != 2 || service.filters.Statuses[0] != "SENT" ||
		service.filters.Statuses[1] != "ACCEPTED" || service.filters.Channel != "EMAIL" ||
		service.filters.SellerID != "seller-1" || service.filters.ClientID != "client-1" {
		t.Fatalf("filters = %+v, want every query value", service.filters)
	}
	var response dto.SellerReportResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Filters.Statuses) != 2 || response.Filters.Statuses[0] != "SENT" ||
		response.Filters.Statuses[1] != "ACCEPTED" {
		t.Fatalf("filter statuses = %v, want the applied statuses", response.Filters.Statuses)
	}
	if response.Filters.Channel == nil || *response.Filters.Channel != "EMAIL" ||
		response.Filters.SellerID == nil || *response.Filters.SellerID != sellerID ||
		response.Filters.SellerName != "Ana" ||
		response.Filters.ClientID == nil || *response.Filters.ClientID != clientID ||
		response.Filters.ClientName != "Obra Norte" {
		t.Fatalf("filter response = %+v, want every applied filter", response.Filters)
	}
	if response.KPIs.ConversionRate == nil || *response.KPIs.ConversionRate != "0.3333" {
		t.Fatalf("conversion rate = %v, want 0.3333", response.KPIs.ConversionRate)
	}
	if response.KPIs.AcceptedAmount != "1250.50" {
		t.Fatalf("accepted amount = %s, want 1250.50", response.KPIs.AcceptedAmount)
	}
	if response.AIMatch.Rate == nil || *response.AIMatch.Rate != "0.5000" {
		t.Fatalf("AI rate = %v, want 0.5000", response.AIMatch.Rate)
	}
	if len(response.TopRevenueProducts) != 1 ||
		response.TopRevenueProducts[0].Revenue != "1250.50" {
		t.Fatalf("revenue products = %+v, want 1250.50", response.TopRevenueProducts)
	}
}

func TestSellerReportResponseKeepsEmptyCollectionsAsArrays(t *testing.T) {
	payload, err := json.Marshal(toSellerReportResponse(domain.SellerReport{}))
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{
		`"quotes_by_status":[]`,
		`"timeline":[]`,
		`"top_products":[]`,
		`"top_revenue_products":[]`,
		`"top_clients":[]`,
	} {
		if !strings.Contains(string(payload), field) {
			t.Fatalf("json = %s, want %s", payload, field)
		}
	}

	var response dto.SellerReportResponse
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatal(err)
	}
	if response.Filters.Statuses == nil {
		t.Fatal("empty report filters.statuses = nil, want an empty array")
	}
	if len(response.Filters.Statuses) != 0 || response.Filters.Channel != nil ||
		response.Filters.SellerID != nil || response.Filters.SellerName != "" ||
		response.Filters.ClientID != nil || response.Filters.ClientName != "" {
		t.Fatalf("empty report filters = %+v, want an unfiltered object", response.Filters)
	}
}
