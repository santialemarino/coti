package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// PromoteMatcherRecommendation labels one unapproved matcher candidate without duplicating it.
func (r *QuoteRepository) PromoteMatcherRecommendation(ctx context.Context, q Querier,
	accountID, versionID, itemID, alternativeID, productID uuid.UUID, kind domain.ProductAlternativeType,
	price, minPrice decimal.NullDecimal,
) (*domain.QuoteItemAlternative, error) {
	row := q.QueryRow(ctx, `WITH changed AS (
		UPDATE quote_item_alternative alternative
		SET recommendation_type = $6::product_alternative_type,
		    price_snapshot = $7, min_price_snapshot = $8
		FROM quote_item item, quote_version version
		WHERE alternative.account_id = $1 AND alternative.id = $4
		  AND alternative.quote_item_id = $3 AND alternative.product_id = $5
		  AND alternative.type = 'PRODUCT' AND alternative.origin = 'AI'
		  AND alternative.recommendation_type IS NULL
		  AND alternative.approved_by_seller = FALSE AND alternative.chosen_by_client = FALSE
		  AND item.account_id = $1 AND item.id = $3 AND item.version_id = $2
		  AND version.account_id = $1 AND version.id = $2 AND version.is_immutable = FALSE
		  AND NOT EXISTS (SELECT 1 FROM quote_item_alternative occupied
		    WHERE occupied.account_id = $1 AND occupied.quote_item_id = $3
		      AND occupied.recommendation_type = $6::product_alternative_type)
		RETURNING alternative.*
	)
	SELECT changed.id, changed.account_id, changed.quote_item_id, changed.product_id,
	       changed.combo_id, changed.type, changed.recommendation_type, changed.origin, changed.rank,
	       changed.confidence_score, changed.price_snapshot, changed.min_price_snapshot,
	       changed.approved_by_seller, changed.chosen_by_client, changed.created_at,
	       product.code, product.canonical_name, product.unit
	FROM changed
	LEFT JOIN product ON product.account_id = changed.account_id AND product.id = changed.product_id`,
		accountID, versionID, itemID, alternativeID, productID, kind, price, minPrice)
	return scanQuoteItemAlternative(row)
}
