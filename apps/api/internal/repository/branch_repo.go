package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

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

// ListAllForAccount returns every branch of the account, closed ones included, for
// administration. It takes no user id on purpose: a caller's reach is always the active branches
// they are assigned, so a read that ignores both cannot be mistaken for one.
func (r *BranchRepository) ListAllForAccount(
	ctx context.Context, q Querier, accountID uuid.UUID,
) ([]domain.Branch, error) {
	rows, err := q.Query(ctx,
		`SELECT id, account_id, name, address, default_expiry_days, is_active,
		        created_at, updated_at
		 FROM branch
		 WHERE account_id = $1
		 ORDER BY is_active DESC, name`,
		accountID)
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

// ListIDsForUser returns the ids of the branches a user may operate on. It backs the
// per-request branch scope, which needs no other column.
func (r *BranchRepository) ListIDsForUser(
	ctx context.Context, q Querier, accountID, userID uuid.UUID, isAdmin bool,
) ([]uuid.UUID, error) {
	rows, err := q.Query(ctx,
		`SELECT b.id
		 FROM branch b
		 LEFT JOIN user_branch ub ON ub.branch_id = b.id AND ub.user_id = $2
		 WHERE b.account_id = $1
		   AND b.is_active = TRUE
		   AND ($3 OR ub.id IS NOT NULL)
		 ORDER BY b.id`,
		accountID, userID, isAdmin)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Non-nil even when empty: an empty scope means "no branches", not "every branch".
	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// GetByID returns one branch of the account, whether or not it is active.
func (r *BranchRepository) GetByID(
	ctx context.Context, q Querier, accountID, branchID uuid.UUID,
) (*domain.Branch, error) {
	branch, err := scanBranch(q.QueryRow(ctx,
		`SELECT id, account_id, name, address, default_expiry_days, is_active,
		        created_at, updated_at
		 FROM branch
		 WHERE account_id = $1 AND id = $2`,
		accountID, branchID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return branch, nil
}

// CountActiveExcluding counts the account's active branches, ignoring one. It answers whether
// closing that one would leave the account with nowhere to operate.
func (r *BranchRepository) CountActiveExcluding(
	ctx context.Context, q Querier, accountID, branchID uuid.UUID,
) (int, error) {
	var count int
	err := q.QueryRow(ctx,
		`SELECT count(*) FROM branch
		 WHERE account_id = $1 AND is_active = TRUE AND id <> $2`,
		accountID, branchID,
	).Scan(&count)
	return count, err
}

// Create opens a branch under the account.
func (r *BranchRepository) Create(
	ctx context.Context, q Querier, accountID uuid.UUID, in domain.NewBranch,
) (*domain.Branch, error) {
	return scanBranch(q.QueryRow(ctx,
		`INSERT INTO branch (account_id, name, address, default_expiry_days)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id, account_id, name, address, default_expiry_days, is_active,
		           created_at, updated_at`,
		accountID, in.Name, in.Address, in.DefaultExpiryDays))
}

// Update replaces the branch's editable fields, leaving is_active alone when isActive is nil.
func (r *BranchRepository) Update(
	ctx context.Context, q Querier, accountID, branchID uuid.UUID, in domain.BranchUpdate,
) (*domain.Branch, error) {
	branch, err := scanBranch(q.QueryRow(ctx,
		`UPDATE branch
		 SET name = $3, address = $4, default_expiry_days = $5,
		     is_active = COALESCE($6, is_active)
		 WHERE account_id = $1 AND id = $2
		 RETURNING id, account_id, name, address, default_expiry_days, is_active,
		           created_at, updated_at`,
		accountID, branchID, in.Name, in.Address, in.DefaultExpiryDays, in.IsActive))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return branch, nil
}

// Deactivate closes a branch without deleting it, so the quotes and prices that reference it
// stay explainable.
func (r *BranchRepository) Deactivate(
	ctx context.Context, q Querier, accountID, branchID uuid.UUID,
) error {
	tag, err := q.Exec(ctx,
		`UPDATE branch SET is_active = FALSE WHERE account_id = $1 AND id = $2`,
		accountID, branchID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// ExistAllInAccount reports whether every given id is an active branch of the account. One
// query, and it counts distinct ids on both sides so a repeated id cannot pass for a missing
// one.
func (r *BranchRepository) ExistAllInAccount(
	ctx context.Context, q Querier, accountID uuid.UUID, ids []uuid.UUID,
) (bool, error) {
	var allPresent bool
	err := q.QueryRow(ctx,
		`SELECT (SELECT count(DISTINCT id) FROM branch
		         WHERE account_id = $1 AND is_active = TRUE AND id = ANY($2::uuid[]))
		      = (SELECT count(DISTINCT x) FROM unnest($2::uuid[]) AS x)`,
		accountID, ids,
	).Scan(&allPresent)
	if err != nil {
		return false, err
	}
	return allPresent, nil
}

func scanBranch(row pgx.Row) (*domain.Branch, error) {
	var b domain.Branch
	if err := row.Scan(&b.ID, &b.AccountID, &b.Name, &b.Address, &b.DefaultExpiryDays,
		&b.IsActive, &b.CreatedAt, &b.UpdatedAt); err != nil {
		return nil, err
	}
	return &b, nil
}
