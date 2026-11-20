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
	ListImportState(ctx context.Context, q repository.Querier, accountID, branchID uuid.UUID, codes []string) (map[string]domain.CatalogProductState, error)
	ListProductsByCodes(ctx context.Context, q repository.Querier, accountID uuid.UUID, codes []string) (map[string]domain.CatalogProductRef, error)
	ApplyImport(ctx context.Context, q repository.Querier, tenant domain.Tenant, effectiveAt time.Time, rows []domain.CatalogImportRow) error
	ApplyRecommendationSlots(ctx context.Context, q repository.Querier, accountID uuid.UUID, changes []domain.CatalogSlotChange) error
	DatabaseTime(ctx context.Context, q repository.Querier) (time.Time, error)
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
		preview, prepareErr = s.prepare(ctx, q, tenant, rawRows)
		return prepareErr
	}); err != nil {
		return nil, err
	}
	return preview, nil
}

// Confirm revalidates the reviewed rows and atomically applies every valid row that changes the
// catalog; a valid row matching what is stored is counted and left alone.
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
			economyAlternative: trimmedAlternative(input.EconomyAlternative),
			premiumAlternative: trimmedAlternative(input.PremiumAlternative),
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
		preview, prepareErr := s.prepare(ctx, q, tenant, rawRows)
		if prepareErr != nil {
			return prepareErr
		}
		if preview.ValidRows == 0 {
			return fmt.Errorf("%w: import has no valid rows", domain.ErrInvalidInput)
		}
		result.CreatedRows, result.UpdatedRows = preview.CreateRows, preview.UpdateRows
		result.UnchangedRows, result.SkippedRows = preview.UnchangedRows, preview.InvalidRows
		changedRows := make([]domain.CatalogImportRow, 0, preview.CreateRows+preview.UpdateRows)
		for _, row := range preview.Rows {
			if len(row.Errors) == 0 && row.Action != domain.CatalogImportUnchanged {
				changedRows = append(changedRows, row)
			}
		}
		if len(changedRows) == 0 {
			return nil
		}
		// The database's clock, so the new prices are in force for the very next read.
		effectiveAt, timeErr := s.catalog.DatabaseTime(ctx, q)
		if timeErr != nil {
			return timeErr
		}
		if applyErr := s.catalog.ApplyImport(ctx, q, tenant, effectiveAt.UTC(), changedRows); applyErr != nil {
			return applyErr
		}
		return s.catalog.ApplyRecommendationSlots(ctx, q, tenant.AccountID, catalogSlotChanges(changedRows))
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *CatalogImportService) prepare(
	ctx context.Context, q repository.Querier, tenant domain.Tenant, rawRows []catalogImportRawRow,
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
	states, err := s.catalog.ListImportState(ctx, q, tenant.AccountID, tenant.BranchID, codes)
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
	alternativeCodes := make([]string, 0)
	for _, raw := range rawRows {
		preview.Rows = append(preview.Rows, prepareCatalogImportRow(raw, states, seen, families))
		for _, code := range []*string{raw.economyAlternative, raw.premiumAlternative} {
			if code != nil && *code != "" {
				alternativeCodes = append(alternativeCodes, *code)
			}
		}
		for _, word := range []string{raw.economyClearWord, raw.premiumClearWord} {
			if word != "" {
				alternativeCodes = append(alternativeCodes, word)
			}
		}
	}
	keepUnchangedSlots(preview.Rows, states)
	stored, err := s.catalog.ListProductsByCodes(ctx, q, tenant.AccountID, alternativeCodes)
	if err != nil {
		return nil, err
	}
	flagAmbiguousClearWords(preview.Rows, rawRows, stored, seen)
	validateCatalogAlternatives(preview.Rows, stored)
	for i := range preview.Rows {
		row := &preview.Rows[i]
		if len(row.Errors) > 0 {
			preview.InvalidRows++
			continue
		}
		preview.ValidRows++
		if state, ok := states[row.Code]; ok && !catalogRowChanges(*row, state) {
			row.Action = domain.CatalogImportUnchanged
		}
		switch row.Action {
		case domain.CatalogImportCreate:
			preview.CreateRows++
		case domain.CatalogImportUpdate:
			preview.UpdateRows++
		default:
			preview.UnchangedRows++
		}
	}
	preview.CanConfirm = preview.CreateRows+preview.UpdateRows > 0
	return preview, nil
}

func prepareCatalogImportRow(
	raw catalogImportRawRow, states map[string]domain.CatalogProductState, seen map[string]int,
	families []domain.ProductFamily,
) domain.CatalogImportRow {
	description := strings.TrimSpace(raw.description)
	row := domain.CatalogImportRow{
		RowNumber: raw.rowNumber, Code: strings.TrimSpace(raw.code),
		Name: strings.TrimSpace(raw.name), Description: description, Unit: strings.TrimSpace(raw.unit),
		IsActive: true, EconomyAlternative: raw.economyAlternative, PremiumAlternative: raw.premiumAlternative,
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
		if _, ok := states[row.Code]; ok {
			isExisting = true
			row.Action = domain.CatalogImportUpdate
		} else {
			row.Action = domain.CatalogImportCreate
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
		row.Errors = append(row.Errors, moneyError("price", err))
	} else {
		row.Price = price
		priceValue = normalizedPrice
	}
	if strings.TrimSpace(raw.minPrice) != "" {
		minPrice, minPriceValue, minErr := normalizeMoney(raw.minPrice)
		if minErr != nil || minPriceValue.Sign() <= 0 {
			row.Errors = append(row.Errors, moneyError("min_price", minErr))
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

// flagAmbiguousClearWords refuses a row whose clear word is also a product code, in the catalog or
// in the file: it can't tell clearing the slot from naming that product.
func flagAmbiguousClearWords(
	rows []domain.CatalogImportRow, rawRows []catalogImportRawRow,
	stored map[string]domain.CatalogProductRef, fileCodes map[string]int,
) {
	for i, raw := range rawRows {
		for _, slot := range []struct{ prefix, word string }{
			{"economy", raw.economyClearWord}, {"premium", raw.premiumClearWord},
		} {
			if slot.word == "" {
				continue
			}
			_, inCatalog := stored[slot.word]
			if inCatalog || fileCodes[slot.word] > 0 {
				rows[i].Errors = append(rows[i].Errors, slot.prefix+"_alternative_ambiguous")
			}
		}
	}
}

/*
 * validateCatalogAlternatives checks every row's recommendation slots against the catalog as it
 * will stand after the import: a product the file changes is judged by its row, any other by its
 * stored values. A row refused here is skipped, which can strand a row that named it, so the check
 * repeats until no further row is refused.
 */
func validateCatalogAlternatives(rows []domain.CatalogImportRow, stored map[string]domain.CatalogProductRef) {
	for {
		final := make(map[string]domain.CatalogProductRef, len(stored)+len(rows))
		for code, ref := range stored {
			final[code] = ref
		}
		for _, row := range rows {
			if len(row.Errors) == 0 {
				familyID := row.FamilyID
				final[row.Code] = domain.CatalogProductRef{
					FamilyID: &familyID, SubgroupID: row.SubgroupID, Unit: row.Unit, IsActive: row.IsActive,
				}
			}
		}
		refused := false
		for i := range rows {
			if len(rows[i].Errors) > 0 {
				continue
			}
			if errs := catalogAlternativeErrors(rows[i], final); len(errs) > 0 {
				rows[i].Errors = append(rows[i].Errors, errs...)
				refused = true
			}
		}
		if !refused {
			return
		}
	}
}

func catalogAlternativeErrors(row domain.CatalogImportRow, final map[string]domain.CatalogProductRef) []string {
	var errs []string
	economy, premium := row.EconomyAlternative, row.PremiumAlternative
	if economy != nil && premium != nil && *economy != "" && *economy == *premium {
		errs = append(errs, "alternative_repeated")
	}
	for _, slot := range []struct {
		prefix string
		code   *string
	}{{"economy", economy}, {"premium", premium}} {
		if slot.code == nil || *slot.code == "" {
			continue
		}
		ref, ok := final[*slot.code]
		switch {
		case *slot.code == row.Code:
			errs = append(errs, slot.prefix+"_alternative_self")
		case !ok:
			errs = append(errs, slot.prefix+"_alternative_not_found")
		case !ref.IsActive:
			errs = append(errs, slot.prefix+"_alternative_inactive")
		case !compatibleCatalogAlternative(row, ref):
			errs = append(errs, slot.prefix+"_alternative_incompatible")
		}
	}
	return errs
}

// compatibleCatalogAlternative mirrors the rule every recommendation slot follows: same family,
// same subgroup and same unit, none of them missing.
func compatibleCatalogAlternative(row domain.CatalogImportRow, ref domain.CatalogProductRef) bool {
	unit := strings.TrimSpace(ref.Unit)
	return row.SubgroupID != nil && ref.FamilyID != nil && ref.SubgroupID != nil &&
		*ref.FamilyID == row.FamilyID && *ref.SubgroupID == *row.SubgroupID &&
		unit != "" && strings.EqualFold(unit, strings.TrimSpace(row.Unit))
}

func catalogSlotChanges(rows []domain.CatalogImportRow) []domain.CatalogSlotChange {
	var changes []domain.CatalogSlotChange
	for _, row := range rows {
		if row.EconomyAlternative != nil {
			changes = append(changes, domain.CatalogSlotChange{
				BaseCode: row.Code, Type: domain.ProductAlternativeEconomy, AlternativeCode: *row.EconomyAlternative,
			})
		}
		if row.PremiumAlternative != nil {
			changes = append(changes, domain.CatalogSlotChange{
				BaseCode: row.Code, Type: domain.ProductAlternativePremium, AlternativeCode: *row.PremiumAlternative,
			})
		}
	}
	return changes
}

func trimmedAlternative(code *string) *string {
	if code == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*code)
	return &trimmed
}

/*
 * keepUnchangedSlots reads a code equal to what the slot already holds, or a clear of a slot that
 * is already empty, as "keep". An exported file carries every stored slot, and one whose product
 * was since deactivated or reclassified would otherwise refuse a row the supplier only opened to
 * change a price.
 */
func keepUnchangedSlots(rows []domain.CatalogImportRow, states map[string]domain.CatalogProductState) {
	for i := range rows {
		state, ok := states[rows[i].Code]
		// Both columns naming one product is refused as repeated, never read as a kept slot.
		if !ok || sameSlot(rows[i].EconomyAlternative, rows[i].PremiumAlternative) {
			continue
		}
		if keepsSlot(rows[i].EconomyAlternative, state.Slots.Economy) {
			rows[i].EconomyAlternative = nil
		}
		if keepsSlot(rows[i].PremiumAlternative, state.Slots.Premium) {
			rows[i].PremiumAlternative = nil
		}
	}
}

func keepsSlot(requested, stored *string) bool {
	return sameSlot(requested, stored) || (requested != nil && *requested == "" && stored == nil)
}

func sameSlot(requested, stored *string) bool {
	return requested != nil && stored != nil && *requested != "" && *requested == *stored
}

/*
 * catalogRowChanges reports whether confirming a valid row would write anything the product does
 * not already hold: an attribute, its availability or price at the branch, or a slot. Text is
 * compared trimmed, a blank description equals none, and money is compared by value.
 */
func catalogRowChanges(row domain.CatalogImportRow, state domain.CatalogProductState) bool {
	if !sameText(row.Name, state.Name) || !sameText(row.Description, state.Description) ||
		!sameText(row.Unit, state.Unit) {
		return true
	}
	if state.FamilyID == nil || *state.FamilyID != row.FamilyID || !sameOptionalID(row.SubgroupID, state.SubgroupID) {
		return true
	}
	// The import writes the active flag to the product and to the branch's availability alike.
	if row.IsActive != state.IsActive || state.BranchActive == nil || *state.BranchActive != row.IsActive {
		return true
	}
	// A blank price keeps the one in force; any other is written when it or its floor differs.
	if row.Price != "" && (state.Price == nil || !sameMoney(row.Price, *state.Price) ||
		!sameOptionalMoney(row.MinPrice, state.MinPrice)) {
		return true
	}
	return row.EconomyAlternative != nil || row.PremiumAlternative != nil
}

func sameText(a, b string) bool {
	return strings.TrimSpace(a) == strings.TrimSpace(b)
}

func sameOptionalID(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
