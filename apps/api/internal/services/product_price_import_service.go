package services

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type productPriceImportRepository interface {
	ListCurrentForExport(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID) (*domain.ProductPriceExport, error)
	GetByCodes(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID, codes []string) (map[string]domain.ProductPriceLookup, error)
	ApplyImport(ctx context.Context, q repository.Querier, tenant domain.Tenant, effectiveAt time.Time, updates []domain.ProductPriceUpdate) error
}

// ProductPriceImportService previews and confirms branch price spreadsheets.
type ProductPriceImportService struct {
	db     tenantScoper
	prices productPriceImportRepository
	now    func() time.Time
}

// NewProductPriceImportService builds a ProductPriceImportService.
func NewProductPriceImportService(db tenantScoper, prices productPriceImportRepository, now func() time.Time) *ProductPriceImportService {
	if now == nil {
		now = time.Now
	}
	return &ProductPriceImportService{db: db, prices: prices, now: now}
}

// Export creates an XLSX template populated with the branch's current prices.
func (s *ProductPriceImportService) Export(
	ctx context.Context, tenant domain.Tenant,
) (*domain.ProductPriceExportFile, error) {
	if !tenant.HasBranch() {
		return nil, fmt.Errorf("%w: select a branch", domain.ErrInvalidInput)
	}
	var export *domain.ProductPriceExport
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		export, err = s.prices.ListCurrentForExport(ctx, q, tenant.AccountID, tenant.BranchID)
		return err
	}); err != nil {
		return nil, err
	}
	if len(export.Rows) == 0 {
		return nil, fmt.Errorf("%w: branch has no current prices", domain.ErrInvalidInput)
	}
	content, err := buildProductPriceXLSX(*export)
	if err != nil {
		return nil, err
	}
	return &domain.ProductPriceExportFile{
		Filename: "precios-" + slugFilename(export.BranchName) + ".xlsx",
		Content:  content,
	}, nil
}

// Preview parses and validates a spreadsheet without changing prices.
func (s *ProductPriceImportService) Preview(
	ctx context.Context, tenant domain.Tenant, filename string, src io.Reader,
) (*domain.ProductPriceImportPreview, error) {
	if !tenant.HasBranch() {
		return nil, fmt.Errorf("%w: select a branch", domain.ErrInvalidInput)
	}
	rawRows, err := parsePriceImport(filename, src)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
	}
	return s.prepare(ctx, tenant, rawRows)
}

// Confirm validates the reviewed rows again and atomically creates the new price versions.
func (s *ProductPriceImportService) Confirm(
	ctx context.Context, tenant domain.Tenant, inputs []domain.ProductPriceImportInput,
) (int, error) {
	if !tenant.HasBranch() {
		return 0, fmt.Errorf("%w: select a branch", domain.ErrInvalidInput)
	}
	if len(inputs) == 0 {
		return 0, fmt.Errorf("%w: no rows to import", domain.ErrInvalidInput)
	}
	rawRows := make([]priceImportRawRow, len(inputs))
	for i, input := range inputs {
		rawRows[i] = priceImportRawRow{
			rowNumber: i + 2,
			code:      input.Code,
			price:     input.Price,
		}
		if input.MinPrice != nil {
			rawRows[i].minPrice = *input.MinPrice
		}
	}

	preview, err := s.prepare(ctx, tenant, rawRows)
	if err != nil {
		return 0, err
	}
	if !preview.CanConfirm {
		return 0, fmt.Errorf("%w: import contains invalid rows", domain.ErrInvalidInput)
	}
	updates := make([]domain.ProductPriceUpdate, len(preview.Rows))
	for i, row := range preview.Rows {
		updates[i] = domain.ProductPriceUpdate{
			ProductID: row.ProductID,
			Price:     row.Price,
			MinPrice:  row.MinPrice,
			Currency:  row.Currency,
		}
	}
	effectiveAt := s.now().UTC()
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		return s.prices.ApplyImport(ctx, q, tenant, effectiveAt, updates)
	}); err != nil {
		return 0, err
	}
	return len(updates), nil
}

func (s *ProductPriceImportService) prepare(
	ctx context.Context, tenant domain.Tenant, rawRows []priceImportRawRow,
) (*domain.ProductPriceImportPreview, error) {
	codes := make([]string, 0, len(rawRows))
	seen := make(map[string]int, len(rawRows))
	for _, row := range rawRows {
		code := strings.TrimSpace(row.code)
		if code != "" {
			codes = append(codes, code)
			seen[code]++
		}
	}

	var products map[string]domain.ProductPriceLookup
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var err error
		products, err = s.prices.GetByCodes(ctx, q, tenant.AccountID, tenant.BranchID, codes)
		return err
	}); err != nil {
		return nil, err
	}

	preview := &domain.ProductPriceImportPreview{
		Rows:        make([]domain.ProductPriceImportRow, 0, len(rawRows)),
		PreviewedAt: s.now().UTC(),
	}
	for _, raw := range rawRows {
		row := preparePriceImportRow(raw, products, seen)
		if len(row.Errors) == 0 {
			preview.ValidRows++
		} else {
			preview.InvalidRows++
		}
		preview.Rows = append(preview.Rows, row)
	}
	preview.CanConfirm = preview.ValidRows > 0 && preview.InvalidRows == 0
	return preview, nil
}

func preparePriceImportRow(
	raw priceImportRawRow, products map[string]domain.ProductPriceLookup, seen map[string]int,
) domain.ProductPriceImportRow {
	row := domain.ProductPriceImportRow{
		RowNumber: raw.rowNumber,
		Code:      strings.TrimSpace(raw.code),
		Currency:  domain.DefaultCurrency,
	}
	if row.Code == "" {
		row.Errors = append(row.Errors, "missing_code")
	} else {
		if seen[row.Code] > 1 {
			row.Errors = append(row.Errors, "duplicate_code")
		}
		product, ok := products[row.Code]
		if !ok {
			row.Errors = append(row.Errors, "unknown_product")
		} else {
			row.ProductID = product.ProductID
			row.ProductName = product.ProductName
			row.CurrentPrice = product.CurrentPrice
			row.CurrentMinPrice = product.CurrentMinPrice
			if product.CurrentCurrency != nil {
				row.Currency = *product.CurrentCurrency
			}
		}
	}

	price, priceValue, err := normalizeMoney(raw.price)
	if err != nil || priceValue.Sign() <= 0 {
		row.Errors = append(row.Errors, "invalid_price")
	} else {
		row.Price = price
	}

	if strings.TrimSpace(raw.minPrice) != "" {
		minPrice, minPriceValue, minErr := normalizeMoney(raw.minPrice)
		if minErr != nil || minPriceValue.Sign() <= 0 {
			row.Errors = append(row.Errors, "invalid_min_price")
		} else {
			row.MinPrice = &minPrice
			if err == nil && minPriceValue.Cmp(priceValue) > 0 {
				row.Errors = append(row.Errors, "min_price_above_price")
			}
		}
	}

	return row
}
