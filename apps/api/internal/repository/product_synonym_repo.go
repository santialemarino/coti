package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// productSynonymColumns keeps the SELECT list, the scan order, and the struct in one place.
const productSynonymColumns = `id, account_id, product_id, term, source, created_at`

// ProductSynonymRepository owns persistence for product_synonym.
type ProductSynonymRepository struct{}

// NewProductSynonymRepository builds a ProductSynonymRepository.
func NewProductSynonymRepository() *ProductSynonymRepository {
	return &ProductSynonymRepository{}
}

// List returns the product's synonyms, oldest first.
func (r *ProductSynonymRepository) List(
	ctx context.Context, q Querier, accountID, productID uuid.UUID,
) ([]domain.ProductSynonym, error) {
	rows, err := q.Query(ctx,
		`SELECT `+productSynonymColumns+`
		 FROM product_synonym
		 WHERE account_id = $1 AND product_id = $2
		 ORDER BY created_at, id`,
		accountID, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var synonyms []domain.ProductSynonym
	for rows.Next() {
		var s domain.ProductSynonym
		if err := rows.Scan(&s.ID, &s.AccountID, &s.ProductID, &s.Term, &s.Source,
			&s.CreatedAt); err != nil {
			return nil, err
		}
		synonyms = append(synonyms, s)
	}
	return synonyms, rows.Err()
}

// Create adds a synonym to a product. Returns domain.ErrConflict when the product already
// carries the term.
//
// The conflict target is uq_product_synonym_term, which indexes lower(term) — leaning on
// the index is what keeps two concurrent requests from both passing.
func (r *ProductSynonymRepository) Create(
	ctx context.Context, q Querier, accountID, productID uuid.UUID, term string,
	source domain.SynonymSource,
) (*domain.ProductSynonym, error) {
	var s domain.ProductSynonym
	err := q.QueryRow(ctx,
		`INSERT INTO product_synonym (account_id, product_id, term, source)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (account_id, product_id, lower(term)) DO NOTHING
		 RETURNING `+productSynonymColumns,
		accountID, productID, term, source,
	).Scan(&s.ID, &s.AccountID, &s.ProductID, &s.Term, &s.Source, &s.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrConflict
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Delete removes one synonym from a product. Returns domain.ErrNotFound when the synonym
// does not exist, belongs to another product, or belongs to another account.
func (r *ProductSynonymRepository) Delete(
	ctx context.Context, q Querier, accountID, productID, id uuid.UUID,
) error {
	tag, err := q.Exec(ctx,
		`DELETE FROM product_synonym WHERE account_id = $1 AND product_id = $2 AND id = $3`,
		accountID, productID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}
