package services

import (
	"context"
	"fmt"
	"io"
	"math/big"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type catalogImportRepository interface {
	ListForExport(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID) (*domain.CatalogExport, error)
	ListTaxonomy(ctx context.Context, q repository.Querier) ([]domain.ProductFamily, error)
	ListExistingCodes(ctx context.Context, q repository.Querier, accountID uuid.UUID, codes []string) (map[string]struct{}, error)
	ApplyImport(ctx context.Context, q repository.Querier, tenant domain.Tenant, effectiveAt time.Time, rows []domain.CatalogImportRow) error
}

// CatalogImportService previews and confirms bulk catalog spreadsheets.
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

// Template creates the Spanish XLSX populated with the account's current catalog.
func (s *CatalogImportService) Template(
	ctx context.Context, tenant domain.Tenant,
) (*domain.CatalogImportFile, error) {
	if !tenant.HasBranch() {
		return nil, fmt.Errorf("%w: select a branch", domain.ErrInvalidInput)
	}
	var families []domain.ProductFamily
	var export *domain.CatalogExport
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var listErr error
		export, listErr = s.catalog.ListForExport(ctx, q, tenant.AccountID, tenant.BranchID)
		if listErr != nil {
			return listErr
		}
		families, listErr = s.catalog.ListTaxonomy(ctx, q)
		return listErr
	}); err != nil {
		return nil, err
	}
	content, err := buildCatalogImportXLSX(*export, families)
	if err != nil {
		return nil, err
	}
	return &domain.CatalogImportFile{
		Filename: "catalogo-" + slugFilename(export.BranchName) + ".xlsx", Content: content,
	}, nil
}

// Preview parses and validates a catalog spreadsheet without writing rows.
func (s *CatalogImportService) Preview(
	ctx context.Context, tenant domain.Tenant, src io.Reader,
) (*domain.CatalogImportPreview, error) {
	if !tenant.HasBranch() {
		return nil, fmt.Errorf("%w: select a branch", domain.ErrInvalidInput)
	}
	rawRows, err := parseCatalogImport(src)
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

// Confirm revalidates the reviewed rows and atomically applies every valid product change.
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
			family: input.Family, active: activeSpreadsheetValue(input.IsActive),
		}
		if input.Subgroup != nil {
			rawRows[i].subgroup = *input.Subgroup
		}
		if input.MinPrice != nil {
			rawRows[i].minPrice = *input.MinPrice
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
		for _, row := range validRows {
			if row.Action == "CREATE" {
				result.CreatedRows++
			} else {
				result.UpdatedRows++
			}
		}
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
	families, err := s.catalog.ListTaxonomy(ctx, q)
	if err != nil {
		return nil, err
	}

	preview := &domain.CatalogImportPreview{
		Rows: make([]domain.CatalogImportRow, 0, len(rawRows)), PreviewedAt: s.now().UTC(),
	}
	for _, raw := range rawRows {
		row := prepareCatalogImportRow(raw, existing, seen, families)
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
	families []domain.ProductFamily,
) domain.CatalogImportRow {
	description := strings.TrimSpace(raw.description)
	row := domain.CatalogImportRow{
		RowNumber: raw.rowNumber, Code: strings.TrimSpace(raw.code),
		Name: strings.TrimSpace(raw.name), Description: description, Unit: strings.TrimSpace(raw.unit),
		IsActive: true,
	}
	isExisting := false
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
			isExisting = true
			row.Action = "UPDATE"
		} else {
			row.Action = "CREATE"
		}
	}
	active, activeErr := parseSpreadsheetActive(raw.active)
	if activeErr != nil {
		row.Errors = append(row.Errors, "invalid_active")
	} else {
		row.IsActive = active
	}
	if utf8.RuneCountInString(description) > 512 {
		row.Errors = append(row.Errors, "description_too_long")
	}
	if row.Name == "" {
		row.Errors = append(row.Errors, "missing_name")
	} else if utf8.RuneCountInString(row.Name) > 255 {
		row.Errors = append(row.Errors, "name_too_long")
	}
	if row.Unit == "" {
		row.Errors = append(row.Errors, "missing_unit")
	} else if utf8.RuneCountInString(row.Unit) > 64 {
		row.Errors = append(row.Errors, "unit_too_long")
	}
	resolveCatalogTaxonomy(&row, raw, families)

	var priceValue *big.Rat
	price, normalizedPrice, err := normalizeMoney(raw.price)
	if strings.TrimSpace(raw.price) == "" && isExisting {
		if strings.TrimSpace(raw.minPrice) != "" {
			row.Errors = append(row.Errors, "invalid_price")
		}
	} else if err != nil || normalizedPrice.Sign() <= 0 {
		row.Errors = append(row.Errors, "invalid_price")
	} else {
		row.Price = price
		priceValue = normalizedPrice
	}
	if strings.TrimSpace(raw.minPrice) != "" {
		minPrice, minPriceValue, minErr := normalizeMoney(raw.minPrice)
		if minErr != nil || minPriceValue.Sign() <= 0 {
			row.Errors = append(row.Errors, "invalid_min_price")
		} else {
			row.MinPrice = &minPrice
			if priceValue != nil && minPriceValue.Cmp(priceValue) > 0 {
				row.Errors = append(row.Errors, "min_price_above_price")
			}
		}
	}

	return row
}

// Taxonomy returns the families and subgroups available to product editors.
func (s *CatalogImportService) Taxonomy(
	ctx context.Context, tenant domain.Tenant,
) ([]domain.ProductFamily, error) {
	var families []domain.ProductFamily
	err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var listErr error
		families, listErr = s.catalog.ListTaxonomy(ctx, q)
		return listErr
	})
	return families, err
}

func parseSpreadsheetActive(raw string) (bool, error) {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "", "SI", "SÍ", "TRUE", "1":
		return true, nil
	case "NO", "FALSE", "0":
		return false, nil
	default:
		return false, fmt.Errorf("invalid active value")
	}
}

func activeSpreadsheetValue(active bool) string {
	if active {
		return "SI"
	}
	return "NO"
}

func resolveCatalogTaxonomy(
	row *domain.CatalogImportRow, raw catalogImportRawRow, families []domain.ProductFamily,
) {
	familyName := strings.ToUpper(strings.TrimSpace(raw.family))
	if familyName == "" {
		row.Errors = append(row.Errors, "missing_family")
		return
	}
	var selected *domain.ProductFamily
	for i := range families {
		if strings.EqualFold(families[i].Name, familyName) {
			selected = &families[i]
			break
		}
	}
	if selected == nil {
		row.Errors = append(row.Errors, "invalid_family")
		return
	}
	row.FamilyID, row.Family = selected.ID, selected.Name
	subgroupName := strings.TrimSpace(raw.subgroup)
	if subgroupName == "" {
		return
	}
	for _, subgroup := range selected.Subgroups {
		if strings.EqualFold(subgroup.Name, subgroupName) {
			row.SubgroupID = &subgroup.ID
			row.Subgroup = &subgroup.Name
			return
		}
	}
	row.Errors = append(row.Errors, "invalid_subgroup")
}
