package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// DeleteRecommendationAlternative removes a commercial recommendation from a mutable version.
func (r *QuoteRepository) DeleteRecommendationAlternative(
	ctx context.Context, q Querier, accountID, versionID, itemID, alternativeID uuid.UUID,
) error {
	tag, err := q.Exec(ctx, `DELETE FROM quote_item_alternative alternative
		USING quote_item item, quote_version version
		WHERE alternative.account_id = $1 AND alternative.id = $4
		  AND alternative.recommendation_type IN ('ECONOMY', 'PREMIUM')
		  AND item.account_id = $1 AND item.id = $3 AND item.version_id = $2
		  AND alternative.quote_item_id = item.id
		  AND version.account_id = $1 AND version.id = $2 AND version.is_immutable = FALSE`,
		accountID, versionID, itemID, alternativeID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrNotFound
	}
	return nil
}
