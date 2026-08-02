package repository

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// ProductPriceRepository owns product-price import persistence.
type ProductPriceRepository struct{}

// NewProductPriceRepository builds a ProductPriceRepository.
func NewProductPriceRepository() *ProductPriceRepository {
	return &ProductPriceRepository{}
}

// ListCurrentForExport loads every current price for the branch in code order.
func (r *ProductPriceRepository) ListCurrentForExport(
	ctx context.Context, q Querier, accountID, branchID uuid.UUID,
) (*domain.ProductPriceExport, error) {
	rows, err := q.Query(ctx,
		`SELECT b.name, p.code, p.canonical_name, pp.price::text, pp.min_price::text,
		        pp.currency, pp.conditions
		 FROM branch b
		 JOIN branch_product bp
		   ON bp.account_id = b.account_id
		  AND bp.branch_id = b.id
		  AND bp.is_active = TRUE
		 JOIN product p
		   ON p.account_id = b.account_id
		  AND p.id = bp.product_id
		  AND p.is_active = TRUE
		  AND p.code IS NOT NULL
		 JOIN LATERAL (
		   SELECT price, min_price, currency, conditions
		   FROM product_price
		   WHERE account_id = $1
		     AND branch_id = $2
		     AND product_id = p.id
		     AND valid_from <= now()
		     AND (valid_to IS NULL OR valid_to > now())
		   ORDER BY valid_from DESC
		   LIMIT 1
		 ) pp ON TRUE
		 WHERE b.account_id = $1
		   AND b.id = $2
		   AND b.is_active = TRUE
		 ORDER BY p.code`,
		accountID, branchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	export := &domain.ProductPriceExport{}
	for rows.Next() {
		var row domain.ProductPriceExportRow
		if err := rows.Scan(&export.BranchName, &row.Code, &row.ProductName, &row.Price,
			&row.MinPrice, &row.Currency, &row.Conditions); err != nil {
			return nil, err
		}
		export.Rows = append(export.Rows, row)
	}
	return export, rows.Err()
}

// GetByCodes loads active catalog products and their current branch prices in one query.
func (r *ProductPriceRepository) GetByCodes(
	ctx context.Context, q Querier, accountID, branchID uuid.UUID, codes []string,
) (map[string]domain.ProductPriceLookup, error) {
	rows, err := q.Query(ctx,
		`SELECT p.id, p.code, p.canonical_name,
		        pp.price::text, pp.min_price::text, pp.currency, pp.conditions
		 FROM product p
		 JOIN branch_product bp
		   ON bp.account_id = $1
		  AND bp.branch_id = $2
		  AND bp.product_id = p.id
		  AND bp.is_active = TRUE
		 LEFT JOIN LATERAL (
		   SELECT price, min_price, currency, conditions
		   FROM product_price
		   WHERE account_id = $1
		     AND branch_id = $2
		     AND product_id = p.id
		     AND valid_from <= now()
		     AND (valid_to IS NULL OR valid_to > now())
		   ORDER BY valid_from DESC
		   LIMIT 1
		 ) pp ON TRUE
		 WHERE p.account_id = $1
		   AND p.code = ANY($3)
		   AND p.is_active = TRUE`,
		accountID, branchID, codes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	products := make(map[string]domain.ProductPriceLookup, len(codes))
	for rows.Next() {
		var product domain.ProductPriceLookup
		if err := rows.Scan(&product.ProductID, &product.Code, &product.ProductName,
			&product.CurrentPrice, &product.CurrentMinPrice, &product.CurrentCurrency,
			&product.CurrentConditions); err != nil {
			return nil, err
		}
		products[product.Code] = product
	}
	return products, rows.Err()
}

// ApplyImport closes current prices and inserts the replacement versions in bulk.
func (r *ProductPriceRepository) ApplyImport(
	ctx context.Context, q Querier, tenant domain.Tenant, effectiveAt time.Time,
	updates []domain.ProductPriceUpdate,
) error {
	productIDs := make([]uuid.UUID, len(updates))
	prices := make([]string, len(updates))
	minPrices := make([]string, len(updates))
	currencies := make([]string, len(updates))
	conditions := make([]string, len(updates))
	for i, update := range updates {
		productIDs[i] = update.ProductID
		prices[i] = update.Price
		currencies[i] = update.Currency
		if update.MinPrice != nil {
			minPrices[i] = *update.MinPrice
		}
		if update.Conditions != nil {
			conditions[i] = *update.Conditions
		}
	}
	lockedRows, err := q.Query(ctx,
		`SELECT p.id
		 FROM product p
		 JOIN branch_product bp
		   ON bp.account_id = $1
		  AND bp.branch_id = $2
		  AND bp.product_id = p.id
		  AND bp.is_active = TRUE
		 WHERE p.account_id = $1
		   AND p.id = ANY($3)
		   AND p.is_active = TRUE
		 ORDER BY p.id
		 FOR UPDATE OF p, bp`,
		tenant.AccountID, tenant.BranchID, productIDs)
	if err != nil {
		return err
	}
	locked := 0
	for lockedRows.Next() {
		var productID uuid.UUID
		if err := lockedRows.Scan(&productID); err != nil {
			lockedRows.Close()
			return err
		}
		locked++
	}
	if err := lockedRows.Err(); err != nil {
		lockedRows.Close()
		return err
	}
	lockedRows.Close()
	if locked != len(productIDs) {
		return domain.ErrNotFound
	}

	if _, err := q.Exec(ctx,
		`UPDATE product_price
		 SET valid_to = $4
		 WHERE account_id = $1
		   AND branch_id = $2
		   AND product_id = ANY($3)
		   AND valid_from <= $4
		   AND (valid_to IS NULL OR valid_to > $4)`,
		tenant.AccountID, tenant.BranchID, productIDs, effectiveAt); err != nil {
		return err
	}

	_, err = q.Exec(ctx,
		`WITH incoming AS (
		   SELECT *
		   FROM unnest($4::uuid[], $5::text[], $6::text[], $7::text[], $8::text[])
		     AS u(product_id, price, min_price, currency, conditions)
		 )
		 INSERT INTO product_price
		   (account_id, branch_id, product_id, user_id, price, min_price, currency,
		    conditions, valid_from, valid_to)
		 SELECT $1, $2, i.product_id, $3, i.price::numeric,
		        NULLIF(i.min_price, '')::numeric, i.currency, NULLIF(i.conditions, ''), $9,
		        (SELECT MIN(pp.valid_from)
		         FROM product_price pp
		         WHERE pp.account_id = $1
		           AND pp.branch_id = $2
		           AND pp.product_id = i.product_id
		           AND pp.valid_from > $9)
		 FROM incoming i`,
		tenant.AccountID, tenant.BranchID, tenant.UserID, productIDs, prices, minPrices,
		currencies, conditions, effectiveAt)
	return err
}
