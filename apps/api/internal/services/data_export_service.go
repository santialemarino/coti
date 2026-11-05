package services

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
	"github.com/santialemarino/coti/apps/api/internal/utils/dataexport"
)

type dataExportStore interface {
	GetBranchName(context.Context, repository.Querier, uuid.UUID, uuid.UUID) (string, error)
	ListQuotes(context.Context, repository.Querier, domain.DataExportFilter) ([]domain.DataExportQuote, error)
	ListCatalog(context.Context, repository.Querier, domain.DataExportFilter) ([]domain.DataExportProduct, error)
	ListClients(context.Context, repository.Querier, domain.DataExportFilter) ([]domain.DataExportClient, error)
	ListInvoices(context.Context, repository.Querier, domain.DataExportFilter) ([]domain.DataExportInvoice, error)
}

// DataExportService turns an account's records into a CSV or XML file for another system.
type DataExportService struct {
	db      tenantTxRunner
	exports dataExportStore
	now     func() time.Time
}

// NewDataExportService builds a DataExportService.
func NewDataExportService(db tenantTxRunner, exports dataExportStore) *DataExportService {
	return &DataExportService{db: db, exports: exports, now: time.Now}
}

// Export reads one entity across the account, narrowed to a branch and period when given.
func (s *DataExportService) Export(
	ctx context.Context, tenant domain.Tenant, request domain.DataExportRequest,
) (*domain.DataExportFile, error) {
	if !tenant.IsAdmin() {
		return nil, domain.ErrForbidden
	}
	if !domain.IsValidDataExportEntity(request.Entity) {
		return nil, fmt.Errorf("%w: unknown export entity", domain.ErrInvalidInput)
	}
	if !domain.IsValidDataExportFormat(request.Format) {
		return nil, fmt.Errorf("%w: format must be csv or xml", domain.ErrInvalidInput)
	}
	filter, err := resolveDataExportFilter(tenant.AccountID, request)
	if err != nil {
		return nil, err
	}

	var table dataexport.Table
	var branchName string
	err = s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		if filter.BranchID != nil {
			name, err := s.exports.GetBranchName(ctx, q, tenant.AccountID, *filter.BranchID)
			if err != nil {
				return branchNotFoundAsInvalid(err)
			}
			branchName = name
		}
		var readErr error
		table, readErr = s.readTable(ctx, q, request.Entity, filter)
		return readErr
	})
	if err != nil {
		return nil, err
	}

	exportedAt := s.now().In(domain.Argentina)
	table.Attributes = dataExportAttributes(exportedAt, branchName, request.Entity, filter)
	var content []byte
	contentType := "text/csv; charset=utf-8"
	if request.Format == domain.DataExportXML {
		content, err = dataexport.WriteXML(table)
		contentType = "application/xml; charset=utf-8"
	} else {
		content, err = dataexport.WriteCSV(table)
	}
	if err != nil {
		return nil, err
	}
	name := table.Root
	if branchName != "" {
		name += "-" + slugFilename(branchName)
	}
	return &domain.DataExportFile{
		Filename:    name + "-" + exportedAt.Format(reportDateLayout) + "." + string(request.Format),
		ContentType: contentType,
		Content:     content,
	}, nil
}

func (s *DataExportService) readTable(ctx context.Context, q repository.Querier,
	entity domain.DataExportEntity, filter domain.DataExportFilter,
) (dataexport.Table, error) {
	switch entity {
	case domain.DataExportQuotes:
		quotes, err := s.exports.ListQuotes(ctx, q, filter)
		if err != nil {
			return dataexport.Table{}, err
		}
		return quoteExportTable(quotes)
	case domain.DataExportCatalog:
		products, err := s.exports.ListCatalog(ctx, q, filter)
		return catalogExportTable(products), err
	case domain.DataExportClients:
		clients, err := s.exports.ListClients(ctx, q, filter)
		return clientExportTable(clients), err
	default:
		invoices, err := s.exports.ListInvoices(ctx, q, filter)
		return invoiceExportTable(invoices), err
	}
}

// resolveDataExportFilter validates the branch and the inclusive Argentine dates; the period applies
// only to quotes and invoices, and the branch to everything but clients.
func resolveDataExportFilter(accountID uuid.UUID, request domain.DataExportRequest,
) (domain.DataExportFilter, error) {
	filter := domain.DataExportFilter{AccountID: accountID}
	if request.BranchID != "" && request.Entity != domain.DataExportClients {
		branchID, err := uuid.Parse(request.BranchID)
		if err != nil {
			return filter, fmt.Errorf("%w: branch_id must be a UUID", domain.ErrInvalidInput)
		}
		filter.BranchID = &branchID
	}
	if request.Entity != domain.DataExportQuotes && request.Entity != domain.DataExportInvoices {
		return filter, nil
	}
	var from, to time.Time
	if request.From != "" {
		parsed, err := time.ParseInLocation(reportDateLayout, request.From, domain.Argentina)
		if err != nil {
			return filter, fmt.Errorf("%w: from must be YYYY-MM-DD", domain.ErrInvalidInput)
		}
		from = parsed
		start := from.UTC()
		filter.StartAt, filter.FromDate = &start, &request.From
	}
	if request.To != "" {
		parsed, err := time.ParseInLocation(reportDateLayout, request.To, domain.Argentina)
		if err != nil {
			return filter, fmt.Errorf("%w: to must be YYYY-MM-DD", domain.ErrInvalidInput)
		}
		to = parsed
		end := to.AddDate(0, 0, 1).UTC()
		filter.EndAt, filter.ToDate = &end, &request.To
	}
	if !from.IsZero() && !to.IsZero() && to.Before(from) {
		return filter, fmt.Errorf("%w: to must not be before from", domain.ErrInvalidInput)
	}
	return filter, nil
}

func branchNotFoundAsInvalid(err error) error {
	if errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("%w: unknown branch", domain.ErrInvalidInput)
	}
	return err
}

func dataExportAttributes(exportedAt time.Time, branchName string, entity domain.DataExportEntity,
	filter domain.DataExportFilter,
) []dataexport.Attribute {
	attributes := []dataexport.Attribute{{Name: "exportado_el", Value: exportedAt.Format(time.RFC3339)}}
	if branchName != "" {
		attributes = append(attributes, dataexport.Attribute{Name: "sucursal", Value: branchName})
	}
	if entity == domain.DataExportQuotes || entity == domain.DataExportInvoices {
		if filter.FromDate != nil {
			attributes = append(attributes, dataexport.Attribute{Name: "desde", Value: *filter.FromDate})
		}
		if filter.ToDate != nil {
			attributes = append(attributes, dataexport.Attribute{Name: "hasta", Value: *filter.ToDate})
		}
	}
	return attributes
}
