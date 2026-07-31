package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// productAlternativeColumns keeps the SELECT list, the scan order, and the struct in one place.
const productAlternativeColumns = `id, account_id, base_product_id, alternative_product_id,
	type, created_at`

// productAlternativePair is the constraint behind "one link per ordered pair".
const productAlternativePair = "uq_product_alternative"

// The two directions are separate statements rather than one query with a conditional
// anchor: each keeps a plain equality predicate, which stays index-friendly and reads
// like what it is. The service still has a single method, so the caller never chooses.
const (
	alternativesOutgoing = `SELECT pa.id, pa.account_id, pa.base_product_id,
		pa.alternative_product_id, pa.type, pa.created_at,
		p.id, p.account_id, p.code, p.canonical_name, p.description, p.unit, p.category,
		p.is_active, p.created_at, p.updated_at
	 FROM product_alternative pa
	 JOIN product p ON p.id = pa.alternative_product_id AND p.account_id = pa.account_id
	 WHERE pa.account_id = $1 AND pa.base_product_id = $2
	 ORDER BY pa.type, p.canonical_name`

	alternativesIncoming = `SELECT pa.id, pa.account_id, pa.base_product_id,
		pa.alternative_product_id, pa.type, pa.created_at,
		p.id, p.account_id, p.code, p.canonical_name, p.description, p.unit, p.category,
		p.is_active, p.created_at, p.updated_at
	 FROM product_alternative pa
	 JOIN product p ON p.id = pa.base_product_id AND p.account_id = pa.account_id
	 WHERE pa.account_id = $1 AND pa.alternative_product_id = $2
	 ORDER BY pa.type, p.canonical_name`
)

// ProductAlternativeRepository owns persistence for product_alternative.
type ProductAlternativeRepository struct{}

// NewProductAlternativeRepository builds a ProductAlternativeRepository.
func NewProductAlternativeRepository() *ProductAlternativeRepository {
	return &ProductAlternativeRepository{}
}

// List returns the product's alternative links anchored on the requested end of the
// relation, with the product on the far end joined in.
func (r *ProductAlternativeRepository) List(
	ctx context.Context, q Querier, accountID, productID uuid.UUID,
	direction domain.AlternativeDirection,
) ([]domain.ProductAlternativeView, error) {
	sql := alternativesOutgoing
	if direction == domain.AlternativeDirectionIncoming {
		sql = alternativesIncoming
	}

	rows, err := q.Query(ctx, sql, accountID, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var views []domain.ProductAlternativeView
	for rows.Next() {
		var v domain.ProductAlternativeView
		if err := rows.Scan(&v.Link.ID, &v.Link.AccountID, &v.Link.BaseProductID,
			&v.Link.AlternativeProductID, &v.Link.Type, &v.Link.CreatedAt,
			&v.Product.ID, &v.Product.AccountID, &v.Product.Code, &v.Product.CanonicalName,
			&v.Product.Description, &v.Product.Unit, &v.Product.Category, &v.Product.IsActive,
			&v.Product.CreatedAt, &v.Product.UpdatedAt); err != nil {
			return nil, err
		}
		views = append(views, v)
	}
	return views, rows.Err()
}

// Create links a base product to an alternative. Returns domain.ErrConflict when the
// ordered pair is already linked.
func (r *ProductAlternativeRepository) Create(
	ctx context.Context, q Querier, accountID, baseProductID, alternativeProductID uuid.UUID,
	alternativeType domain.ProductAlternativeType,
) (*domain.ProductAlternative, error) {
	var a domain.ProductAlternative
	err := q.QueryRow(ctx,
		`INSERT INTO product_alternative
		   (account_id, base_product_id, alternative_product_id, type)
		 VALUES ($1, $2, $3, $4)
		 RETURNING `+productAlternativeColumns,
		accountID, baseProductID, alternativeProductID, alternativeType,
	).Scan(&a.ID, &a.AccountID, &a.BaseProductID, &a.AlternativeProductID, &a.Type, &a.CreatedAt)
	if isUniqueViolation(err, productAlternativePair) {
		return nil, domain.ErrConflict
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// Delete removes one link, from either end.
//
// The product id has to match one of the two ends so the route that carries it stays
// meaningful: a link between two other products is not deletable through a third one.
// Returns domain.ErrNotFound when nothing matches.
func (r *ProductAlternativeRepository) Delete(
	ctx context.Context, q Querier, accountID, productID, id uuid.UUID,
) error {
	tag, err := q.Exec(ctx,
		`DELETE FROM product_alternative
		 WHERE account_id = $1 AND id = $2
		   AND (base_product_id = $3 OR alternative_product_id = $3)`,
		accountID, id, productID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
