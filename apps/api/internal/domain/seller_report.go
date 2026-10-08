package domain

import "time"

// SellerReportFilter selects the RFQ receipt dates included in a seller report.
type SellerReportFilter struct {
	DateFrom *time.Time
	DateTo   *time.Time
}

// SellerReport contains activity and rankings for one seller.
type SellerReport struct {
	OrdersReceived          int64
	QuotesSent              int64
	QuotesAccepted          int64
	AverageQuoteTimeSeconds *int64
	Statuses                []SellerReportStatus
	TopMaterials            []SellerReportMaterial
	TopClients              []SellerReportClient
}

// SellerReportStatus is a quote status count in a seller report.
type SellerReportStatus struct {
	Status string
	Count  int64
}

// SellerReportMaterial is a frequently requested product in a seller report.
type SellerReportMaterial struct {
	Name       string
	OrderCount int64
}

// SellerReportClient is an active client in a seller report.
type SellerReportClient struct {
	Name       string
	OrderCount int64
}
