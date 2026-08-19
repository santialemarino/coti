package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// productPriceColumns keeps the SELECT list, the scan order, and the struct in one place.
const productPriceColumns = `id, account_id, branch_id, product_id, user_id, price, currency,
	min_price, valid_from, valid_to, created_at`

// priceInForce is the one definition of the price that applies right now: a period that has
// started and has not ended. Every query that reads a current price uses this rather than
// spelling the dates out again, so the definition cannot drift between them.
const priceInForce = `valid_from <= now() AND (valid_to IS NULL OR valid_to > now())`

// ProductPriceRepository owns persistence for product_price.
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
		`SELECT b.name, p.code, p.canonical_name, pp.price::text, pp.min_price::text
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
		   SELECT price, min_price
		   FROM product_price
		   WHERE account_id = $1
		     AND branch_id = $2
		     AND product_id = p.id
		     AND `+priceInForce+`
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
			&row.MinPrice); err != nil {
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
		        pp.price::text, pp.min_price::text, pp.currency
		 FROM product p
		 JOIN branch_product bp
		   ON bp.account_id = $1
		  AND bp.branch_id = $2
		  AND bp.product_id = p.id
		  AND bp.is_active = TRUE
		 LEFT JOIN LATERAL (
		   SELECT price, min_price, currency
		   FROM product_price
		   WHERE account_id = $1
		     AND branch_id = $2
		     AND product_id = p.id
		     AND `+priceInForce+`
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
			&product.CurrentPrice, &product.CurrentMinPrice, &product.CurrentCurrency); err != nil {
			return nil, err
		}
		products[product.Code] = product
	}
	return products, rows.Err()
}

// GetCurrentByProductIDs loads the price in force at one branch for each product asked for, keyed
// by product id. A product the branch cannot sell is absent from the map rather than priced at zero.
func (r *ProductPriceRepository) GetCurrentByProductIDs(
	ctx context.Context, q Querier, accountID, branchID uuid.UUID, productIDs []uuid.UUID,
) (map[uuid.UUID]domain.BranchPrice, error) {
	rows, err := q.Query(ctx,
		`SELECT DISTINCT ON (p.id) p.id, pp.price, pp.min_price
		 FROM product p
		 JOIN branch_product bp
		   ON bp.account_id = $1
		  AND bp.branch_id = $2
		  AND bp.product_id = p.id
		  AND bp.is_active = TRUE
		 JOIN product_price pp
		   ON pp.account_id = $1
		  AND pp.branch_id = $2
		  AND pp.product_id = p.id
		  AND `+priceInForce+`
		 WHERE p.account_id = $1
		   AND p.id = ANY($3)
		   AND p.is_active = TRUE
		 ORDER BY p.id, pp.valid_from DESC`,
		accountID, branchID, productIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	prices := make(map[uuid.UUID]domain.BranchPrice, len(productIDs))
	for rows.Next() {
		var price domain.BranchPrice
		if err := rows.Scan(&price.ProductID, &price.Price, &price.MinPrice); err != nil {
			return nil, err
		}
		prices[price.ProductID] = price
	}
	return prices, rows.Err()
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
	for i, update := range updates {
		productIDs[i] = update.ProductID
		prices[i] = update.Price
		currencies[i] = update.Currency
		if update.MinPrice != nil {
			minPrices[i] = *update.MinPrice
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
		   FROM unnest($4::uuid[], $5::text[], $6::text[], $7::text[])
		     AS u(product_id, price, min_price, currency)
		 )
		 INSERT INTO product_price
		   (account_id, branch_id, product_id, user_id, price, min_price, currency,
		    valid_from, valid_to)
		 SELECT $1, $2, i.product_id, $3, i.price::numeric,
		        NULLIF(i.min_price, '')::numeric, i.currency, $8,
		        (SELECT MIN(pp.valid_from)
		         FROM product_price pp
		         WHERE pp.account_id = $1
		           AND pp.branch_id = $2
		           AND pp.product_id = i.product_id
		           AND pp.valid_from > $8)
		 FROM incoming i`,
		tenant.AccountID, tenant.BranchID, tenant.UserID, productIDs, prices, minPrices,
		currencies, effectiveAt)
	return err
}

// ListByProduct returns the product's price history, grouped by branch and newest period
// first. A nil branchIDs reads every branch of the account; a set one narrows to it, and an
// empty one reads nothing.
func (r *ProductPriceRepository) ListByProduct(
	ctx context.Context, q Querier, accountID, productID uuid.UUID, branchIDs []uuid.UUID,
) ([]domain.ProductPrice, error) {
	rows, err := q.Query(ctx,
		`SELECT `+productPriceColumns+`
		 FROM product_price
		 WHERE account_id = $1 AND product_id = $2
		   AND ($3::uuid[] IS NULL OR branch_id = ANY($3::uuid[]))
		 ORDER BY branch_id, valid_from DESC, created_at DESC`,
		accountID, productID, branchIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var prices []domain.ProductPrice
	for rows.Next() {
		p, scanErr := scanProductPrice(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		prices = append(prices, *p)
	}
	return prices, rows.Err()
}

// GetOpenPeriod loads the price in force for a product at a branch — the row with no
// valid_to. Returns domain.ErrNotFound when the branch has never priced the product.
func (r *ProductPriceRepository) GetOpenPeriod(
	ctx context.Context, q Querier, accountID, branchID, productID uuid.UUID,
) (*domain.ProductPrice, error) {
	p, err := scanProductPrice(q.QueryRow(ctx,
		`SELECT `+productPriceColumns+`
		 FROM product_price
		 WHERE account_id = $1 AND branch_id = $2 AND product_id = $3 AND valid_to IS NULL
		 ORDER BY valid_from DESC
		 LIMIT 1`,
		accountID, branchID, productID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return p, err
}

// Create opens a new price period. The caller closes the previous one first, in the same
// transaction.
func (r *ProductPriceRepository) Create(
	ctx context.Context, q Querier, accountID, branchID, productID uuid.UUID, userID *uuid.UUID,
	in domain.NewProductPrice,
) (*domain.ProductPrice, error) {
	return scanProductPrice(q.QueryRow(ctx,
		`INSERT INTO product_price
		   (account_id, branch_id, product_id, user_id, price, currency, min_price, valid_from)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING `+productPriceColumns,
		accountID, branchID, productID, userID, in.Price, in.Currency, in.MinPrice, in.ValidFrom))
}

// CloseOpenPeriod stamps valid_to on the open period, so the price stops applying at the
// moment the next one starts. Returns the number of periods it closed.
func (r *ProductPriceRepository) CloseOpenPeriod(
	ctx context.Context, q Querier, accountID, branchID, productID uuid.UUID, at time.Time,
) (int64, error) {
	tag, err := q.Exec(ctx,
		`UPDATE product_price SET valid_to = $4
		 WHERE account_id = $1 AND branch_id = $2 AND product_id = $3 AND valid_to IS NULL`,
		accountID, branchID, productID, at)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// scanRow is the read surface pgx.Row and pgx.Rows share, so one scan helper serves both
// the single-row and the list queries.
type scanRow interface {
	Scan(dest ...any) error
}

func scanProductPrice(row scanRow) (*domain.ProductPrice, error) {
	var p domain.ProductPrice
	err := row.Scan(&p.ID, &p.AccountID, &p.BranchID, &p.ProductID, &p.UserID, &p.Price,
		&p.Currency, &p.MinPrice, &p.ValidFrom, &p.ValidTo, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}
