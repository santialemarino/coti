package services

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type catalogImportRepository interface {
	ListExistingCodes(ctx context.Context, q repository.Querier, accountID uuid.UUID, codes []string) (map[string]struct{}, error)
	ApplyImport(ctx context.Context, q repository.Querier, tenant domain.Tenant, effectiveAt time.Time, rows []domain.CatalogImportRow) error
}

// CatalogImportService previews and confirms initial catalog spreadsheets.
type CatalogImportService struct {
	db      tenantTxRunner
	catalog catalogImportRepository
	now     func() time.Time
}

// NewCatalogImportService builds a CatalogImportService.
func NewCatalogImportService(
	db tenantTxRunner, catalog catalogImportRepository, now func() time.Time,
) *CatalogImportService {
	if now == nil {
		now = time.Now
	}
	return &CatalogImportService{db: db, catalog: catalog, now: now}
}

// Template creates the Spanish XLSX used for an initial catalog load.
func (s *CatalogImportService) Template(
	_ context.Context, tenant domain.Tenant,
) (*domain.CatalogImportFile, error) {
	if !tenant.HasBranch() {
		return nil, fmt.Errorf("%w: select a branch", domain.ErrInvalidInput)
	}
	content, err := buildCatalogImportXLSX()
	if err != nil {
		return nil, err
	}
	return &domain.CatalogImportFile{Filename: "catalogo-inicial.xlsx", Content: content}, nil
}

// Preview parses and validates a catalog spreadsheet without writing rows.
func (s *CatalogImportService) Preview(
	ctx context.Context, tenant domain.Tenant, filename string, src io.Reader,
) (*domain.CatalogImportPreview, error) {
	if !tenant.HasBranch() {
		return nil, fmt.Errorf("%w: select a branch", domain.ErrInvalidInput)
	}
	rawRows, err := parseCatalogImport(filename, src)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
	}
	var preview *domain.CatalogImportPreview
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var prepareErr error
		preview, prepareErr = s.prepare(ctx, q, tenant.AccountID, rawRows)
		return prepareErr
	}); err != nil {
		return nil, err
	}
	return preview, nil
}

// Confirm revalidates the reviewed rows and atomically creates every valid product.
func (s *CatalogImportService) Confirm(
	ctx context.Context, tenant domain.Tenant, inputs []domain.CatalogImportInput,
) (*domain.CatalogImportResult, error) {
	if !tenant.HasBranch() {
		return nil, fmt.Errorf("%w: select a branch", domain.ErrInvalidInput)
	}
	if len(inputs) == 0 {
		return nil, fmt.Errorf("%w: no rows to import", domain.ErrInvalidInput)
	}
	rawRows := make([]catalogImportRawRow, len(inputs))
	for i, input := range inputs {
		rawRows[i] = catalogImportRawRow{
			rowNumber: i + 2, code: input.Code, name: input.Name,
			description: input.Description, unit: input.Unit, price: input.Price,
			currency: input.Currency,
		}
		if input.Category != nil {
			rawRows[i].category = *input.Category
		}
		if input.MinPrice != nil {
			rawRows[i].minPrice = *input.MinPrice
		}
		if input.Conditions != nil {
			rawRows[i].conditions = *input.Conditions
		}
	}

	result := &domain.CatalogImportResult{}
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		preview, prepareErr := s.prepare(ctx, q, tenant.AccountID, rawRows)
		if prepareErr != nil {
			return prepareErr
		}
		validRows := make([]domain.CatalogImportRow, 0, preview.ValidRows)
		for _, row := range preview.Rows {
			if len(row.Errors) == 0 {
				validRows = append(validRows, row)
			}
		}
		if len(validRows) == 0 {
			return fmt.Errorf("%w: import has no valid rows", domain.ErrInvalidInput)
		}
		if applyErr := s.catalog.ApplyImport(ctx, q, tenant, s.now().UTC(), validRows); applyErr != nil {
			return applyErr
		}
		result.ImportedRows = len(validRows)
		result.SkippedRows = preview.InvalidRows
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *CatalogImportService) prepare(
	ctx context.Context, q repository.Querier, accountID uuid.UUID, rawRows []catalogImportRawRow,
) (*domain.CatalogImportPreview, error) {
	codes := make([]string, 0, len(rawRows))
	seen := make(map[string]int, len(rawRows))
	for _, row := range rawRows {
		code := strings.TrimSpace(row.code)
		if code != "" {
			codes = append(codes, code)
			seen[code]++
		}
	}
	existing, err := s.catalog.ListExistingCodes(ctx, q, accountID, codes)
	if err != nil {
		return nil, err
	}

	preview := &domain.CatalogImportPreview{
		Rows: make([]domain.CatalogImportRow, 0, len(rawRows)), PreviewedAt: s.now().UTC(),
	}
	for _, raw := range rawRows {
		row := prepareCatalogImportRow(raw, existing, seen)
		if len(row.Errors) == 0 {
			preview.ValidRows++
		} else {
			preview.InvalidRows++
		}
		preview.Rows = append(preview.Rows, row)
	}
	preview.CanConfirm = preview.ValidRows > 0
	return preview, nil
}

func prepareCatalogImportRow(
	raw catalogImportRawRow, existing map[string]struct{}, seen map[string]int,
) domain.CatalogImportRow {
	description := strings.TrimSpace(raw.description)
	row := domain.CatalogImportRow{
		RowNumber: raw.rowNumber, Code: strings.TrimSpace(raw.code),
		Name: strings.TrimSpace(raw.name), Description: description, Unit: strings.TrimSpace(raw.unit),
	}
	if row.Name == "" {
		row.Name = description
	}
	if row.Code == "" {
		row.Errors = append(row.Errors, "missing_code")
	} else {
		if utf8.RuneCountInString(row.Code) > 255 {
			row.Errors = append(row.Errors, "code_too_long")
		}
		if seen[row.Code] > 1 {
			row.Errors = append(row.Errors, "duplicate_code")
		}
		if _, ok := existing[row.Code]; ok {
			row.Errors = append(row.Errors, "existing_code")
		}
	}
	if description == "" {
		row.Errors = append(row.Errors, "missing_description")
	} else if utf8.RuneCountInString(description) > 512 {
		row.Errors = append(row.Errors, "description_too_long")
	}
	if utf8.RuneCountInString(row.Name) > 255 {
		row.Errors = append(row.Errors, "name_too_long")
	}
	if row.Unit == "" {
		row.Errors = append(row.Errors, "missing_unit")
	} else if utf8.RuneCountInString(row.Unit) > 64 {
		row.Errors = append(row.Errors, "unit_too_long")
	}
	category := strings.TrimSpace(raw.category)
	if utf8.RuneCountInString(category) > 255 {
		row.Errors = append(row.Errors, "category_too_long")
	} else if category != "" {
		row.Category = &category
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

	row.Currency = strings.ToUpper(strings.TrimSpace(raw.currency))
	if row.Currency == "" {
		row.Currency = domain.DefaultCurrency
	}
	if !currencyPattern.MatchString(row.Currency) {
		row.Errors = append(row.Errors, "invalid_currency")
	}
	conditions := strings.TrimSpace(raw.conditions)
	if utf8.RuneCountInString(conditions) > 255 {
		row.Errors = append(row.Errors, "conditions_too_long")
	} else if conditions != "" {
		row.Conditions = &conditions
	}
	return row
}
