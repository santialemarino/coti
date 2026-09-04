package repository

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// productColumns keeps the SELECT list, the scan order, and the struct in one place.
// embedding is left out on purpose: 1536 floats that no catalog read needs.
const productColumns = `id, account_id, code, canonical_name, description, unit, family_id, subgroup_id,
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

// List returns one page of the account's catalog plus the total the filter matches. The
// total is a window function, so it cannot disagree with the page it describes.
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
		   AND ($4::uuid IS NULL OR family_id = $4)
		   AND ($5::uuid IS NULL OR subgroup_id = $5)
		 ORDER BY canonical_name, id
		 LIMIT $6 OFFSET $7`,
		accountID, f.IncludeInactive, f.Search, f.FamilyID, f.SubgroupID, f.Limit, f.Offset)
	if err != nil {
		return domain.ProductPage{}, err
	}
	defer rows.Close()

	page := domain.ProductPage{Limit: f.Limit, Offset: f.Offset}
	for rows.Next() {
		var p domain.Product
		if err := rows.Scan(&p.ID, &p.AccountID, &p.Code, &p.CanonicalName, &p.Description,
			&p.Unit, &p.FamilyID, &p.SubgroupID, &p.IsActive, &p.CreatedAt, &p.UpdatedAt, &page.Total); err != nil {
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

// GetByIDForUpdate loads one product within the account and holds a row lock on it until the
// transaction ends, so writes that must not interleave for the same product serialize on it.
func (r *ProductRepository) GetByIDForUpdate(
	ctx context.Context, q Querier, accountID, id uuid.UUID,
) (*domain.Product, error) {
	return scanProduct(q.QueryRow(ctx,
		`SELECT `+productColumns+`
		 FROM product
		 WHERE account_id = $1 AND id = $2
		 FOR UPDATE`,
		accountID, id))
}

// ListPendingEmbedding returns the next products whose vector is missing or older than their
// last edit, in id order from after cursor. A zero cursor starts at the beginning of the
// catalog, and refreshAll takes every active product rather than only the stale ones.
func (r *ProductRepository) ListPendingEmbedding(
	ctx context.Context, q Querier, accountID, cursor uuid.UUID, refreshAll bool, limit int,
) ([]domain.ProductEmbeddingInput, error) {
	rows, err := q.Query(ctx,
		`SELECT id, canonical_name, description, updated_at
		 FROM product
		 WHERE account_id = $1
		   AND is_active = TRUE
		   AND id > $2
		   AND ($3 OR embedding IS NULL OR embedding_updated_at IS NULL
		        OR embedding_updated_at < updated_at)
		 ORDER BY id
		 LIMIT $4`,
		accountID, cursor, refreshAll, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var pending []domain.ProductEmbeddingInput
	for rows.Next() {
		var in domain.ProductEmbeddingInput
		if err := rows.Scan(&in.ProductID, &in.CanonicalName, &in.Description, &in.UpdatedAt); err != nil {
			return nil, err
		}
		pending = append(pending, in)
	}
	return pending, rows.Err()
}

// Create inserts a catalog item. Returns domain.ErrConflict when the account already has
// a product carrying the same code.
func (r *ProductRepository) Create(
	ctx context.Context, q Querier, accountID uuid.UUID, in domain.NewProduct,
) (*domain.Product, error) {
	p, err := scanProduct(q.QueryRow(ctx,
		`INSERT INTO product (account_id, code, canonical_name, description, unit, family_id, subgroup_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING `+productColumns,
		accountID, in.Code, in.CanonicalName, in.Description, in.Unit, in.FamilyID, in.SubgroupID))
	if isUniqueViolation(err, productCodeIndex) {
		return nil, domain.ErrConflict
	}
	return p, err
}

// Update replaces the product's editable attributes and returns the stored row. A nil
// IsActive keeps the current flag, so an edit cannot silently revive a soft-deleted item.
func (r *ProductRepository) Update(
	ctx context.Context, q Querier, accountID, id uuid.UUID, in domain.ProductUpdate,
) (*domain.Product, error) {
	p, err := scanProduct(q.QueryRow(ctx,
		`UPDATE product
		 SET code = $3, canonical_name = $4, description = $5, unit = $6, family_id = $7,
		     subgroup_id = $8, is_active = coalesce($9, is_active)
		 WHERE account_id = $1 AND id = $2
		 RETURNING `+productColumns,
		accountID, id, in.Code, in.CanonicalName, in.Description, in.Unit, in.FamilyID,
		in.SubgroupID, in.IsActive))
	if isUniqueViolation(err, productCodeIndex) {
		return nil, domain.ErrConflict
	}
	return p, err
}

// SetEmbeddings stores a batch of vectors in one statement and stamps when each was computed,
// so an edit made afterwards reads as stale on the next backfill. Returns the rows written.
//
// A row edited since it was read is skipped rather than overwritten: its vector was computed
// from text that is no longer the product's, and stamping it would mark that stale one fresh.
func (r *ProductRepository) SetEmbeddings(
	ctx context.Context, q Querier, accountID uuid.UUID, embeddings []domain.ProductEmbedding,
) (int, error) {
	if len(embeddings) == 0 {
		return 0, nil
	}
	ids := make([]uuid.UUID, len(embeddings))
	vectors := make([]pgvector.Vector, len(embeddings))
	readAt := make([]time.Time, len(embeddings))
	for i, e := range embeddings {
		ids[i] = e.ProductID
		vectors[i] = e.Vector
		readAt[i] = e.UpdatedAt
	}

	tag, err := q.Exec(ctx,
		`UPDATE product p
		 SET embedding = v.embedding, embedding_updated_at = now()
		 FROM unnest($2::uuid[], $3::vector[], $4::timestamptz[]) AS v(id, embedding, read_at)
		 WHERE p.id = v.id AND p.account_id = $1 AND p.updated_at = v.read_at`,
		accountID, ids, vectors, readAt)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// Delete deactivates the product. Soft, because quote items and price history point at the
// row and removing it would rewrite closed history.
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

// SetSearchProbes tells the approximate vector scan how many index partitions to visit,
// for the rest of the transaction. The database's own default is one, which recalls too
// little of the catalog to survive the branch filter.
func (r *ProductRepository) SetSearchProbes(ctx context.Context, q Querier, probes int) error {
	_, err := q.Exec(ctx, `SELECT set_config('ivfflat.probes', $1, true)`, strconv.Itoa(probes))
	return err
}

// searchCandidatesQuery is the hybrid catalog search: the closest vectors and the full-text
// matches over product names and their synonyms, narrowed to what the branch carries.
//
// Kept as a const so the test that reads its execution plan measures this statement rather than
// a copy of it.
const searchCandidatesQuery = `
	WITH ask_document AS (
	    SELECT to_tsvector('spanish_unaccent'::regconfig, $3) AS document
	),
	ask AS (
	    SELECT plainto_tsquery('spanish_unaccent'::regconfig, $3) AS query,
	           document,
	           to_tsquery('spanish_unaccent'::regconfig,
	               array_to_string(tsvector_to_array(document), ' | ')) AS any_term_query
	    FROM ask_document
	),
	semantic AS (
	    SELECT id, embedding <=> $4 AS distance
	    FROM product
	    WHERE account_id = $1 AND is_active = TRUE AND embedding IS NOT NULL
	    ORDER BY embedding <=> $4
	    LIMIT $5
	),
	learned AS (
	    SELECT product_id, min(embedding <=> $4) AS distance
	    FROM quote_correction_memory
	    WHERE account_id = $1 AND kind = 'CATALOG' AND status = 'READY'
	      AND embedding <=> $4 <= $6
	    GROUP BY product_id
	),
	lexical_hit AS (
	    SELECT p.id AS product_id, ts_rank(p.search_document, ask.query)::float8 AS score
	    FROM product p, ask
	    WHERE p.account_id = $1 AND p.is_active = TRUE
	      AND p.search_document @@ ask.query
	    UNION ALL
	    SELECT s.product_id,
	           ts_rank(ask.document,
	               plainto_tsquery('spanish_unaccent'::regconfig, s.term))::float8 AS score
	    FROM product_synonym s
	    JOIN product sp ON sp.id = s.product_id AND sp.account_id = $1 AND sp.is_active = TRUE
	    CROSS JOIN ask
	    WHERE s.account_id = $1
	      AND s.search_document @@ ask.any_term_query
	      AND ask.document @@ plainto_tsquery('spanish_unaccent'::regconfig, s.term)
	),
	lexical AS (
	    SELECT product_id, max(score)::float8 AS score
	    FROM lexical_hit
	    GROUP BY product_id
	    ORDER BY score DESC
	    LIMIT $5
	),
	candidate AS (
	    SELECT id FROM semantic
	    UNION
	    SELECT product_id FROM lexical
	    UNION
	    SELECT product_id FROM learned
	)
	SELECT p.id, p.code, p.canonical_name, p.unit, semantic.distance, lexical.score,
	       learned.distance
	FROM candidate c
	JOIN product p ON p.id = c.id AND p.account_id = $1
	JOIN branch_product bp ON bp.product_id = p.id AND bp.account_id = $1
	  AND bp.branch_id = $2 AND bp.is_active = TRUE
	LEFT JOIN semantic ON semantic.id = p.id
	LEFT JOIN learned ON learned.product_id = p.id
	LEFT JOIN lexical ON lexical.product_id = p.id`

// SearchCandidates returns the catalog items the branch carries that either half of the search
// reached.
//
// Each half is asked for fetch rows because the approximate scan orders before the branch filter
// runs, so the result comes back short of what the caller wanted rather than wrong. Ranking the
// two halves together and trimming to the caller's K is the service's job.
func (r *ProductRepository) SearchCandidates(
	ctx context.Context, q Querier, accountID, branchID uuid.UUID,
	text string, embedding pgvector.Vector, fetch int, correctionMaxDistance float64,
) ([]domain.CatalogCandidate, error) {
	rows, err := q.Query(ctx, searchCandidatesQuery, accountID, branchID, text, embedding, fetch,
		correctionMaxDistance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var candidates []domain.CatalogCandidate
	for rows.Next() {
		var c domain.CatalogCandidate
		if err := rows.Scan(&c.ProductID, &c.Code, &c.CanonicalName, &c.Unit,
			&c.Distance, &c.LexicalScore, &c.LearnedDistance); err != nil {
			return nil, err
		}
		candidates = append(candidates, c)
	}
	return candidates, rows.Err()
}

func scanProduct(row pgx.Row) (*domain.Product, error) {
	var p domain.Product
	err := row.Scan(&p.ID, &p.AccountID, &p.Code, &p.CanonicalName, &p.Description, &p.Unit,
		&p.FamilyID, &p.SubgroupID, &p.IsActive, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}
