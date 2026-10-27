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
	from   string
	to     string
	report *domain.SellerReport
	err    error
}

func (s *sellerReportServiceStub) Get(
	_ context.Context, _ domain.Tenant, from, to string,
) (*domain.SellerReport, error) {
	s.from = from
	s.to = to
	return s.report, s.err
}

func TestSellerReportHandlerGetPassesDatesAndFormatsDecimals(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rate := decimal.RequireFromString("0.3333")
	aiRate := decimal.RequireFromString("0.5")
	service := &sellerReportServiceStub{report: &domain.SellerReport{
		Range: domain.SellerReportRange{From: "2026-10-01", To: "2026-10-02"},
		KPIs: domain.SellerReportKPIs{
			ConversionRate: &rate,
			AcceptedAmount: decimal.RequireFromString("1250.5"),
		},
		AIMatch: domain.SellerReportAIMatch{Rate: &aiRate},
	}}
	handler := NewSellerReportHandler(service)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/reports?from=2026-10-01&to=2026-10-02", nil)
	middleware.SetTenant(c, domain.Tenant{AccountID: uuid.New()})

	handler.Get(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", recorder.Code)
	}
	if service.from != "2026-10-01" || service.to != "2026-10-02" {
		t.Fatalf("dates = %s..%s, want the query values", service.from, service.to)
	}
	var response dto.SellerReportResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
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
		`"top_clients":[]`,
	} {
		if !strings.Contains(string(payload), field) {
			t.Fatalf("json = %s, want %s", payload, field)
		}
	}
}
