package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// ComboRepository owns persistence for catalog combos, their items and branch availability.
type ComboRepository struct{}

// NewComboRepository builds a ComboRepository.
func NewComboRepository() *ComboRepository {
	return &ComboRepository{}
}

// List returns every combo of an account with its items and branches, by name.
func (r *ComboRepository) List(ctx context.Context, q Querier, accountID uuid.UUID) ([]domain.Combo, error) {
	rows, err := q.Query(ctx,
		`SELECT id, account_id, name, description, is_active, created_at, updated_at
		 FROM combo WHERE account_id = $1 ORDER BY name, id`, accountID)
	if err != nil {
		return nil, err
	}
	return r.withChildren(ctx, q, accountID, rows)
}

// ListSoldAtBranch returns the active combos a branch sells, with their items.
func (r *ComboRepository) ListSoldAtBranch(
	ctx context.Context, q Querier, accountID, branchID uuid.UUID,
) ([]domain.Combo, error) {
	rows, err := q.Query(ctx,
		`SELECT c.id, c.account_id, c.name, c.description, c.is_active, c.created_at, c.updated_at
		 FROM combo c
		 JOIN branch_combo bc ON bc.combo_id = c.id AND bc.account_id = c.account_id
		 WHERE c.account_id = $1 AND c.is_active = TRUE AND bc.branch_id = $2 AND bc.is_active = TRUE
		 ORDER BY c.name, c.id`, accountID, branchID)
	if err != nil {
		return nil, err
	}
	return r.withChildren(ctx, q, accountID, rows)
}

// GetByID loads one combo of an account with its items and branches.
func (r *ComboRepository) GetByID(ctx context.Context, q Querier, accountID, id uuid.UUID) (*domain.Combo, error) {
	rows, err := q.Query(ctx,
		`SELECT id, account_id, name, description, is_active, created_at, updated_at
		 FROM combo WHERE account_id = $1 AND id = $2`, accountID, id)
	if err != nil {
		return nil, err
	}
	combos, err := r.withChildren(ctx, q, accountID, rows)
	if err != nil {
		return nil, err
	}
	if len(combos) == 0 {
		return nil, domain.ErrNotFound
	}
	return &combos[0], nil
}

// Create inserts a combo with its items and branches in the caller's transaction.
func (r *ComboRepository) Create(
	ctx context.Context, q Querier, accountID uuid.UUID, in domain.ComboInput,
) (uuid.UUID, error) {
	var id uuid.UUID
	if err := q.QueryRow(ctx,
		`INSERT INTO combo (account_id, name, description, is_active)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		accountID, in.Name, in.Description, in.IsActive).Scan(&id); err != nil {
		return uuid.Nil, err
	}
	return id, r.writeChildren(ctx, q, accountID, id, in)
}

// Replace rewrites a combo's definition, items and branches in the caller's transaction.
func (r *ComboRepository) Replace(
	ctx context.Context, q Querier, accountID, id uuid.UUID, in domain.ComboInput,
) error {
	tag, err := q.Exec(ctx,
		`UPDATE combo SET name = $3, description = $4, is_active = $5
		 WHERE account_id = $1 AND id = $2`,
		accountID, id, in.Name, in.Description, in.IsActive)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	if _, err := q.Exec(ctx,
		`DELETE FROM combo_item WHERE account_id = $1 AND combo_id = $2`, accountID, id); err != nil {
		return err
	}
	return r.writeChildren(ctx, q, accountID, id, in)
}

/*
 * writeChildren inserts the items and sets the branch availability. Items and branches are read
 * through the account's own rows, so an id from another account inserts nothing and is refused.
 * A branch left out is switched off rather than deleted, the way branch_product is.
 */
func (r *ComboRepository) writeChildren(
	ctx context.Context, q Querier, accountID, comboID uuid.UUID, in domain.ComboInput,
) error {
	productIDs := make([]uuid.UUID, len(in.Items))
	quantities := make([]string, len(in.Items))
	for i, item := range in.Items {
		productIDs[i] = item.ProductID
		quantities[i] = item.Quantity.String()
	}
	tag, err := q.Exec(ctx,
		`INSERT INTO combo_item (account_id, combo_id, product_id, quantity)
		 SELECT $1, $2, p.id, u.quantity::numeric
		 FROM unnest($3::uuid[], $4::text[]) AS u(product_id, quantity)
		 JOIN product p ON p.account_id = $1 AND p.id = u.product_id`,
		accountID, comboID, productIDs, quantities)
	if err != nil {
		return err
	}
	if int(tag.RowsAffected()) != len(in.Items) {
		return domain.ErrInvalidInput
	}
	if _, err := q.Exec(ctx,
		`UPDATE branch_combo SET is_active = FALSE
		 WHERE account_id = $1 AND combo_id = $2 AND NOT (branch_id = ANY($3::uuid[]))`,
		accountID, comboID, in.BranchIDs); err != nil {
		return err
	}
	tag, err = q.Exec(ctx,
		`INSERT INTO branch_combo (account_id, branch_id, combo_id, is_active)
		 SELECT $1, b.id, $2, TRUE FROM branch b WHERE b.account_id = $1 AND b.id = ANY($3::uuid[])
		 ON CONFLICT (branch_id, combo_id) DO UPDATE SET is_active = TRUE`,
		accountID, comboID, in.BranchIDs)
	if err != nil {
		return err
	}
	if int(tag.RowsAffected()) != len(in.BranchIDs) {
		return domain.ErrInvalidInput
	}
	return nil
}

// withChildren scans combo rows and loads every combo's items and branches in two queries.
func (r *ComboRepository) withChildren(
	ctx context.Context, q Querier, accountID uuid.UUID, rows pgx.Rows,
) ([]domain.Combo, error) {
	var combos []domain.Combo
	for rows.Next() {
		var c domain.Combo
		if err := rows.Scan(&c.ID, &c.AccountID, &c.Name, &c.Description, &c.IsActive,
			&c.CreatedAt, &c.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		combos = append(combos, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(combos) == 0 {
		return []domain.Combo{}, nil
	}
	ids := make([]uuid.UUID, len(combos))
	index := make(map[uuid.UUID]int, len(combos))
	for i, c := range combos {
		ids[i] = c.ID
		index[c.ID] = i
	}

	itemRows, err := q.Query(ctx,
		`SELECT ci.combo_id, ci.product_id, ci.quantity, p.code, p.canonical_name, p.unit, p.is_active
		 FROM combo_item ci
		 JOIN product p ON p.id = ci.product_id AND p.account_id = ci.account_id
		 WHERE ci.account_id = $1 AND ci.combo_id = ANY($2)
		 ORDER BY ci.created_at, p.canonical_name`, accountID, ids)
	if err != nil {
		return nil, err
	}
	for itemRows.Next() {
		var comboID uuid.UUID
		var item domain.ComboItem
		if err := itemRows.Scan(&comboID, &item.ProductID, &item.Quantity, &item.ProductCode,
			&item.ProductName, &item.ProductUnit, &item.ProductIsActive); err != nil {
			itemRows.Close()
			return nil, err
		}
		combos[index[comboID]].Items = append(combos[index[comboID]].Items, item)
	}
	itemRows.Close()
	if err := itemRows.Err(); err != nil {
		return nil, err
	}

	branchRows, err := q.Query(ctx,
		`SELECT combo_id, branch_id FROM branch_combo
		 WHERE account_id = $1 AND combo_id = ANY($2) AND is_active = TRUE
		 ORDER BY branch_id`, accountID, ids)
	if err != nil {
		return nil, err
	}
	defer branchRows.Close()
	for branchRows.Next() {
		var comboID, branchID uuid.UUID
		if err := branchRows.Scan(&comboID, &branchID); err != nil {
			return nil, err
		}
		combos[index[comboID]].BranchIDs = append(combos[index[comboID]].BranchIDs, branchID)
	}
	return combos, branchRows.Err()
}
