package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// CatalogImportRepository owns the bulk persistence used by initial catalog loads.
type CatalogImportRepository struct{}

// NewCatalogImportRepository builds a CatalogImportRepository.
func NewCatalogImportRepository() *CatalogImportRepository {
	return &CatalogImportRepository{}
}

// ListExistingCodes returns the requested codes already owned by the account.
func (r *CatalogImportRepository) ListExistingCodes(
	ctx context.Context, q Querier, accountID uuid.UUID, codes []string,
) (map[string]struct{}, error) {
	rows, err := q.Query(ctx,
		`SELECT code
		 FROM product
		 WHERE account_id = $1
		   AND code = ANY($2)`,
		accountID, codes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	existing := make(map[string]struct{}, len(codes))
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, err
		}
		existing[code] = struct{}{}
	}
	return existing, rows.Err()
}

// ApplyImport creates products, branch availability, and initial prices in one batch.
func (r *CatalogImportRepository) ApplyImport(
	ctx context.Context, q Querier, tenant domain.Tenant, effectiveAt time.Time,
	rows []domain.CatalogImportRow,
) error {
	codes := make([]string, len(rows))
	names := make([]string, len(rows))
	descriptions := make([]string, len(rows))
	units := make([]string, len(rows))
	categories := make([]string, len(rows))
	prices := make([]string, len(rows))
	minPrices := make([]string, len(rows))
	currencies := make([]string, len(rows))
	conditions := make([]string, len(rows))
	for i, row := range rows {
		codes[i] = row.Code
		names[i] = row.Name
		descriptions[i] = row.Description
		units[i] = row.Unit
		prices[i] = row.Price
		currencies[i] = row.Currency
		if row.Category != nil {
			categories[i] = *row.Category
		}
		if row.MinPrice != nil {
			minPrices[i] = *row.MinPrice
		}
		if row.Conditions != nil {
			conditions[i] = *row.Conditions
		}
	}

	var imported int
	err := q.QueryRow(ctx,
		`WITH incoming AS (
		   SELECT code, name, description, unit, category, price, min_price, currency, conditions
		   FROM unnest(
		     $5::text[], $6::text[], $7::text[], $8::text[], $9::text[],
		     $10::text[], $11::text[], $12::text[], $13::text[]
		   ) AS u(code, name, description, unit, category, price, min_price, currency, conditions)
		 ), inserted_products AS (
		   INSERT INTO product (account_id, code, canonical_name, description, unit, category)
		   SELECT $1, code, name, description, unit, NULLIF(category, '')
		   FROM incoming
		   RETURNING id, code
		 ), inserted_availability AS (
		   INSERT INTO branch_product (account_id, branch_id, product_id, stock, is_active)
		   SELECT $1, $2, id, NULL, TRUE
		   FROM inserted_products
		   RETURNING product_id
		 ), inserted_prices AS (
		   INSERT INTO product_price
		     (account_id, branch_id, product_id, user_id, price, currency, conditions,
		      min_price, valid_from)
		   SELECT $1, $2, p.id, $3, i.price::numeric, i.currency,
		          NULLIF(i.conditions, ''), NULLIF(i.min_price, '')::numeric, $4
		   FROM inserted_products p
		   JOIN incoming i ON i.code = p.code
		   JOIN inserted_availability a ON a.product_id = p.id
		   RETURNING product_id
		 )
		 SELECT count(*) FROM inserted_prices`,
		tenant.AccountID, tenant.BranchID, tenant.UserID, effectiveAt, codes, names,
		descriptions, units, categories, prices, minPrices, currencies, conditions,
	).Scan(&imported)
	if isUniqueViolation(err, productCodeIndex) {
		return domain.ErrConflict
	}
	if err != nil {
		return err
	}
	if imported != len(rows) {
		return domain.ErrConflict
	}
	return nil
}
