package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// productColumns keeps the SELECT list, the scan order, and the struct in one place.
// embedding is left out on purpose: 1536 floats that no catalog read needs.
const productColumns = `id, account_id, code, canonical_name, description, unit, category,
	is_active, created_at, updated_at`

// productCodeIndex is the partial unique index behind "one code per account". Partial
// because code is nullable, so unnamed products do not collide with each other.
const productCodeIndex = "uq_product_account_code"

// ProductRepository owns persistence for product.
type ProductRepository struct{}

// NewProductRepository builds a ProductRepository.
func NewProductRepository() *ProductRepository {
	return &ProductRepository{}
}

// List returns one page of the account's catalog plus the total the filter matches.
//
// The total comes from a window function rather than a second COUNT query: one round
// trip, and the count cannot disagree with the page it describes.
func (r *ProductRepository) List(
	ctx context.Context, q Querier, accountID uuid.UUID, f domain.ProductFilter,
) (domain.ProductPage, error) {
	rows, err := q.Query(ctx,
		`SELECT `+productColumns+`, count(*) OVER () AS total
		 FROM product
		 WHERE account_id = $1
		   AND ($2 OR is_active = TRUE)
		   AND ($3::text = '' OR canonical_name ILIKE '%' || $3 || '%'
		        OR coalesce(code, '') ILIKE '%' || $3 || '%')
		   AND ($4::text = '' OR category = $4)
		 ORDER BY canonical_name, id
		 LIMIT $5 OFFSET $6`,
		accountID, f.IncludeInactive, f.Search, f.Category, f.Limit, f.Offset)
	if err != nil {
		return domain.ProductPage{}, err
	}
	defer rows.Close()

	page := domain.ProductPage{Limit: f.Limit, Offset: f.Offset}
	for rows.Next() {
		var p domain.Product
		if err := rows.Scan(&p.ID, &p.AccountID, &p.Code, &p.CanonicalName, &p.Description,
			&p.Unit, &p.Category, &p.IsActive, &p.CreatedAt, &p.UpdatedAt, &page.Total); err != nil {
			return domain.ProductPage{}, err
		}
		page.Items = append(page.Items, p)
	}
	return page, rows.Err()
}

// GetByID loads one product within the account. Returns domain.ErrNotFound if absent.
func (r *ProductRepository) GetByID(
	ctx context.Context, q Querier, accountID, id uuid.UUID,
) (*domain.Product, error) {
	return scanProduct(q.QueryRow(ctx,
		`SELECT `+productColumns+` FROM product WHERE account_id = $1 AND id = $2`,
		accountID, id))
}

// Create inserts a catalog item. Returns domain.ErrConflict when the account already has
// a product carrying the same code.
func (r *ProductRepository) Create(
	ctx context.Context, q Querier, accountID uuid.UUID, in domain.NewProduct,
) (*domain.Product, error) {
	p, err := scanProduct(q.QueryRow(ctx,
		`INSERT INTO product (account_id, code, canonical_name, description, unit, category)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING `+productColumns,
		accountID, in.Code, in.CanonicalName, in.Description, in.Unit, in.Category))
	if isUniqueViolation(err, productCodeIndex) {
		return nil, domain.ErrConflict
	}
	return p, err
}

// Update replaces the product's editable attributes and returns the stored row.
//
// A nil IsActive keeps the current flag, so an edit cannot silently revive a
// soft-deleted item. Returns domain.ErrNotFound if the product is not in the account,
// domain.ErrConflict on a duplicate code.
func (r *ProductRepository) Update(
	ctx context.Context, q Querier, accountID, id uuid.UUID, in domain.ProductUpdate,
) (*domain.Product, error) {
	p, err := scanProduct(q.QueryRow(ctx,
		`UPDATE product
		 SET code = $3, canonical_name = $4, description = $5, unit = $6, category = $7,
		     is_active = coalesce($8, is_active)
		 WHERE account_id = $1 AND id = $2
		 RETURNING `+productColumns,
		accountID, id, in.Code, in.CanonicalName, in.Description, in.Unit, in.Category,
		in.IsActive))
	if isUniqueViolation(err, productCodeIndex) {
		return nil, domain.ErrConflict
	}
	return p, err
}

// Delete deactivates the product. It is a soft delete because quote items and price
// history keep pointing at the row, so removing it would rewrite closed history.
func (r *ProductRepository) Delete(ctx context.Context, q Querier, accountID, id uuid.UUID) error {
	tag, err := q.Exec(ctx,
		`UPDATE product SET is_active = FALSE WHERE account_id = $1 AND id = $2`,
		accountID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func scanProduct(row pgx.Row) (*domain.Product, error) {
	var p domain.Product
	err := row.Scan(&p.ID, &p.AccountID, &p.Code, &p.CanonicalName, &p.Description, &p.Unit,
		&p.Category, &p.IsActive, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}
