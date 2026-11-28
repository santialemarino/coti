package pdf

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func sellerReportFixture() domain.SellerReport {
	rate := decimal.RequireFromString("0.6250")
	accepted := decimal.RequireFromString("178800.00")
	currency := "ARS"
	branchID := uuid.New()
	return domain.SellerReport{
		Range: domain.SellerReportRange{From: "2026-09-26", To: "2026-10-25"},
		Scope: domain.SellerReportScope{MultiBranch: true},
		KPIs: domain.SellerReportKPIs{
			RFQsReceived:     128,
			QuotesSent:       64,
			QuotesAccepted:   16,
			ConversionRate:   &rate,
			AcceptedAmount:   &accepted,
			AcceptedCurrency: &currency,
			AcceptedAmounts:  []domain.SellerReportAmount{{Currency: currency, Amount: accepted}},
			ResponseTime: domain.SellerReportResponseTime{
				AverageSeconds: int64Pointer(934),
				MedianSeconds:  int64Pointer(812),
				Sample:         42,
			},
		},
		Branches: []domain.SellerReportBranch{
			{BranchID: branchID, BranchName: "Casa central", RFQs: 82},
			{BranchID: uuid.New(), BranchName: "Sucursal Norte", RFQs: 46},
		},
		QuotesByStatus: []domain.SellerReportStatusCount{
			{Status: domain.QuoteStatusDraft, Count: 3},
			{Status: domain.QuoteStatusQuoted, Count: 8},
			{Status: domain.QuoteStatusSent, Count: 31},
			{Status: domain.QuoteStatusChangeRequested, Count: 5},
			{Status: domain.QuoteStatusAccepted, Count: 16},
			{Status: domain.QuoteStatusRejected, Count: 4},
		},
		Timeline: []domain.SellerReportTimelinePoint{
			{Date: "2026-09-26", Sent: 1, Accepted: 0},
			{Date: "2026-10-01", Sent: 3, Accepted: 1},
			{Date: "2026-10-08", Sent: 5, Accepted: 2},
			{Date: "2026-10-15", Sent: 2, Accepted: 1},
			{Date: "2026-10-25", Sent: 4, Accepted: 2},
		},
		TopProducts: []domain.SellerReportProduct{
			{Name: "Cemento portland 50 kg", Requests: 32},
			{Name: "Arena gruesa", Requests: 24},
			{Name: "Ladrillo hueco 12x18x33", Requests: 19},
		},
		TopRevenueProducts: []domain.SellerReportRevenueProduct{
			{ProductID: uuid.New(), Name: "Cemento portland 50 kg", Currency: currency, Revenue: decimal.RequireFromString("98000.00")},
			{ProductID: uuid.New(), Name: "Hierro del 8", Currency: currency, Revenue: decimal.RequireFromString("54800.00")},
			{ProductID: uuid.New(), Name: "Arena gruesa", Currency: currency, Revenue: decimal.RequireFromString("26000.00")},
		},
		TopClients: []domain.SellerReportClient{
			{Name: "Obra Norte", Currency: currency, AcceptedAmount: decimal.RequireFromString("84000.00")},
			{Name: "Constructora del Sur", Currency: currency, AcceptedAmount: decimal.RequireFromString("54800.00")},
			{Name: "Viviendas Rivadavia", Currency: currency, AcceptedAmount: decimal.RequireFromString("40000.00")},
		},
		AIMatch: domain.SellerReportAIMatch{
			Matched: 40,
			Total:   64,
			Rate:    &rate,
			Confidence: domain.SellerReportConfidence{
				Low: 6, Medium: 14, High: 20,
			},
		},
	}
}

func TestSellerReportRenderer_ProducesAPrintablePDF(t *testing.T) {
	generatedAt := time.Date(2026, time.October, 31, 14, 30, 0, 0, domain.Argentina)
	content, err := NewSellerReportRenderer().Render(sellerReportFixture(), generatedAt)
	if err != nil {
		t.Fatalf("Render() error = %v", err)
	}
	if !bytes.HasPrefix(content, []byte("%PDF-")) {
		t.Fatalf("PDF prefix = %q", content[:min(len(content), 8)])
	}
	if len(content) < 3000 {
		t.Fatalf("PDF size = %d, want an embedded-font document", len(content))
	}
	if pages := bytes.Count(content, []byte("/Type /Page\n")); pages < 2 {
		t.Fatalf("pages = %d, want the report sections across pages", pages)
	}
	if directory := os.Getenv("TEST_REPORT_PDF_OUTPUT_DIR"); directory != "" {
		if err := os.WriteFile(filepath.Join(directory, "seller-report.pdf"), content, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSellerReportRenderer_RendersAnEmptyPeriod(t *testing.T) {
	report := domain.SellerReport{
		Range: domain.SellerReportRange{From: "2026-10-01", To: "2026-10-02"},
	}
	content, err := NewSellerReportRenderer().Render(report, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(content, []byte("%PDF-")) {
		t.Fatal("empty period did not render")
	}
}

func TestSellerReportRenderer_SeparatesCurrencySections(t *testing.T) {
	report := sellerReportFixture()
	report.KPIs.AcceptedAmount, report.KPIs.AcceptedCurrency = nil, nil
	report.KPIs.AcceptedAmounts = []domain.SellerReportAmount{
		{Currency: "ARS", Amount: decimal.RequireFromString("100.00")},
		{Currency: "USD", Amount: decimal.RequireFromString("10.00")},
	}
	report.TopRevenueProducts = []domain.SellerReportRevenueProduct{
		{ProductID: uuid.New(), Name: "Same product", Currency: "ARS", Revenue: decimal.RequireFromString("100.00")},
		{ProductID: uuid.New(), Name: "Same product", Currency: "USD", Revenue: decimal.RequireFromString("10.00")},
	}
	report.TopClients = []domain.SellerReportClient{
		{Name: "Same client", Currency: "ARS", AcceptedAmount: decimal.RequireFromString("100.00")},
		{Name: "Same client", Currency: "USD", AcceptedAmount: decimal.RequireFromString("10.00")},
	}
	if value := reportAcceptedMoney(report.KPIs); value != "ARS 100,00\nUSD 10,00" {
		t.Fatalf("mixed headline = %q, want a currency breakdown", value)
	}
	content, err := NewSellerReportRenderer().Render(report, time.Date(2026, 11, 27, 12, 0, 0, 0, domain.Argentina))
	if err != nil {
		t.Fatal(err)
	}
	if pages := bytes.Count(content, []byte("/Type /Page\n")); pages < 4 {
		t.Fatalf("currency PDF pages = %d, want separate sections for both currencies", pages)
	}
	if directory := os.Getenv("TEST_REPORT_PDF_OUTPUT_DIR"); directory != "" {
		if err := os.WriteFile(filepath.Join(directory, "seller-report-currencies.pdf"), content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	usd, currency := decimal.RequireFromString("10.00"), "USD"
	report.KPIs.AcceptedAmount, report.KPIs.AcceptedCurrency = &usd, &currency
	if value := reportAcceptedMoney(report.KPIs); value != "USD 10,00" {
		t.Fatalf("single-currency headline = %q, want USD 10,00", value)
	}
	large := sellerReportFixture()
	usd = decimal.RequireFromString("999999999999.99")
	large.KPIs.AcceptedAmount, large.KPIs.AcceptedCurrency = &usd, &currency
	large.KPIs.AcceptedAmounts = []domain.SellerReportAmount{{Currency: currency, Amount: usd}}
	for index := range large.TopRevenueProducts {
		large.TopRevenueProducts[index].Currency = currency
	}
	large.TopRevenueProducts[0].Revenue = usd
	for index := range large.TopClients {
		large.TopClients[index].Currency = currency
	}
	large.TopClients[0].AcceptedAmount = usd
	content, err = NewSellerReportRenderer().Render(large, time.Date(2026, 11, 27, 12, 0, 0, 0, domain.Argentina))
	if err != nil {
		t.Fatal(err)
	}
	if directory := os.Getenv("TEST_REPORT_PDF_OUTPUT_DIR"); directory != "" {
		if err := os.WriteFile(filepath.Join(directory, "seller-report-usd.pdf"), content, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSellerReportRenderer_PaginatesManyBranches(t *testing.T) {
	pagesWith := func(branches int) int {
		t.Helper()
		report := sellerReportFixture()
		report.Branches = nil
		for index := 0; index < branches; index++ {
			report.Branches = append(report.Branches, domain.SellerReportBranch{
				BranchID:   uuid.New(),
				BranchName: "Sucursal con un nombre extenso para probar el corte de pagina",
				RFQs:       index,
			})
		}
		content, err := NewSellerReportRenderer().Render(report, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		return bytes.Count(content, []byte("/Type /Page\n"))
	}
	// Forty 8 mm rows overflow a page on their own.
	if one, many := pagesWith(1), pagesWith(40); many <= one {
		t.Fatalf("pages = %d with one branch and %d with forty, want the rows paginated", one, many)
	}
}

func int64Pointer(value int64) *int64 {
	return &value
}

func TestReportDurationReadsLongWaitsInDays(t *testing.T) {
	for seconds, want := range map[int64]string{
		45:                       "45 s",
		90 * 60:                  "1 h 30 min",
		47 * 3600:                "47 h",
		48 * 3600:                "2 d",
		(534*60 + 44) * 60:       "22 d 6 h",
		(3*24*3600 + 30*60) + 10: "3 d",
	} {
		value := seconds
		if got := reportDuration(&value); got != want {
			t.Errorf("reportDuration(%d) = %q, want %q", seconds, got, want)
		}
	}
}
