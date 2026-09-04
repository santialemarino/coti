package repository

import (
	"context"

	"github.com/google/uuid"
)

// UserBranchRepository owns persistence for user_branch, the seller-to-branch assignment.
type UserBranchRepository struct{}

// NewUserBranchRepository builds a UserBranchRepository.
func NewUserBranchRepository() *UserBranchRepository {
	return &UserBranchRepository{}
}

// ListByUsers returns each user's assigned branch ids in one query, so listing users never
// walks the table per row.
func (r *UserBranchRepository) ListByUsers(
	ctx context.Context, q Querier, accountID uuid.UUID, userIDs []uuid.UUID,
) (map[uuid.UUID][]uuid.UUID, error) {
	rows, err := q.Query(ctx,
		`SELECT user_id, branch_id
		 FROM user_branch
		 WHERE account_id = $1 AND user_id = ANY($2::uuid[])
		 ORDER BY user_id, branch_id`,
		accountID, userIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byUser := make(map[uuid.UUID][]uuid.UUID, len(userIDs))
	for rows.Next() {
		var userID, branchID uuid.UUID
		if err := rows.Scan(&userID, &branchID); err != nil {
			return nil, err
		}
		byUser[userID] = append(byUser[userID], branchID)
	}
	return byUser, rows.Err()
}

// Replace makes branchIDs the user's whole assignment set: rows outside it go, missing ones
// arrive. Two statements rather than a per-branch loop, and an empty set clears the user.
func (r *UserBranchRepository) Replace(
	ctx context.Context, q Querier, accountID, userID uuid.UUID, branchIDs []uuid.UUID,
) error {
	// COALESCE, because `<> ALL(NULL)` is NULL: a nil set would delete nothing and leave the
	// old assignments in place instead of clearing them.
	if _, err := q.Exec(ctx,
		`DELETE FROM user_branch
		 WHERE account_id = $1 AND user_id = $2
		   AND branch_id <> ALL(COALESCE($3::uuid[], '{}'::uuid[]))`,
		accountID, userID, branchIDs); err != nil {
		return err
	}

	_, err := q.Exec(ctx,
		`INSERT INTO user_branch (account_id, user_id, branch_id)
		 SELECT $1, $2, x FROM unnest($3::uuid[]) AS x
		 ON CONFLICT (user_id, branch_id) DO NOTHING`,
		accountID, userID, branchIDs)
	return err
}
