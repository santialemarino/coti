package repository

import (
	"context"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// GetSiteCrossAccount resolves a registered hostname without exposing private account data.
func (r *PublicRFQRepository) GetSiteCrossAccount(ctx context.Context, q Querier, hostname string) (*domain.PublicRFQSite, error) {
	rows, err := q.Query(ctx, `SELECT a.name, a.brand_logo_url, b.name, c.id
 FROM account a
 JOIN branch b ON b.account_id = a.id AND b.is_active
 JOIN channel c ON c.account_id = a.id AND c.branch_id = b.id AND c.type = 'WEBAPP' AND c.is_active
 WHERE a.public_hostname = $1 AND a.is_active ORDER BY lower(b.name), b.id`, hostname)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := &domain.PublicRFQSite{Branches: []domain.PublicRFQSiteBranch{}}
	for rows.Next() {
		var branch domain.PublicRFQSiteBranch
		if err := rows.Scan(&result.SupplierName, &result.LogoURL, &branch.Name, &branch.ChannelID); err != nil {
			return nil, err
		}
		result.Branches = append(result.Branches, branch)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result.Branches) == 0 {
		return nil, domain.ErrNotFound
	}
	return result, nil
}
