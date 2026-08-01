package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// branchProductColumns keeps the SELECT list, the scan order, and the struct in one place.
const branchProductColumns = `id, account_id, branch_id, product_id, stock, is_active,
	created_at, updated_at`

// BranchProductRepository owns persistence for branch_product.
type BranchProductRepository struct{}

// NewBranchProductRepository builds a BranchProductRepository.
func NewBranchProductRepository() *BranchProductRepository {
	return &BranchProductRepository{}
}

// ListByProduct returns the product's availability rows. A nil branchIDs reads every branch
// of the account; a set one narrows to it, and an empty one reads nothing.
func (r *BranchProductRepository) ListByProduct(
	ctx context.Context, q Querier, accountID, productID uuid.UUID, branchIDs []uuid.UUID,
) ([]domain.BranchProduct, error) {
	rows, err := q.Query(ctx,
		`SELECT `+branchProductColumns+`
		 FROM branch_product
		 WHERE account_id = $1 AND product_id = $2
		   AND ($3::uuid[] IS NULL OR branch_id = ANY($3::uuid[]))
		 ORDER BY branch_id`,
		accountID, productID, branchIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var availability []domain.BranchProduct
	for rows.Next() {
		var bp domain.BranchProduct
		if err := rows.Scan(&bp.ID, &bp.AccountID, &bp.BranchID, &bp.ProductID, &bp.Stock,
			&bp.IsActive, &bp.CreatedAt, &bp.UpdatedAt); err != nil {
			return nil, err
		}
		availability = append(availability, bp)
	}
	return availability, rows.Err()
}

// Save sets whether a branch carries the product and with how much stock, upserting on
// uq_branch_product so the caller never has to know whether the row exists.
func (r *BranchProductRepository) Save(
	ctx context.Context, q Querier, accountID, branchID, productID uuid.UUID,
	in domain.BranchAvailability,
) (*domain.BranchProduct, error) {
	var bp domain.BranchProduct
	err := q.QueryRow(ctx,
		`INSERT INTO branch_product (account_id, branch_id, product_id, stock, is_active)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (branch_id, product_id) DO UPDATE
		   SET stock = EXCLUDED.stock, is_active = EXCLUDED.is_active
		 RETURNING `+branchProductColumns,
		accountID, branchID, productID, in.Stock, in.IsActive,
	).Scan(&bp.ID, &bp.AccountID, &bp.BranchID, &bp.ProductID, &bp.Stock, &bp.IsActive,
		&bp.CreatedAt, &bp.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &bp, nil
}
