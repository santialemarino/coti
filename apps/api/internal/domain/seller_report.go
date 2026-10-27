package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// SellerReportQuery is the resolved scope and period one report read runs with.
type SellerReportQuery struct {
	AccountID      uuid.UUID
	BranchFilter   []uuid.UUID
	SellerID       *uuid.UUID
	StartAt        time.Time
	EndAt          time.Time
	FromDate       string
	ToDate         string
	TimeZone       string
	HighConfidence decimal.Decimal
}

// SellerReport is the seller dashboard's read model.
type SellerReport struct {
	Range          SellerReportRange
	Scope          SellerReportScope
	KPIs           SellerReportKPIs
	Branches       []SellerReportBranch
	QuotesByStatus []SellerReportStatusCount
	Timeline       []SellerReportTimelinePoint
	TopProducts    []SellerReportProduct
	TopClients     []SellerReportClient
	AIMatch        SellerReportAIMatch
}

// SellerReportRange is the inclusive Argentine calendar-day period the report covers.
type SellerReportRange struct {
	From string
	To   string
}

// SellerReportScope says whether the report covers every reachable branch or one selected branch.
type SellerReportScope struct {
	MultiBranch bool
	BranchID    *uuid.UUID
}

// SellerReportKPIs are the headline numbers for the period.
type SellerReportKPIs struct {
	RFQsReceived   int
	QuotesSent     int
	QuotesAccepted int
	// ConversionRate is accepted over sent events in the period, so it can exceed one; nil when
	// nothing was sent.
	ConversionRate *decimal.Decimal
	// AcceptedAmount sums the commercial totals of the accepted versions found for the period.
	AcceptedAmount decimal.Decimal
	ResponseTime   SellerReportResponseTime
}

// SellerReportResponseTime measures the first send after an order arrived, manual entries excluded.
type SellerReportResponseTime struct {
	AverageSeconds *int64
	MedianSeconds  *int64
	Sample         int
}

// SellerReportBranch is one branch's order volume when the report spans every branch.
type SellerReportBranch struct {
	BranchID   uuid.UUID
	BranchName string
	RFQs       int
}

// SellerReportStatusCount is how many quotes currently sit in one status.
type SellerReportStatusCount struct {
	Status QuoteStatus
	Count  int
}

// SellerReportTimelinePoint is one Argentine calendar day's sent and accepted quote counts.
type SellerReportTimelinePoint struct {
	Date     string
	Sent     int
	Accepted int
}

// SellerReportProduct is one requested material, ranked by how many quote lines asked for it.
type SellerReportProduct struct {
	ProductID *uuid.UUID
	Name      string
	Requests  int
}

// SellerReportClient is one client, ranked by the commercial amount accepted in the period.
type SellerReportClient struct {
	ClientID       *uuid.UUID
	Name           string
	AcceptedAmount decimal.Decimal
}

// SellerReportAIMatch is the auto-match outcome of the latest AI generation per order.
type SellerReportAIMatch struct {
	Matched    int
	Total      int
	Rate       *decimal.Decimal
	Confidence SellerReportConfidence
}

// SellerReportConfidence splits scored AI lines by how settled the catalog match read.
type SellerReportConfidence struct {
	Low    int
	Medium int
	High   int
}
