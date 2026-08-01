package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// BranchRepository owns persistence for branch.
type BranchRepository struct{}

// NewBranchRepository builds a BranchRepository.
func NewBranchRepository() *BranchRepository {
	return &BranchRepository{}
}

// IsAccessibleBy reports whether the branch exists in the account, is active, and the user
// may operate on it. A seller needs a user_branch row; an admin skips that check.
//
// It is the only thing standing between a caller and another branch's data: row level
// security guards the account boundary, not the branch one.
func (r *BranchRepository) IsAccessibleBy(
	ctx context.Context, q Querier, accountID, userID, branchID uuid.UUID, isAdmin bool,
) (bool, error) {
	var accessible bool
	err := q.QueryRow(ctx,
		`SELECT EXISTS (
		   SELECT 1
		   FROM branch b
		   LEFT JOIN user_branch ub ON ub.branch_id = b.id AND ub.user_id = $3
		   WHERE b.id = $2
		     AND b.account_id = $1
		     AND b.is_active = TRUE
		     AND ($4 OR ub.id IS NOT NULL)
		 )`,
		accountID, branchID, userID, isAdmin,
	).Scan(&accessible)
	if err != nil {
		return false, err
	}
	return accessible, nil
}

// ListForUser returns the branches a user may operate on, so the frontend can render a
// branch switcher without guessing. Admins get every active branch in the account.
func (r *BranchRepository) ListForUser(
	ctx context.Context, q Querier, accountID, userID uuid.UUID, isAdmin bool,
) ([]domain.Branch, error) {
	rows, err := q.Query(ctx,
		`SELECT b.id, b.account_id, b.name, b.address, b.default_expiry_days, b.is_active,
		        b.created_at, b.updated_at
		 FROM branch b
		 LEFT JOIN user_branch ub ON ub.branch_id = b.id AND ub.user_id = $2
		 WHERE b.account_id = $1
		   AND b.is_active = TRUE
		   AND ($3 OR ub.id IS NOT NULL)
		 ORDER BY b.name`,
		accountID, userID, isAdmin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var branches []domain.Branch
	for rows.Next() {
		var b domain.Branch
		if err := rows.Scan(&b.ID, &b.AccountID, &b.Name, &b.Address, &b.DefaultExpiryDays,
			&b.IsActive, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}
		branches = append(branches, b)
	}
	return branches, rows.Err()
}
