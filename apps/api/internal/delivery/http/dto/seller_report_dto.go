package dto

// SellerReportQuery is the optional date filter for GET /v1/reports.
type SellerReportQuery struct {
	DateFrom string `form:"date_from" binding:"omitempty,datetime=2006-01-02"`
	DateTo   string `form:"date_to" binding:"omitempty,datetime=2006-01-02"`
}

// SellerReportResponse is returned by GET /v1/reports.
type SellerReportResponse struct {
	OrdersReceived          int64                          `json:"orders_received"`
	QuotesSent              int64                          `json:"quotes_sent"`
	QuotesAccepted          int64                          `json:"quotes_accepted"`
	AverageQuoteTimeSeconds *int64                         `json:"average_quote_time_seconds"`
	Statuses                []SellerReportStatusResponse   `json:"statuses"`
	TopMaterials            []SellerReportMaterialResponse `json:"top_materials"`
	TopClients              []SellerReportClientResponse   `json:"top_clients"`
}

// SellerReportStatusResponse is one current quote status count.
type SellerReportStatusResponse struct {
	Status string `json:"status"`
	Count  int64  `json:"count"`
}

// SellerReportMaterialResponse is one frequently ordered catalog product.
type SellerReportMaterialResponse struct {
	Name       string `json:"name"`
	OrderCount int64  `json:"order_count"`
}

// SellerReportClientResponse is one client ranked by received order count.
type SellerReportClientResponse struct {
	Name       string `json:"name"`
	OrderCount int64  `json:"order_count"`
}
