package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// GetReadiness derives setup from current data without storing completion flags.
func (r *OnboardingRepository) GetReadiness(ctx context.Context, q Querier, accountID uuid.UUID, branchIDs []uuid.UUID) (*domain.OnboardingReadiness, error) {
	out := &domain.OnboardingReadiness{Branches: []domain.BranchReadiness{}}
	if err := q.QueryRow(ctx, `SELECT is_active AND length(trim(name)) > 0 FROM account WHERE id=$1`, accountID).Scan(&out.AccountReady); err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `SELECT b.id,b.name,
 count(p.id),count(p.id) FILTER (WHERE NOT EXISTS (
 SELECT 1 FROM product_price pp WHERE pp.account_id=$1 AND pp.branch_id=b.id AND pp.product_id=p.id AND pp.price>=0 AND `+priceInForce+`)),
 EXISTS (SELECT 1 FROM app_user u WHERE u.account_id=$1 AND u.is_active AND
 (u.role='ADMIN' OR EXISTS (SELECT 1 FROM user_branch ub WHERE ub.account_id=$1 AND ub.user_id=u.id AND ub.branch_id=b.id))),
 EXISTS (SELECT 1 FROM channel c WHERE c.account_id=$1 AND c.branch_id=b.id AND c.is_active AND c.type IN ('EMAIL','WHATSAPP','WEBAPP'))
 FROM branch b
 LEFT JOIN branch_product bp ON bp.account_id=$1 AND bp.branch_id=b.id AND bp.is_active
 LEFT JOIN product p ON p.account_id=$1 AND p.id=bp.product_id AND p.is_active
 WHERE b.account_id=$1 AND b.is_active AND ($2::uuid[] IS NULL OR b.id=ANY($2::uuid[]))
 GROUP BY b.id,b.name ORDER BY b.name,b.id`, accountID, branchIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var branch domain.BranchReadiness
		if err := rows.Scan(&branch.ID, &branch.Name, &branch.Products, &branch.MissingPrices, &branch.HasStaff, &branch.HasChannel); err != nil {
			return nil, err
		}
		out.Branches = append(out.Branches, branch)
	}
	return out, rows.Err()
}
