package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// quoteDiscountRowColumns is the single-row projection the create returns. A MANUAL_SELLER
// row is typed with no promotion, so its promotion name is its own description, and the
// widened projection keeps one scan shape for every read. The insert's RETURNING names each
// column of the target table, which the join the update uses would leave ambiguous.
const quoteDiscountRowColumns = `id, account_id, quote_version_id, promotion_id,
	condition_type, scope, origin, amount, action_type, action_value, description,
	suppressed_by_seller, created_at, description AS promotion_name`

// quoteDiscountRowColumnsQd qualifies the update's RETURNING: its FROM quote_version shares
// id, account_id and created_at with quote_discount, so an unqualified reference fails.
const quoteDiscountRowColumnsQd = `qd.id, qd.account_id, qd.quote_version_id, qd.promotion_id,
	qd.condition_type, qd.scope, qd.origin, qd.amount, qd.action_type, qd.action_value,
	qd.description, qd.suppressed_by_seller, qd.created_at, qd.description AS promotion_name`

// quoteDiscountColumnsPrefixed joins promotion.name so a row can answer what it displays as;
// for a seller-typed discount with no promotion, that is its own description.
const quoteDiscountColumnsPrefixed = `qd.id, qd.account_id, qd.quote_version_id, qd.promotion_id,
	qd.condition_type, qd.scope, qd.origin, qd.amount, qd.action_type, qd.action_value,
	qd.description, qd.suppressed_by_seller, qd.created_at, COALESCE(p.name, qd.description)`

// QuoteDiscountRepository owns persistence for one discount application on a quote version.
type QuoteDiscountRepository struct{}

// NewQuoteDiscountRepository builds a QuoteDiscountRepository.
func NewQuoteDiscountRepository() *QuoteDiscountRepository {
	return &QuoteDiscountRepository{}
}

// ListByVersionID loads every discount application on a version, newest to oldest.
func (r *QuoteDiscountRepository) ListByVersionID(
	ctx context.Context, q Querier, accountID, versionID uuid.UUID,
) ([]domain.QuoteDiscount, error) {
	rows, err := q.Query(ctx,
		`SELECT `+quoteDiscountColumnsPrefixed+`
		 FROM quote_discount qd
		 LEFT JOIN promotion p ON p.account_id = qd.account_id AND p.id = qd.promotion_id
		 WHERE qd.account_id = $1 AND qd.quote_version_id = $2
		 ORDER BY qd.created_at DESC, qd.id DESC`,
		accountID, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var discounts []domain.QuoteDiscount
	for rows.Next() {
		var discount domain.QuoteDiscount
		if err := rows.Scan(&discount.ID, &discount.AccountID, &discount.QuoteVersionID,
			&discount.PromotionID, &discount.ConditionType, &discount.Scope, &discount.Origin,
			&discount.Amount, &discount.ActionType, &discount.ActionValue,
			&discount.Description, &discount.SuppressedBySeller,
			&discount.CreatedAt, &discount.PromotionName); err != nil {
			return nil, err
		}
		discounts = append(discounts, discount)
	}
	return discounts, rows.Err()
}

// Create adds a seller-typed discount to a mutable version. The amount is the money the
// caller's engine computed from the typed value; the row keeps the rule alongside it. The
// condition type is derived from the scope, mirroring how the promotion sweep types rows.
func (r *QuoteDiscountRepository) Create(
	ctx context.Context, q Querier, accountID, versionID uuid.UUID, in domain.QuoteDiscountCreate,
) (*domain.QuoteDiscount, error) {
	return scanQuoteDiscount(q.QueryRow(ctx,
		`INSERT INTO quote_discount (account_id, quote_version_id, condition_type, scope, origin,
		        amount, action_type, action_value, description)
		 SELECT $1, qv.id, CASE $3::discount_scope
		                     WHEN 'TOTAL'::discount_scope THEN 'ON_TOTAL'::promotion_condition_type
		                     WHEN 'ITEM'::discount_scope THEN 'PER_ITEM'::promotion_condition_type
		                     ELSE 'ITEM_SET'::promotion_condition_type END,
		        $3::discount_scope, 'MANUAL_SELLER'::discount_origin, $4::numeric,
		        $5::promotion_action_type, $6::numeric, $7::text
		 FROM quote_version qv
		 WHERE qv.account_id = $1 AND qv.id = $2 AND qv.is_immutable = FALSE
		 RETURNING `+quoteDiscountRowColumns,
		accountID, versionID, in.Scope, in.Amount, in.ActionType, in.Value,
		in.Description))
}

// CreateApplications writes the engine's AUTOMATIC applications to a mutable version, then the
// lines each one covers. Each application carries the id its links name.
func (r *QuoteDiscountRepository) CreateApplications(
	ctx context.Context, q Querier, accountID, versionID uuid.UUID, apps []domain.DiscountApplication,
) error {
	if len(apps) == 0 {
		return nil
	}
	type row struct {
		ID            uuid.UUID `json:"id"`
		PromotionID   uuid.UUID `json:"promotion_id"`
		ConditionType string    `json:"condition_type"`
		Scope         string    `json:"scope"`
		ActionType    string    `json:"action_type"`
		ActionValue   string    `json:"action_value"`
		Amount        string    `json:"amount"`
	}
	type link struct {
		DiscountID uuid.UUID `json:"discount_id"`
		ItemID     uuid.UUID `json:"item_id"`
	}
	rows := make([]row, 0, len(apps))
	links := make([]link, 0)
	for _, app := range apps {
		if app.ID == uuid.Nil {
			return errors.New("discount application needs a caller-chosen id")
		}
		rows = append(rows, row{ID: app.ID, PromotionID: app.PromotionID,
			ConditionType: string(app.ConditionType), Scope: string(app.Scope),
			ActionType: string(app.ActionType), ActionValue: app.ActionValue.String(),
			Amount: app.Amount.String()})
		for _, itemID := range app.ItemIDs {
			links = append(links, link{DiscountID: app.ID, ItemID: itemID})
		}
	}
	encodedRows, err := json.Marshal(rows)
	if err != nil {
		return err
	}
	tag, err := q.Exec(ctx,
		`INSERT INTO quote_discount (id, account_id, quote_version_id, promotion_id, condition_type,
		        scope, origin, amount, action_type, action_value)
		 SELECT x.id, $1, qv.id, p.id, x.condition_type::promotion_condition_type,
		        x.scope::discount_scope, 'AUTOMATIC'::discount_origin, x.amount,
		        x.action_type::promotion_action_type, x.action_value
		 FROM jsonb_to_recordset($3::jsonb) AS x(id uuid, promotion_id uuid, condition_type text,
		      scope text, action_type text, action_value numeric, amount numeric)
		 JOIN quote_version qv ON qv.account_id = $1 AND qv.id = $2 AND qv.is_immutable = FALSE
		 JOIN promotion p ON p.account_id = $1 AND p.id = x.promotion_id`,
		accountID, versionID, encodedRows)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != int64(len(rows)) {
		return domain.ErrNotFound
	}
	if len(links) == 0 {
		return nil
	}
	encodedLinks, err := json.Marshal(links)
	if err != nil {
		return err
	}
	tag, err = q.Exec(ctx,
		`INSERT INTO quote_discount_item (account_id, quote_discount_id, quote_item_id)
		 SELECT $1, x.discount_id, qi.id
		 FROM jsonb_to_recordset($3::jsonb) AS x(discount_id uuid, item_id uuid)
		 JOIN quote_item qi ON qi.account_id = $1 AND qi.version_id = $2 AND qi.id = x.item_id`,
		accountID, versionID, encodedLinks)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != int64(len(links)) {
		return domain.ErrNotFound
	}
	return nil
}

// CopySellerDecisions copies a version's seller-typed discounts and the promotions the seller
// suppressed into a draft cloned from it, each covering the draft lines copied from its own. In the
// new draft an AI adaptation is just a promotion, so it lands as AUTOMATIC.
func (r *QuoteDiscountRepository) CopySellerDecisions(
	ctx context.Context, q Querier, accountID, sourceVersionID, draftVersionID uuid.UUID,
) error {
	_, err := q.Exec(ctx,
		`WITH source AS (
		   SELECT qd.*, gen_random_uuid() AS copy_id
		   FROM quote_discount qd
		   WHERE qd.account_id = $1 AND qd.quote_version_id = $2
		     AND (qd.origin = 'MANUAL_SELLER'::discount_origin OR qd.suppressed_by_seller)
		 ), copied AS (
		   INSERT INTO quote_discount (id, account_id, quote_version_id, promotion_id,
		          condition_type, scope, origin, amount, action_type, action_value, description,
		          suppressed_by_seller)
		   SELECT source.copy_id, $1, draft.id, source.promotion_id, source.condition_type,
		          source.scope,
		          CASE WHEN source.origin = 'AI_ADAPTATION'::discount_origin
		               THEN 'AUTOMATIC'::discount_origin ELSE source.origin END,
		          source.amount, source.action_type,
		          source.action_value, source.description, source.suppressed_by_seller
		   FROM source
		   JOIN quote_version draft ON draft.account_id = $1 AND draft.id = $3
		     AND draft.is_immutable = FALSE
		   RETURNING id
		 )
		 INSERT INTO quote_discount_item (account_id, quote_discount_id, quote_item_id)
		 SELECT $1, copied.id, line.id
		 FROM copied
		 JOIN source ON source.copy_id = copied.id
		 JOIN quote_discount_item covered ON covered.account_id = $1
		   AND covered.quote_discount_id = source.id
		 JOIN quote_item line ON line.account_id = $1 AND line.version_id = $3
		   AND line.source_item_id = covered.quote_item_id`,
		accountID, sourceVersionID, draftVersionID)
	return err
}

// CreateItemLinks ties a discount to the quote lines it covers. Each item must belong to the
// discount's own version, so a link can never point at another version's row.
func (r *QuoteDiscountRepository) CreateItemLinks(
	ctx context.Context, q Querier, accountID, versionID, discountID uuid.UUID, itemIDs []uuid.UUID,
) error {
	if len(itemIDs) == 0 {
		return nil
	}
	_, err := q.Exec(ctx,
		`INSERT INTO quote_discount_item (account_id, quote_discount_id, quote_item_id)
		 SELECT $1, $2, qi.id
		 FROM quote_item qi
		 WHERE qi.account_id = $1 AND qi.version_id = $3 AND qi.id = ANY($4::uuid[])
		 ON CONFLICT (quote_discount_id, quote_item_id) DO NOTHING`,
		accountID, discountID, versionID, itemIDs)
	return err
}

// ReplaceItemLinks swaps the lines a discount covers, for an edit that changes its scope.
func (r *QuoteDiscountRepository) ReplaceItemLinks(
	ctx context.Context, q Querier, accountID, versionID, discountID uuid.UUID, itemIDs []uuid.UUID,
) error {
	if _, err := q.Exec(ctx,
		`DELETE FROM quote_discount_item
		 WHERE account_id = $1 AND quote_discount_id = $2`,
		accountID, discountID); err != nil {
		return err
	}
	return r.CreateItemLinks(ctx, q, accountID, versionID, discountID, itemIDs)
}

// ListItemIDs loads the quote lines one discount covers, in link order.
func (r *QuoteDiscountRepository) ListItemIDs(
	ctx context.Context, q Querier, accountID, discountID uuid.UUID,
) ([]uuid.UUID, error) {
	rows, err := q.Query(ctx,
		`SELECT quote_item_id FROM quote_discount_item
		 WHERE account_id = $1 AND quote_discount_id = $2
		 ORDER BY created_at, quote_item_id`,
		accountID, discountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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

// ListItemIDsByDiscountIDs loads every version discount's covered lines in one query, so the
// total recalculator never pays a round trip per percentage discount.
func (r *QuoteDiscountRepository) ListItemIDsByDiscountIDs(
	ctx context.Context, q Querier, accountID, versionID uuid.UUID, discountIDs []uuid.UUID,
) (map[uuid.UUID][]uuid.UUID, error) {
	linked := make(map[uuid.UUID][]uuid.UUID)
	if len(discountIDs) == 0 {
		return linked, nil
	}
	rows, err := q.Query(ctx,
		`SELECT qdi.quote_discount_id, qi.id
		 FROM quote_discount qd
		 JOIN quote_discount_item qdi ON qdi.account_id = qd.account_id
		                       AND qdi.quote_discount_id = qd.id
		 JOIN quote_item qi ON qi.account_id = qdi.account_id AND qi.id = qdi.quote_item_id
		 WHERE qd.account_id = $1 AND qd.quote_version_id = $2
		   AND qd.id = ANY($3::uuid[])
		 ORDER BY qdi.created_at, qi.id`,
		accountID, versionID, discountIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var discountID, itemID uuid.UUID
		if err := rows.Scan(&discountID, &itemID); err != nil {
			return nil, err
		}
		linked[discountID] = append(linked[discountID], itemID)
	}
	return linked, rows.Err()
}

// GetByID loads one discount application on a version, with its promotion name.
func (r *QuoteDiscountRepository) GetByID(
	ctx context.Context, q Querier, accountID, versionID, discountID uuid.UUID,
) (*domain.QuoteDiscount, error) {
	return scanQuoteDiscount(q.QueryRow(ctx,
		`SELECT `+quoteDiscountColumnsPrefixed+`
		 FROM quote_discount qd
		 LEFT JOIN promotion p ON p.account_id = qd.account_id AND p.id = qd.promotion_id
		 WHERE qd.account_id = $1 AND qd.quote_version_id = $2 AND qd.id = $3`,
		accountID, versionID, discountID))
}

// UpdateByID patches the rule a seller-typed discount was entered with. Only present fields
// are written, so a suppression toggle leaves the rule alone. The money amount is rewritten
// by UpdateAmount once the rule settles; this method does not expose it directly.
func (r *QuoteDiscountRepository) UpdateByID(
	ctx context.Context, q Querier, accountID, versionID, discountID uuid.UUID,
	in domain.QuoteDiscountUpdate,
) (*domain.QuoteDiscount, error) {
	return scanQuoteDiscount(q.QueryRow(ctx,
		`UPDATE quote_discount qd
		 SET action_type       = COALESCE($4::promotion_action_type, qd.action_type),
		     action_value      = COALESCE($5::numeric, qd.action_value),
		     scope             = COALESCE($6::discount_scope, qd.scope),
		     condition_type    = COALESCE($7::promotion_condition_type, qd.condition_type),
		     description       = COALESCE($8::text, qd.description),
		     suppressed_by_seller = COALESCE($9::boolean, qd.suppressed_by_seller)
		 FROM quote_version qv
		 WHERE qv.account_id = $1 AND qv.id = $2 AND qv.is_immutable = FALSE
		   AND qd.account_id = $1 AND qd.quote_version_id = qv.id AND qd.id = $3
		 RETURNING `+quoteDiscountRowColumnsQd,
		accountID, versionID, discountID,
		in.ActionType, in.Value, in.Scope, in.ConditionType, in.Description, in.SuppressedBySeller))
}

// UpdateAmount rewrites the computed money on a discount application, for a total
// recalculation that moves in step with the version's items.
func (r *QuoteDiscountRepository) UpdateAmount(
	ctx context.Context, q Querier, accountID, versionID, discountID uuid.UUID,
	amount decimal.Decimal,
) error {
	return r.mustAffect(ctx, q,
		`UPDATE quote_discount qd
		 SET amount = $4::numeric
		 FROM quote_version qv
		 WHERE qv.account_id = $1 AND qv.id = $2 AND qv.is_immutable = FALSE
		   AND qd.account_id = $1 AND qd.quote_version_id = qv.id AND qd.id = $3
		 RETURNING qd.id`,
		accountID, versionID, discountID, amount)
}

// UpdateApplication rewrites the figure and the money of an engine-earned application on a mutable
// version, for a sweep whose rule still applies but to different quantities.
func (r *QuoteDiscountRepository) UpdateApplication(
	ctx context.Context, q Querier, accountID, versionID, discountID uuid.UUID,
	actionValue, amount decimal.Decimal,
) error {
	return r.mustAffect(ctx, q,
		`UPDATE quote_discount qd
		 SET action_value = $4::numeric, amount = $5::numeric
		 FROM quote_version qv
		 WHERE qv.account_id = $1 AND qv.id = $2 AND qv.is_immutable = FALSE
		   AND qd.account_id = $1 AND qd.quote_version_id = qv.id AND qd.id = $3
		   AND qd.origin IN ('AUTOMATIC'::discount_origin, 'AI_ADAPTATION'::discount_origin)
		 RETURNING qd.id`,
		accountID, versionID, discountID, actionValue, amount)
}

// DeleteByID removes a discount application and the line links it carries. Used only for
// MANUAL_SELLER rows; an AUTOMATIC one is suppressed, never deleted.
func (r *QuoteDiscountRepository) DeleteByID(
	ctx context.Context, q Querier, accountID, versionID, discountID uuid.UUID,
) error {
	if _, err := q.Exec(ctx,
		`DELETE FROM quote_discount_item qdi
		 USING quote_discount qd, quote_version qv
		 WHERE qv.account_id = $1 AND qv.id = $2 AND qv.is_immutable = FALSE
		   AND qd.account_id = $1 AND qd.quote_version_id = qv.id AND qd.id = $3
		   AND qdi.account_id = $1 AND qdi.quote_discount_id = qd.id`,
		accountID, versionID, discountID); err != nil {
		return err
	}
	return r.mustAffect(ctx, q,
		`DELETE FROM quote_discount qd
		 USING quote_version qv
		 WHERE qv.account_id = $1 AND qv.id = $2 AND qv.is_immutable = FALSE
		   AND qd.account_id = $1 AND qd.quote_version_id = qv.id AND qd.id = $3
		 RETURNING qd.id`,
		accountID, versionID, discountID)
}

// DeleteByVersion removes every discount on a mutable version, whatever its origin, with its links.
func (r *QuoteDiscountRepository) DeleteByVersion(
	ctx context.Context, q Querier, accountID, versionID uuid.UUID,
) error {
	if _, err := q.Exec(ctx,
		`DELETE FROM quote_discount_item qdi
		 USING quote_discount qd, quote_version qv
		 WHERE qv.account_id = $1 AND qv.id = $2 AND qv.is_immutable = FALSE
		   AND qd.account_id = $1 AND qd.quote_version_id = qv.id
		   AND qdi.account_id = $1 AND qdi.quote_discount_id = qd.id`,
		accountID, versionID); err != nil {
		return err
	}
	_, err := q.Exec(ctx,
		`DELETE FROM quote_discount qd
		 USING quote_version qv
		 WHERE qv.account_id = $1 AND qv.id = $2 AND qv.is_immutable = FALSE
		   AND qd.account_id = $1 AND qd.quote_version_id = qv.id`,
		accountID, versionID)
	return err
}

// DeleteApplications removes engine-earned applications a sweep no longer earns, with their links.
func (r *QuoteDiscountRepository) DeleteApplications(
	ctx context.Context, q Querier, accountID, versionID uuid.UUID, discountIDs []uuid.UUID,
) error {
	if len(discountIDs) == 0 {
		return nil
	}
	if _, err := q.Exec(ctx,
		`DELETE FROM quote_discount_item qdi
		 USING quote_discount qd, quote_version qv
		 WHERE qv.account_id = $1 AND qv.id = $2 AND qv.is_immutable = FALSE
		   AND qd.account_id = $1 AND qd.quote_version_id = qv.id AND qd.id = ANY($3)
		   AND qd.origin IN ('AUTOMATIC'::discount_origin, 'AI_ADAPTATION'::discount_origin)
		   AND qdi.account_id = $1 AND qdi.quote_discount_id = qd.id`,
		accountID, versionID, discountIDs); err != nil {
		return err
	}
	tag, err := q.Exec(ctx,
		`DELETE FROM quote_discount qd
		 USING quote_version qv
		 WHERE qv.account_id = $1 AND qv.id = $2 AND qv.is_immutable = FALSE
		   AND qd.account_id = $1 AND qd.quote_version_id = qv.id AND qd.id = ANY($3)
		   AND qd.origin IN ('AUTOMATIC'::discount_origin, 'AI_ADAPTATION'::discount_origin)`,
		accountID, versionID, discountIDs)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != int64(len(discountIDs)) {
		return domain.ErrNotFound
	}
	return nil
}

// MarkAdaptations turns AUTOMATIC applications on a mutable version into AI_ADAPTATION ones: the
// promotions the AI reached by raising quantities.
func (r *QuoteDiscountRepository) MarkAdaptations(
	ctx context.Context, q Querier, accountID, versionID uuid.UUID, discountIDs []uuid.UUID,
) error {
	if len(discountIDs) == 0 {
		return nil
	}
	tag, err := q.Exec(ctx,
		`UPDATE quote_discount qd
		 SET origin = 'AI_ADAPTATION'::discount_origin
		 FROM quote_version qv
		 WHERE qv.account_id = $1 AND qv.id = $2 AND qv.is_immutable = FALSE
		   AND qd.account_id = $1 AND qd.quote_version_id = qv.id AND qd.id = ANY($3)
		   AND qd.origin = 'AUTOMATIC'::discount_origin`,
		accountID, versionID, discountIDs)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != int64(len(discountIDs)) {
		return domain.ErrNotFound
	}
	return nil
}

func (r *QuoteDiscountRepository) mustAffect(ctx context.Context, q Querier, query string, args ...any) error {
	var id uuid.UUID
	err := q.QueryRow(ctx, query, args...).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

func scanQuoteDiscount(row pgx.Row) (*domain.QuoteDiscount, error) {
	var discount domain.QuoteDiscount
	err := row.Scan(&discount.ID, &discount.AccountID, &discount.QuoteVersionID,
		&discount.PromotionID, &discount.ConditionType, &discount.Scope, &discount.Origin,
		&discount.Amount, &discount.ActionType, &discount.ActionValue,
		&discount.Description, &discount.SuppressedBySeller,
		&discount.CreatedAt, &discount.PromotionName)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &discount, nil
}
