package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// ListPriceSelectionCandidates expands matched products to every active product in their subgroup.
func (r *ProductRepository) ListPriceSelectionCandidates(ctx context.Context, q Querier,
	accountID, branchID uuid.UUID, productIDs []uuid.UUID,
) (map[uuid.UUID][]domain.RFQPriceCandidate, error) {
	groups := make(map[uuid.UUID][]domain.RFQPriceCandidate)
	if len(productIDs) == 0 {
		return groups, nil
	}
	rows, err := q.Query(ctx, `
		SELECT anchor.id, p.id, p.account_id, p.canonical_name, p.description, p.unit,
		       p.family_id, p.subgroup_id, p.is_active,
		       ARRAY(SELECT ps.term FROM product_synonym ps
		             WHERE ps.account_id = $1 AND ps.product_id = p.id ORDER BY ps.term)
		FROM product anchor
		JOIN branch_product abp ON abp.account_id = $1 AND abp.branch_id = $2
		  AND abp.product_id = anchor.id AND abp.is_active = TRUE
		JOIN product p ON p.account_id = $1
		  AND p.family_id = anchor.family_id AND p.subgroup_id = anchor.subgroup_id
		  AND p.is_active = TRUE
		JOIN branch_product bp ON bp.account_id = $1 AND bp.branch_id = $2
		  AND bp.product_id = p.id AND bp.is_active = TRUE
		WHERE anchor.account_id = $1 AND anchor.id = ANY($3::uuid[]) AND anchor.is_active = TRUE
		ORDER BY anchor.id, p.canonical_name, p.id`, accountID, branchID, productIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var anchor uuid.UUID
		var candidate domain.RFQPriceCandidate
		p := &candidate.Product
		if err := rows.Scan(&anchor, &p.ID, &p.AccountID, &p.CanonicalName, &p.Description,
			&p.Unit, &p.FamilyID, &p.SubgroupID, &p.IsActive, &candidate.Synonyms); err != nil {
			return nil, err
		}
		groups[anchor] = append(groups[anchor], candidate)
	}
	return groups, rows.Err()
}
