package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

const (
	reportDefaultDays = 30
	reportDateLayout  = "2006-01-02"
)

type sellerReportStore interface {
	Build(context.Context, repository.Querier, domain.SellerReportQuery) (*domain.SellerReport, error)
}

// SellerReportSettings carries the bounds and thresholds a report query reads.
type SellerReportSettings struct {
	MaxRangeDays   int
	HighConfidence decimal.Decimal
}

// SellerReportService turns a requested period into a scoped seller dashboard read.
type SellerReportService struct {
	db       tenantTxRunner
	reports  sellerReportStore
	renderer domain.SellerReportPDFRenderer
	now      func() time.Time
	cfg      SellerReportSettings
}

// NewSellerReportService builds a SellerReportService.
func NewSellerReportService(db tenantTxRunner, reports sellerReportStore,
	renderer domain.SellerReportPDFRenderer, cfg SellerReportSettings,
) *SellerReportService {
	return &SellerReportService{
		db: db, reports: reports, renderer: renderer, now: time.Now, cfg: cfg,
	}
}

// Get returns the report for the requested inclusive dates, defaulting to the last 30 days.
func (s *SellerReportService) Get(ctx context.Context, tenant domain.Tenant, fromRaw,
	toRaw string, filterInput domain.SellerReportFilterInput,
) (*domain.SellerReport, error) {
	from, to, err := s.resolveRange(fromRaw, toRaw)
	if err != nil {
		return nil, err
	}
	filters, err := resolveReportFilters(tenant, filterInput)
	if err != nil {
		return nil, err
	}
	start := from.UTC()
	end := to.AddDate(0, 0, 1).UTC()
	sellerID := sellerParam(tenant)
	branchFilter := narrowBranches(tenant.BranchFilter(), filters.Branches)
	multiBranch := !tenant.HasBranch() && (branchFilter == nil || len(branchFilter) > 1)
	var requestedBranches []uuid.UUID
	if len(filters.Branches) > 0 {
		requestedBranches = branchFilter
	}
	query := domain.SellerReportQuery{
		AccountID:         tenant.AccountID,
		BranchFilter:      branchFilter,
		MultiBranch:       multiBranch,
		RequestedBranches: requestedBranches,
		SellerID:          sellerID,
		SellerFilter:      filters.SellerID,
		ClientFilter:      filters.ClientID,
		TagFilter:         optionIDs(filters.Tags),
		Statuses:          filters.Statuses,
		Channel:           filters.Channel,
		StartAt:           start,
		EndAt:             end,
		FromDate:          from.Format(reportDateLayout),
		ToDate:            to.Format(reportDateLayout),
		TimeZone:          domain.Argentina.String(),
		HighConfidence:    s.cfg.HighConfidence,
	}

	var report *domain.SellerReport
	err = s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var readErr error
		report, readErr = s.reports.Build(ctx, q, query)
		return readErr
	})
	if err != nil {
		return nil, err
	}
	report.Range = domain.SellerReportRange{
		From: query.FromDate,
		To:   query.ToDate,
	}
	report.Scope = domain.SellerReportScope{
		MultiBranch: multiBranch,
	}
	if tenant.HasBranch() {
		branchID := tenant.BranchID
		report.Scope.BranchID = &branchID
	}
	report.Filters = domain.SellerReportFilters{
		Statuses:   filters.Statuses,
		Channel:    filters.Channel,
		SellerID:   filters.SellerID,
		SellerName: report.Filters.SellerName,
		ClientID:   filters.ClientID,
		ClientName: report.Filters.ClientName,
		Branches:   report.Filters.Branches,
		Tags:       report.Filters.Tags,
	}
	return report, nil
}

// RenderPDF returns the report as a PDF document, with the period it resolved to.
func (s *SellerReportService) RenderPDF(ctx context.Context, tenant domain.Tenant, fromRaw,
	toRaw string, filterInput domain.SellerReportFilterInput,
) ([]byte, domain.SellerReportRange, error) {
	report, err := s.Get(ctx, tenant, fromRaw, toRaw, filterInput)
	if err != nil {
		return nil, domain.SellerReportRange{}, err
	}
	content, err := s.renderer.Render(*report, s.now().In(domain.Argentina))
	if err != nil {
		return nil, domain.SellerReportRange{}, err
	}
	return content, report.Range, nil
}

func resolveReportFilters(tenant domain.Tenant, in domain.SellerReportFilterInput) (
	domain.SellerReportFilters, error,
) {
	filters := domain.SellerReportFilters{Statuses: []domain.QuoteStatus{}}
	seen := make(map[domain.QuoteStatus]struct{})
	for _, raw := range in.Statuses {
		for _, part := range strings.Split(raw, ",") {
			trimmed := strings.TrimSpace(part)
			if trimmed == "" {
				continue
			}
			status := domain.QuoteStatus(strings.ToUpper(trimmed))
			if !domain.IsValidQuoteStatus(status) {
				return domain.SellerReportFilters{},
					fmt.Errorf("%w: unknown status %q", domain.ErrInvalidInput, part)
			}
			if _, ok := seen[status]; ok {
				continue
			}
			seen[status] = struct{}{}
			filters.Statuses = append(filters.Statuses, status)
		}
	}

	if raw := strings.TrimSpace(in.Channel); raw != "" {
		channel := domain.ChannelType(strings.ToUpper(raw))
		if !domain.IsValidChannelType(channel) {
			return domain.SellerReportFilters{},
				fmt.Errorf("%w: unknown channel %q", domain.ErrInvalidInput, in.Channel)
		}
		filters.Channel = &channel
	}

	if tenant.IsAdmin() {
		sellerID, err := reportFilterID(in.SellerID, "seller_id")
		if err != nil {
			return domain.SellerReportFilters{}, err
		}
		filters.SellerID = sellerID
	}
	clientID, err := reportFilterID(in.ClientID, "client_id")
	if err != nil {
		return domain.SellerReportFilters{}, err
	}
	filters.ClientID = clientID

	branchIDs, err := reportFilterIDs(in.BranchIDs, "branch_id")
	if err != nil {
		return domain.SellerReportFilters{}, err
	}
	tagIDs, err := reportFilterIDs(in.TagIDs, "tag_id")
	if err != nil {
		return domain.SellerReportFilters{}, err
	}
	// An active branch already fixes the reach, so a branch filter left over from the all-branches view
	// is dropped rather than intersected into an empty report.
	if !tenant.HasBranch() {
		filters.Branches = filterOptions(branchIDs)
	}
	filters.Tags = filterOptions(tagIDs)
	return filters, nil
}

// narrowBranches keeps the requested branches the caller reaches; nil reach is an admin's whole account.
func narrowBranches(reach []uuid.UUID, requested []domain.SellerReportFilterOption) []uuid.UUID {
	if len(requested) == 0 {
		return reach
	}
	if reach == nil {
		return optionIDs(requested)
	}
	allowed := make(map[uuid.UUID]struct{}, len(reach))
	for _, id := range reach {
		allowed[id] = struct{}{}
	}
	narrowed := []uuid.UUID{}
	for _, option := range requested {
		if _, ok := allowed[option.ID]; ok {
			narrowed = append(narrowed, option.ID)
		}
	}
	return narrowed
}

func reportFilterIDs(raw []string, field string) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	seen := make(map[uuid.UUID]struct{})
	for _, value := range raw {
		for _, part := range strings.Split(value, ",") {
			id, err := reportFilterID(part, field)
			if err != nil {
				return nil, err
			}
			if id == nil {
				continue
			}
			if _, ok := seen[*id]; ok {
				continue
			}
			seen[*id] = struct{}{}
			ids = append(ids, *id)
		}
	}
	return ids, nil
}

func filterOptions(ids []uuid.UUID) []domain.SellerReportFilterOption {
	options := make([]domain.SellerReportFilterOption, 0, len(ids))
	for _, id := range ids {
		options = append(options, domain.SellerReportFilterOption{ID: id})
	}
	return options
}

func optionIDs(options []domain.SellerReportFilterOption) []uuid.UUID {
	if len(options) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(options))
	for _, option := range options {
		ids = append(ids, option.ID)
	}
	return ids
}

func reportFilterID(raw, field string) (*uuid.UUID, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	id, err := uuid.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("%w: %s must be a UUID", domain.ErrInvalidInput, field)
	}
	return &id, nil
}

func (s *SellerReportService) resolveRange(fromRaw, toRaw string) (time.Time, time.Time, error) {
	if (fromRaw == "") != (toRaw == "") {
		return time.Time{}, time.Time{},
			fmt.Errorf("%w: from and to must be provided together", domain.ErrInvalidInput)
	}
	if fromRaw == "" {
		today := s.now().In(domain.Argentina)
		to := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, domain.Argentina)
		return to.AddDate(0, 0, -(reportDefaultDays - 1)), to, nil
	}
	from, err := time.ParseInLocation(reportDateLayout, fromRaw, domain.Argentina)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: from must be YYYY-MM-DD",
			domain.ErrInvalidInput)
	}
	to, err := time.ParseInLocation(reportDateLayout, toRaw, domain.Argentina)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: to must be YYYY-MM-DD",
			domain.ErrInvalidInput)
	}
	if from.After(to) {
		return time.Time{}, time.Time{},
			fmt.Errorf("%w: from must not be after to", domain.ErrInvalidInput)
	}
	if int(to.Sub(from).Hours()/24)+1 > s.cfg.MaxRangeDays {
		return time.Time{}, time.Time{}, fmt.Errorf(
			"%w: the period must not exceed %d days", domain.ErrInvalidInput, s.cfg.MaxRangeDays)
	}
	return from, to, nil
}

func sellerParam(tenant domain.Tenant) *uuid.UUID {
	if tenant.IsAdmin() {
		return nil
	}
	return &tenant.UserID
}
