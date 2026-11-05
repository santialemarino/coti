package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// SellerReportQuery is the resolved scope, period and filters one report read runs with.
type SellerReportQuery struct {
	AccountID    uuid.UUID
	BranchFilter []uuid.UUID
	// MultiBranch asks for the per-branch breakdown: no branch is selected and the reach spans several.
	MultiBranch bool
	// RequestedBranches are the branch filter's choices inside the caller's reach, echoed by name.
	RequestedBranches []uuid.UUID
	SellerID          *uuid.UUID
	SellerFilter      *uuid.UUID
	ClientFilter      *uuid.UUID
	TagFilter         []uuid.UUID
	Statuses          []QuoteStatus
	Channel           *ChannelType
	StartAt           time.Time
	EndAt             time.Time
	FromDate          string
	ToDate            string
	TimeZone          string
	HighConfidence    decimal.Decimal
}

// SellerReportFilterInput carries the raw filter values of one report request.
type SellerReportFilterInput struct {
	Statuses  []string
	Channel   string
	SellerID  string
	ClientID  string
	BranchIDs []string
	TagIDs    []string
}

// SellerReportFilters is the applied narrowing, with the display names the response echoes.
type SellerReportFilters struct {
	Statuses   []QuoteStatus
	Channel    *ChannelType
	SellerID   *uuid.UUID
	SellerName string
	ClientID   *uuid.UUID
	ClientName string
	Branches   []SellerReportFilterOption
	Tags       []SellerReportFilterOption
}

// SellerReportFilterOption is one selected branch or tag, with the name the response echoes.
type SellerReportFilterOption struct {
	ID   uuid.UUID
	Name string
}

// SellerReportPDFRenderer renders the seller dashboard snapshot as a PDF.
type SellerReportPDFRenderer interface {
	Render(report SellerReport, generatedAt time.Time) ([]byte, error)
}

// SellerReport is the seller dashboard's read model.
type SellerReport struct {
	Range              SellerReportRange
	Scope              SellerReportScope
	Filters            SellerReportFilters
	KPIs               SellerReportKPIs
	Branches           []SellerReportBranch
	QuotesByStatus     []SellerReportStatusCount
	Timeline           []SellerReportTimelinePoint
	TopProducts        []SellerReportProduct
	TopRevenueProducts []SellerReportRevenueProduct
	TopClients         []SellerReportClient
	AIMatch            SellerReportAIMatch
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
	// SentAccepted counts the quotes first sent in the period that were accepted since.
	SentAccepted int
	// ConversionRate is SentAccepted over QuotesSent, a cohort share in [0, 1]; nil when nothing was sent.
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

// SellerReportRevenueProduct is one matched product, ranked by its accepted line revenue.
type SellerReportRevenueProduct struct {
	ProductID uuid.UUID
	Name      string
	Revenue   decimal.Decimal
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
