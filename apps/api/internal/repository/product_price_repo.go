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
	conditions, min_price, valid_from, valid_to, created_at`

// ProductPriceRepository owns persistence for product_price.
type ProductPriceRepository struct{}

// NewProductPriceRepository builds a ProductPriceRepository.
func NewProductPriceRepository() *ProductPriceRepository {
	return &ProductPriceRepository{}
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
		   (account_id, branch_id, product_id, user_id, price, currency, conditions,
		    min_price, valid_from)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 RETURNING `+productPriceColumns,
		accountID, branchID, productID, userID, in.Price, in.Currency, in.Conditions,
		in.MinPrice, in.ValidFrom))
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
		&p.Currency, &p.Conditions, &p.MinPrice, &p.ValidFrom, &p.ValidTo, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}
