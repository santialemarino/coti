package domain

import (
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// ReportGranularity is the bucket size of a report's trend series.
type ReportGranularity string

const (
	ReportGranularityDay   ReportGranularity = "day"
	ReportGranularityWeek  ReportGranularity = "week"
	ReportGranularityMonth ReportGranularity = "month"
)

// IsValidReportGranularity reports whether granularity is one of the trend bucket sizes.
func IsValidReportGranularity(granularity ReportGranularity) bool {
	switch granularity {
	case ReportGranularityDay, ReportGranularityWeek, ReportGranularityMonth:
		return true
	}
	return false
}

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
	Granularity       ReportGranularity
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
	// Granularity is the requested trend bucket; empty picks one from the period's length.
	Granularity string
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
	Granularity        ReportGranularity
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
	Assistant          SellerReportAssistant
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
	// ResponseTime is the first send after an order arrived, manual entries excluded.
	ResponseTime SellerReportResponseTime
	// TimeToAcceptance runs from a quote's first send to its acceptance, for acceptances in the period.
	TimeToAcceptance SellerReportResponseTime
	// FollowUpPending counts the quotes in scope flagged for follow-up now, whatever the period.
	FollowUpPending int
}

// SellerReportResponseTime summarizes one measured interval: its mean, its median and the sample size.
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

// SellerReportTimelinePoint is one trend bucket: a day, a week from Monday or a calendar month.
type SellerReportTimelinePoint struct {
	// Date is the bucket's first day inside the period, so a partial first bucket starts at the period.
	Date     string
	RFQs     int
	Sent     int
	Accepted int
	// SentAccepted counts the bucket's first sends that were accepted since.
	SentAccepted int
	// ConversionRate is SentAccepted over Sent; nil when the bucket sent nothing.
	ConversionRate *decimal.Decimal
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
	// VersionLines and VersionUnmatched read the current versions, after the seller's review.
	VersionLines     int
	VersionUnmatched int
}

// SellerReportAssistant splits the conversation assistant's proposals by what the seller did.
type SellerReportAssistant struct {
	Proposals      int
	ApprovedAsIs   int
	Edited         int
	Rejected       int
	ManualOverride int
	// Pending proposals are undecided and stay in the denominator: an ignored proposal is data.
	Pending int
	// ApprovalRate is ApprovedAsIs over Proposals; nil when the assistant proposed nothing.
	ApprovalRate *decimal.Decimal
}

// SellerReportConfidence splits scored AI lines by how settled the catalog match read.
type SellerReportConfidence struct {
	Low    int
	Medium int
	High   int
}
