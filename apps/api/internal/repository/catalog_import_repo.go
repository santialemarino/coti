package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// CatalogImportRepository owns bulk catalog export and persistence.
type CatalogImportRepository struct{}

// NewCatalogImportRepository builds a CatalogImportRepository.
func NewCatalogImportRepository() *CatalogImportRepository {
	return &CatalogImportRepository{}
}

// ListForExport loads every coded product and its current values for one branch.
func (r *CatalogImportRepository) ListForExport(
	ctx context.Context, q Querier, accountID, branchID uuid.UUID,
) (*domain.CatalogExport, error) {
	export := &domain.CatalogExport{}
	if err := q.QueryRow(ctx,
		`SELECT name FROM branch WHERE account_id = $1 AND id = $2 AND is_active = TRUE`,
		accountID, branchID).Scan(&export.BranchName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}

	rows, err := q.Query(ctx,
		`SELECT p.code, p.canonical_name, coalesce(p.description, ''),
		        coalesce(p.unit, ''), f.name, s.name, pp.price::text, pp.min_price::text,
		        p.is_active
		 FROM product p
		 JOIN product_family f ON f.id = p.family_id
		 LEFT JOIN product_subgroup s ON s.id = p.subgroup_id AND s.family_id = p.family_id
		 LEFT JOIN LATERAL (
		   SELECT price, min_price
		   FROM product_price
		   WHERE account_id = $1 AND branch_id = $2 AND product_id = p.id
		     AND valid_from <= now() AND (valid_to IS NULL OR valid_to > now())
		   ORDER BY valid_from DESC
		   LIMIT 1
		 ) pp ON TRUE
		 WHERE p.account_id = $1 AND p.code IS NOT NULL
		 ORDER BY p.code`,
		accountID, branchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var row domain.CatalogExportRow
		if err := rows.Scan(&row.Code, &row.Name, &row.Description, &row.Unit,
			&row.Family, &row.Subgroup, &row.Price, &row.MinPrice, &row.IsActive); err != nil {
			return nil, err
		}
		export.Rows = append(export.Rows, row)
	}
	return export, rows.Err()
}

// ListTaxonomy returns the global families and their optional subgroups in display order.
func (r *CatalogImportRepository) ListTaxonomy(
	ctx context.Context, q Querier,
) ([]domain.ProductFamily, error) {
	rows, err := q.Query(ctx,
		`SELECT f.id, f.name, s.id, s.name
		 FROM product_family f
		 LEFT JOIN product_subgroup s ON s.family_id = f.id
		 ORDER BY f.sort_order, s.sort_order`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var families []domain.ProductFamily
	for rows.Next() {
		var familyID uuid.UUID
		var familyName string
		var subgroupID *uuid.UUID
		var subgroupName *string
		if err := rows.Scan(&familyID, &familyName, &subgroupID, &subgroupName); err != nil {
			return nil, err
		}
		if len(families) == 0 || families[len(families)-1].ID != familyID {
			families = append(families, domain.ProductFamily{ID: familyID, Name: familyName})
		}
		if subgroupID != nil {
			families[len(families)-1].Subgroups = append(families[len(families)-1].Subgroups,
				domain.ProductSubgroup{ID: *subgroupID, FamilyID: familyID, Name: *subgroupName})
		}
	}
	return families, rows.Err()
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

// ApplyImport upserts products, branch availability, and changed prices in one batch.
func (r *CatalogImportRepository) ApplyImport(
	ctx context.Context, q Querier, tenant domain.Tenant, effectiveAt time.Time,
	rows []domain.CatalogImportRow,
) error {
	codes := make([]string, len(rows))
	names := make([]string, len(rows))
	descriptions := make([]string, len(rows))
	units := make([]string, len(rows))
	familyIDs := make([]uuid.UUID, len(rows))
	subgroupIDs := make([]string, len(rows))
	prices := make([]string, len(rows))
	minPrices := make([]string, len(rows))
	active := make([]bool, len(rows))
	for i, row := range rows {
		codes[i] = row.Code
		names[i] = row.Name
		descriptions[i] = row.Description
		units[i] = row.Unit
		prices[i] = row.Price
		active[i] = row.IsActive
		familyIDs[i] = row.FamilyID
		if row.SubgroupID != nil {
			subgroupIDs[i] = row.SubgroupID.String()
		}
		if row.MinPrice != nil {
			minPrices[i] = *row.MinPrice
		}
	}

	var imported int
	err := q.QueryRow(ctx,
		`WITH incoming AS (
		   SELECT code, name, description, unit, family_id, subgroup_id, price, min_price, is_active
		   FROM unnest(
		     $5::text[], $6::text[], $7::text[], $8::text[], $9::uuid[],
		     $10::text[], $11::text[], $12::text[], $13::boolean[]
		   ) AS u(code, name, description, unit, family_id, subgroup_id, price, min_price, is_active)
		 ), upserted_products AS (
		   INSERT INTO product
		     (account_id, code, canonical_name, description, unit, family_id, subgroup_id, is_active)
		   SELECT $1, code, name, NULLIF(description, ''), unit, family_id,
		          NULLIF(subgroup_id, '')::uuid, is_active
		   FROM incoming
		   ON CONFLICT (account_id, code) WHERE code IS NOT NULL DO UPDATE
		   SET canonical_name = EXCLUDED.canonical_name,
		       description = EXCLUDED.description,
		       unit = EXCLUDED.unit,
		       family_id = EXCLUDED.family_id,
		       subgroup_id = EXCLUDED.subgroup_id,
		       is_active = EXCLUDED.is_active
		   RETURNING id, code, is_active
		 ), upserted_availability AS (
		   INSERT INTO branch_product (account_id, branch_id, product_id, stock, is_active)
		   SELECT $1, $2, id, NULL, is_active
		   FROM upserted_products
		   ON CONFLICT (branch_id, product_id) DO UPDATE
		   SET is_active = EXCLUDED.is_active
		   RETURNING product_id
		 ), prices_to_change AS (
		   SELECT p.id AS product_id, i.price, i.min_price
		   FROM upserted_products p
		   JOIN incoming i ON i.code = p.code
		   LEFT JOIN LATERAL (
		     SELECT price, min_price
		     FROM product_price
		     WHERE account_id = $1 AND branch_id = $2 AND product_id = p.id
		       AND valid_from <= $4 AND (valid_to IS NULL OR valid_to > $4)
		     ORDER BY valid_from DESC
		     LIMIT 1
		   ) current_price ON TRUE
		   WHERE i.price <> ''
		     AND (current_price.price IS DISTINCT FROM NULLIF(i.price, '')::numeric
		          OR current_price.min_price IS DISTINCT FROM NULLIF(i.min_price, '')::numeric)
		 ), closed_prices AS (
		   UPDATE product_price pp
		   SET valid_to = $4
		   FROM prices_to_change c
		   WHERE pp.account_id = $1 AND pp.branch_id = $2 AND pp.product_id = c.product_id
		     AND pp.valid_from <= $4 AND (pp.valid_to IS NULL OR pp.valid_to > $4)
		   RETURNING pp.product_id
		 ), inserted_prices AS (
		   INSERT INTO product_price
		     (account_id, branch_id, product_id, user_id, price, currency, min_price, valid_from)
		   SELECT $1, $2, c.product_id, $3, c.price::numeric, 'ARS',
		          NULLIF(c.min_price, '')::numeric, $4
		   FROM prices_to_change c
		   JOIN upserted_availability a ON a.product_id = c.product_id
		   LEFT JOIN (SELECT DISTINCT product_id FROM closed_prices) closed
		     ON closed.product_id = c.product_id
		   RETURNING product_id
		 )
		 SELECT count(*) FROM upserted_products`,
		tenant.AccountID, tenant.BranchID, tenant.UserID, effectiveAt, codes, names,
		descriptions, units, familyIDs, subgroupIDs, prices, minPrices, active,
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
