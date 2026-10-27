package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const promotionColumns = `id, account_id, branch_id, name, description, condition_type,
	action_type, action_value, min_total, valid_from, valid_to, is_active, is_exclusive, priority,
	created_at, updated_at`

// PromotionRepository owns persistence for promotion rules and their conditions and tiers.
type PromotionRepository struct{}

// NewPromotionRepository builds a PromotionRepository.
func NewPromotionRepository() *PromotionRepository {
	return &PromotionRepository{}
}

// List returns every promotion of an account with its conditions and tiers, highest priority first.
func (r *PromotionRepository) List(ctx context.Context, q Querier, accountID uuid.UUID) ([]domain.Promotion, error) {
	rows, err := q.Query(ctx,
		`SELECT `+promotionColumns+`
		 FROM promotion
		 WHERE account_id = $1
		 ORDER BY priority DESC, name, id`,
		accountID)
	if err != nil {
		return nil, err
	}
	return r.withChildren(ctx, q, accountID, rows)
}

// ListInForce returns the active promotions a branch's quote can earn at an instant: the
// account-wide ones and the branch's own, within their validity window.
func (r *PromotionRepository) ListInForce(
	ctx context.Context, q Querier, accountID, branchID uuid.UUID, at time.Time,
) ([]domain.Promotion, error) {
	rows, err := q.Query(ctx,
		`SELECT `+promotionColumns+`
		 FROM promotion
		 WHERE account_id = $1 AND is_active = TRUE
		   AND (branch_id IS NULL OR branch_id = $2)
		   AND (valid_from IS NULL OR valid_from <= $3)
		   AND (valid_to IS NULL OR valid_to >= $3)
		 ORDER BY priority DESC, id`,
		accountID, branchID, at)
	if err != nil {
		return nil, err
	}
	return r.withChildren(ctx, q, accountID, rows)
}

// GetByID loads one promotion of an account with its conditions and tiers.
func (r *PromotionRepository) GetByID(ctx context.Context, q Querier, accountID, id uuid.UUID) (*domain.Promotion, error) {
	rows, err := q.Query(ctx,
		`SELECT `+promotionColumns+` FROM promotion WHERE account_id = $1 AND id = $2`,
		accountID, id)
	if err != nil {
		return nil, err
	}
	promotions, err := r.withChildren(ctx, q, accountID, rows)
	if err != nil {
		return nil, err
	}
	if len(promotions) == 0 {
		return nil, domain.ErrNotFound
	}
	return &promotions[0], nil
}

// ListProductGroups returns where each of an account's products sits in the taxonomy.
func (r *PromotionRepository) ListProductGroups(
	ctx context.Context, q Querier, accountID uuid.UUID, productIDs []uuid.UUID,
) (map[uuid.UUID]domain.ProductGroup, error) {
	groups := make(map[uuid.UUID]domain.ProductGroup, len(productIDs))
	if len(productIDs) == 0 {
		return groups, nil
	}
	rows, err := q.Query(ctx,
		`SELECT id, family_id, subgroup_id FROM product WHERE account_id = $1 AND id = ANY($2)`,
		accountID, productIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var group domain.ProductGroup
		if err := rows.Scan(&id, &group.FamilyID, &group.SubgroupID); err != nil {
			return nil, err
		}
		groups[id] = group
	}
	return groups, rows.Err()
}

// CountTargets counts how many of the given ids exist where a promotion may point: the account's
// own branches and products, and the shared taxonomy.
func (r *PromotionRepository) CountTargets(
	ctx context.Context, q Querier, accountID uuid.UUID, branchIDs, productIDs, familyIDs, subgroupIDs []uuid.UUID,
) (branches, products, families, subgroups int, err error) {
	err = q.QueryRow(ctx,
		`SELECT (SELECT count(*) FROM branch WHERE account_id = $1 AND id = ANY($2)),
		        (SELECT count(*) FROM product WHERE account_id = $1 AND id = ANY($3)),
		        (SELECT count(*) FROM product_family WHERE id = ANY($4)),
		        (SELECT count(*) FROM product_subgroup WHERE id = ANY($5))`,
		accountID, branchIDs, productIDs, familyIDs, subgroupIDs,
	).Scan(&branches, &products, &families, &subgroups)
	return branches, products, families, subgroups, err
}

// Create inserts a promotion with its conditions and tiers.
func (r *PromotionRepository) Create(
	ctx context.Context, q Querier, accountID uuid.UUID, in domain.PromotionInput,
) (*domain.Promotion, error) {
	var id uuid.UUID
	if err := q.QueryRow(ctx,
		`INSERT INTO promotion (account_id, branch_id, name, description, condition_type,
		        action_type, action_value, min_total, valid_from, valid_to, is_active, is_exclusive,
		        priority)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		 RETURNING id`,
		accountID, in.BranchID, in.Name, in.Description, in.ConditionType, in.ActionType,
		in.ActionValue, in.MinTotal, in.ValidFrom, in.ValidTo, in.IsActive, in.IsExclusive,
		in.Priority,
	).Scan(&id); err != nil {
		return nil, err
	}
	if err := r.createChildren(ctx, q, accountID, id, in); err != nil {
		return nil, err
	}
	return r.GetByID(ctx, q, accountID, id)
}

// Replace rewrites a promotion's whole rule, conditions and tiers included.
func (r *PromotionRepository) Replace(
	ctx context.Context, q Querier, accountID, id uuid.UUID, in domain.PromotionInput,
) (*domain.Promotion, error) {
	var updated uuid.UUID
	err := q.QueryRow(ctx,
		`UPDATE promotion
		 SET branch_id = $3, name = $4, description = $5, condition_type = $6, action_type = $7,
		     action_value = $8, min_total = $9, valid_from = $10, valid_to = $11, is_active = $12,
		     is_exclusive = $13, priority = $14
		 WHERE account_id = $1 AND id = $2
		 RETURNING id`,
		accountID, id, in.BranchID, in.Name, in.Description, in.ConditionType, in.ActionType,
		in.ActionValue, in.MinTotal, in.ValidFrom, in.ValidTo, in.IsActive, in.IsExclusive,
		in.Priority,
	).Scan(&updated)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if _, err := q.Exec(ctx,
		`DELETE FROM promotion_condition_item WHERE account_id = $1 AND promotion_id = $2`,
		accountID, id); err != nil {
		return nil, err
	}
	if _, err := q.Exec(ctx,
		`DELETE FROM promotion_tier WHERE account_id = $1 AND promotion_id = $2`,
		accountID, id); err != nil {
		return nil, err
	}
	if err := r.createChildren(ctx, q, accountID, id, in); err != nil {
		return nil, err
	}
	return r.GetByID(ctx, q, accountID, id)
}

func (r *PromotionRepository) createChildren(
	ctx context.Context, q Querier, accountID, promotionID uuid.UUID, in domain.PromotionInput,
) error {
	if len(in.Conditions) > 0 {
		type row struct {
			ProductID   *uuid.UUID `json:"product_id"`
			FamilyID    *uuid.UUID `json:"family_id"`
			SubgroupID  *uuid.UUID `json:"subgroup_id"`
			MinQuantity *string    `json:"min_quantity"`
		}
		payload := make([]row, len(in.Conditions))
		for i, c := range in.Conditions {
			payload[i] = row{ProductID: c.ProductID, FamilyID: c.FamilyID, SubgroupID: c.SubgroupID,
				MinQuantity: decimalPtrString(c.MinQuantity)}
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if _, err := q.Exec(ctx,
			`INSERT INTO promotion_condition_item (account_id, promotion_id, product_id, family_id,
			        subgroup_id, min_quantity)
			 SELECT $1, $2, x.product_id, x.family_id, x.subgroup_id, x.min_quantity
			 FROM jsonb_to_recordset($3::jsonb) AS x(product_id uuid, family_id uuid,
			      subgroup_id uuid, min_quantity numeric)`,
			accountID, promotionID, encoded); err != nil {
			return err
		}
	}
	if len(in.Tiers) > 0 {
		type row struct {
			FromQuantity string  `json:"from_quantity"`
			ToQuantity   *string `json:"to_quantity"`
			Value        string  `json:"value"`
		}
		payload := make([]row, len(in.Tiers))
		for i, t := range in.Tiers {
			payload[i] = row{FromQuantity: t.FromQuantity.String(),
				ToQuantity: decimalPtrString(t.ToQuantity), Value: t.Value.String()}
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		if _, err := q.Exec(ctx,
			`INSERT INTO promotion_tier (account_id, promotion_id, from_quantity, to_quantity, value)
			 SELECT $1, $2, x.from_quantity, x.to_quantity, x.value
			 FROM jsonb_to_recordset($3::jsonb) AS x(from_quantity numeric, to_quantity numeric,
			      value numeric)`,
			accountID, promotionID, encoded); err != nil {
			return err
		}
	}
	return nil
}

// withChildren scans promotion rows and attaches their conditions and tiers in two batch reads.
func (r *PromotionRepository) withChildren(
	ctx context.Context, q Querier, accountID uuid.UUID, rows pgx.Rows,
) ([]domain.Promotion, error) {
	promotions, err := scanPromotions(rows)
	if err != nil || len(promotions) == 0 {
		return promotions, err
	}
	ids := make([]uuid.UUID, len(promotions))
	index := make(map[uuid.UUID]int, len(promotions))
	for i, p := range promotions {
		ids[i] = p.ID
		index[p.ID] = i
	}

	conditionRows, err := q.Query(ctx,
		`SELECT promotion_id, product_id, family_id, subgroup_id, min_quantity
		 FROM promotion_condition_item
		 WHERE account_id = $1 AND promotion_id = ANY($2)
		 ORDER BY created_at, id`,
		accountID, ids)
	if err != nil {
		return nil, err
	}
	defer conditionRows.Close()
	for conditionRows.Next() {
		var promotionID uuid.UUID
		var c domain.PromotionCondition
		if err := conditionRows.Scan(&promotionID, &c.ProductID, &c.FamilyID, &c.SubgroupID,
			&c.MinQuantity); err != nil {
			return nil, err
		}
		i := index[promotionID]
		promotions[i].Conditions = append(promotions[i].Conditions, c)
	}
	if err := conditionRows.Err(); err != nil {
		return nil, err
	}

	tierRows, err := q.Query(ctx,
		`SELECT promotion_id, from_quantity, to_quantity, value
		 FROM promotion_tier
		 WHERE account_id = $1 AND promotion_id = ANY($2)
		 ORDER BY from_quantity, id`,
		accountID, ids)
	if err != nil {
		return nil, err
	}
	defer tierRows.Close()
	for tierRows.Next() {
		var promotionID uuid.UUID
		var t domain.PromotionTier
		if err := tierRows.Scan(&promotionID, &t.FromQuantity, &t.ToQuantity, &t.Value); err != nil {
			return nil, err
		}
		i := index[promotionID]
		promotions[i].Tiers = append(promotions[i].Tiers, t)
	}
	return promotions, tierRows.Err()
}

func scanPromotions(rows pgx.Rows) ([]domain.Promotion, error) {
	defer rows.Close()
	var promotions []domain.Promotion
	for rows.Next() {
		var p domain.Promotion
		if err := rows.Scan(&p.ID, &p.AccountID, &p.BranchID, &p.Name, &p.Description,
			&p.ConditionType, &p.ActionType, &p.ActionValue, &p.MinTotal, &p.ValidFrom, &p.ValidTo,
			&p.IsActive, &p.IsExclusive, &p.Priority, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		promotions = append(promotions, p)
	}
	return promotions, rows.Err()
}

func decimalPtrString(d *decimal.Decimal) *string {
	if d == nil {
		return nil
	}
	s := d.String()
	return &s
}
