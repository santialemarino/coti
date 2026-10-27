package dto

import "github.com/google/uuid"

// SellerReportResponse is the seller dashboard read for one period.
type SellerReportResponse struct {
	Range          SellerReportRangeResponse      `json:"range"`
	Scope          SellerReportScopeResponse      `json:"scope"`
	KPIs           SellerReportKPIsResponse       `json:"kpis"`
	Branches       []SellerReportBranchResponse   `json:"branches,omitempty"`
	QuotesByStatus []SellerReportStatusResponse   `json:"quotes_by_status"`
	Timeline       []SellerReportTimelineResponse `json:"timeline"`
	TopProducts    []SellerReportProductResponse  `json:"top_products"`
	TopClients     []SellerReportClientResponse   `json:"top_clients"`
	AIMatch        SellerReportAIMatchResponse    `json:"ai_match"`
}

// SellerReportRangeResponse is the inclusive period the report covers.
type SellerReportRangeResponse struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// SellerReportScopeResponse says whether the figures span every branch or the selected one.
type SellerReportScopeResponse struct {
	MultiBranch bool       `json:"multi_branch"`
	BranchID    *uuid.UUID `json:"branch_id"`
}

// SellerReportKPIsResponse holds the period's headline numbers.
type SellerReportKPIsResponse struct {
	RFQsReceived   int                              `json:"rfqs_received"`
	QuotesSent     int                              `json:"quotes_sent"`
	QuotesAccepted int                              `json:"quotes_accepted"`
	ConversionRate *string                          `json:"conversion_rate"`
	AcceptedAmount string                           `json:"accepted_amount"`
	ResponseTime   SellerReportResponseTimeResponse `json:"response_time"`
}

// SellerReportResponseTimeResponse measures the first send after an order arrived.
type SellerReportResponseTimeResponse struct {
	AverageSeconds *int64 `json:"average_seconds"`
	MedianSeconds  *int64 `json:"median_seconds"`
	Sample         int    `json:"sample"`
}

// SellerReportBranchResponse is one branch's order volume.
type SellerReportBranchResponse struct {
	BranchID   uuid.UUID `json:"branch_id"`
	BranchName string    `json:"branch_name"`
	RFQs       int       `json:"rfqs"`
}

// SellerReportStatusResponse is how many quotes currently sit in one status.
type SellerReportStatusResponse struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

// SellerReportTimelineResponse is one day's sent and accepted quote counts.
type SellerReportTimelineResponse struct {
	Date     string `json:"date"`
	Sent     int    `json:"sent"`
	Accepted int    `json:"accepted"`
}

// SellerReportProductResponse is one requested material ranked by quote lines.
type SellerReportProductResponse struct {
	ProductID *uuid.UUID `json:"product_id"`
	Name      string     `json:"name"`
	Requests  int        `json:"requests"`
}

// SellerReportClientResponse is one client ranked by accepted amount.
type SellerReportClientResponse struct {
	ClientID       *uuid.UUID `json:"client_id"`
	Name           string     `json:"name"`
	AcceptedAmount string     `json:"accepted_amount"`
}

// SellerReportAIMatchResponse is the auto-match outcome of the latest AI generation per order.
type SellerReportAIMatchResponse struct {
	Matched    int                            `json:"matched"`
	Total      int                            `json:"total"`
	Rate       *string                        `json:"rate"`
	Confidence SellerReportConfidenceResponse `json:"confidence"`
}

// SellerReportConfidenceResponse splits scored AI lines by how settled the match read.
type SellerReportConfidenceResponse struct {
	Low    int `json:"low"`
	Medium int `json:"medium"`
	High   int `json:"high"`
}
