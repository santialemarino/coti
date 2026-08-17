//go:build integration

package repository

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// These tests cover the hybrid catalog search against a real database: the vector codec, the
// branch filter, the lexical half reaching a trade term through a synonym, and the account
// boundary. The vectors are synthetic, so no provider is involved and every distance below is
// arithmetic anyone can redo by hand.

// alignedVector builds a unit vector whose cosine distance from queryVector is exactly
// 1 - alignment: all the weight on the first two axes, at a chosen angle.
func alignedVector(alignment float64) pgvector.Vector {
	values := make([]float32, domain.EmbeddingDimension)
	values[0] = float32(alignment)
	values[1] = float32(math.Sqrt(1 - alignment*alignment))
	return pgvector.NewVector(values)
}

// queryVector is what the searches below ask with: distance to alignedVector(a) is 1 - a.
func queryVector() pgvector.Vector { return alignedVector(1) }

// seedCatalogProduct inserts a product with a description, so the lexical document has both
// halves the generated column composes.
func seedCatalogProduct(t *testing.T, db *DB, accountID uuid.UUID, name, description string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := db.CrossAccount().Exec(context.Background(),
		`INSERT INTO product (id, account_id, canonical_name, description) VALUES ($1, $2, $3, $4)`,
		id, accountID, name, description); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DELETE FROM branch_product WHERE product_id = $1`, id)
		mustCleanup(t, db.CrossAccount(), `DELETE FROM product_synonym WHERE product_id = $1`, id)
		mustCleanup(t, db.CrossAccount(), `DELETE FROM product WHERE id = $1`, id)
	})
	return id
}

// stockBranch records that a branch carries the product.
func stockBranch(t *testing.T, db *DB, accountID, branchID, productID uuid.UUID, active bool) {
	t.Helper()
	if _, err := db.CrossAccount().Exec(context.Background(),
		`INSERT INTO branch_product (account_id, branch_id, product_id, is_active)
		 VALUES ($1, $2, $3, $4)`,
		accountID, branchID, productID, active); err != nil {
		t.Fatalf("seed branch_product: %v", err)
	}
}

func seedSynonym(t *testing.T, db *DB, accountID, productID uuid.UUID, term string) {
	t.Helper()
	if _, err := db.CrossAccount().Exec(context.Background(),
		`INSERT INTO product_synonym (account_id, product_id, term) VALUES ($1, $2, $3)`,
		accountID, productID, term); err != nil {
		t.Fatalf("seed product_synonym: %v", err)
	}
}

// currentUpdatedAt reads the row version a write has to still be against.
func currentUpdatedAt(t *testing.T, db *DB, productID uuid.UUID) time.Time {
	t.Helper()
	var updatedAt time.Time
	if err := db.CrossAccount().QueryRow(context.Background(),
		`SELECT updated_at FROM product WHERE id = $1`, productID).Scan(&updatedAt); err != nil {
		t.Fatalf("read updated_at: %v", err)
	}
	return updatedAt
}

// storeEmbedding stores a vector through the repository and reports how many rows it wrote,
// which is also what proves a pgvector.Vector can be bound as a query argument at all.
func storeEmbedding(
	t *testing.T, db *DB, accountID, productID uuid.UUID, vector pgvector.Vector,
	readAt time.Time,
) int {
	t.Helper()
	repo := NewProductRepository()
	ctx := context.Background()
	var written int
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountID}, func(q Querier) error {
		var err error
		written, err = repo.SetEmbeddings(ctx, q, accountID, []domain.ProductEmbedding{
			{ProductID: productID, Vector: vector, UpdatedAt: readAt},
		})
		return err
	}); err != nil {
		t.Fatalf("SetEmbeddings() = %v, want no error", err)
	}
	return written
}

// writeEmbedding stores a vector against the product as it stands right now.
func writeEmbedding(t *testing.T, db *DB, accountID, productID uuid.UUID, vector pgvector.Vector) {
	t.Helper()
	if written := storeEmbedding(t, db, accountID, productID, vector,
		currentUpdatedAt(t, db, productID)); written != 1 {
		t.Fatalf("SetEmbeddings() wrote %d rows, want 1", written)
	}
}

func searchCandidates(
	t *testing.T, db *DB, accountID, branchID uuid.UUID, text string, fetch int,
) []domain.CatalogCandidate {
	t.Helper()
	repo := NewProductRepository()
	ctx := context.Background()
	var candidates []domain.CatalogCandidate
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: accountID}, func(q Querier) error {
		var err error
		candidates, err = repo.SearchCandidates(ctx, q, accountID, branchID, text,
			queryVector(), fetch)
		return err
	}); err != nil {
		t.Fatalf("SearchCandidates() = %v, want no error", err)
	}
	return candidates
}

func nameOf(candidates []domain.CatalogCandidate) []string {
	names := make([]string, len(candidates))
	for i, c := range candidates {
		names[i] = c.CanonicalName
	}
	return names
}

// A vector written through the repository has to come back as the distance it implies. Before
// the codec was registered, binding one as a query argument failed outright.
func TestProductRepository_SetEmbeddingsStoresAVectorThatOrdersByDistance(t *testing.T) {
	db := testDB(t)
	account := seedAccount(t, db, "Corralon Vectores")
	branch := branchOf(t, db, account)

	near := seedCatalogProduct(t, db, account, "Cemento Portland 50kg", "bolsa")
	far := seedCatalogProduct(t, db, account, "Pintura latex 20L", "balde")
	stockBranch(t, db, account, branch, near, true)
	stockBranch(t, db, account, branch, far, true)
	writeEmbedding(t, db, account, near, alignedVector(1))
	writeEmbedding(t, db, account, far, alignedVector(0.25))

	candidates := searchCandidates(t, db, account, branch, "nada que coincida", 10)
	if len(candidates) != 2 {
		t.Fatalf("candidates = %d, want 2", len(candidates))
	}

	byID := map[uuid.UUID]domain.CatalogCandidate{}
	for _, c := range candidates {
		byID[c.ProductID] = c
	}
	// 1 - alignment, so 0.00 and 0.75 exactly.
	for _, tc := range []struct {
		name      string
		productID uuid.UUID
		want      float64
	}{
		{"the aligned product", near, 0},
		{"the tilted one", far, 0.75},
	} {
		got := byID[tc.productID]
		if got.Distance == nil {
			t.Fatalf("%s came back with no distance", tc.name)
		}
		if math.Abs(*got.Distance-tc.want) > 1e-6 {
			t.Errorf("%s distance = %v, want %v", tc.name, *got.Distance, tc.want)
		}
	}
}

// updated_at says a person changed the product. A backfill over a loaded catalog would otherwise
// stamp every row as edited at once, and leave the staleness comparison with nothing to measure.
func TestProductRepository_SetEmbeddingsDoesNotMarkTheProductEdited(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	account := seedAccount(t, db, "Corralon Trigger")
	product := seedCatalogProduct(t, db, account, "Cemento Portland 50kg", "bolsa")

	before := currentUpdatedAt(t, db, product)
	writeEmbedding(t, db, account, product, alignedVector(1))

	if after := currentUpdatedAt(t, db, product); !after.Equal(before) {
		t.Errorf("updated_at moved from %v to %v on an embedding write", before, after)
	}

	// A real edit still moves it, or nothing would ever read as stale again.
	if _, err := db.CrossAccount().Exec(ctx,
		`UPDATE product SET canonical_name = 'Cemento Portland 25kg' WHERE id = $1`,
		product); err != nil {
		t.Fatalf("edit product: %v", err)
	}
	if after := currentUpdatedAt(t, db, product); !after.After(before) {
		t.Errorf("updated_at = %v after an edit, want it past %v", after, before)
	}
}

// Embedding a product takes a provider call, and an edit can land during it. Writing the vector
// anyway would store one computed from text the product no longer has, and stamp it as current.
func TestProductRepository_SetEmbeddingsSkipsAProductEditedSinceItWasRead(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	account := seedAccount(t, db, "Corralon Carrera")
	product := seedCatalogProduct(t, db, account, "Cemento Portland 50kg", "bolsa")

	readAt := currentUpdatedAt(t, db, product)
	if _, err := db.CrossAccount().Exec(ctx,
		`UPDATE product SET canonical_name = 'Cemento Portland 25kg' WHERE id = $1`,
		product); err != nil {
		t.Fatalf("edit product: %v", err)
	}

	if written := storeEmbedding(t, db, account, product, alignedVector(1), readAt); written != 0 {
		t.Errorf("SetEmbeddings() wrote %d rows, want 0 for a product edited since the read", written)
	}

	var embedded bool
	if err := db.CrossAccount().QueryRow(ctx,
		`SELECT embedding IS NOT NULL FROM product WHERE id = $1`, product).Scan(&embedded); err != nil {
		t.Fatalf("read embedding: %v", err)
	}
	if embedded {
		t.Error("the stale vector was stored anyway")
	}
}

// The acceptance criterion: a search never offers a product the branch does not sell.
func TestProductRepository_SearchCandidatesExcludesWhatTheBranchDoesNotCarry(t *testing.T) {
	db := testDB(t)
	account := seedAccount(t, db, "Corralon Sucursales")
	centro := branchOf(t, db, account)
	norte := seedExtraBranch(t, db, account, "Norte")

	carried := seedCatalogProduct(t, db, account, "Cemento Portland 50kg", "bolsa")
	elsewhere := seedCatalogProduct(t, db, account, "Cemento Portland 25kg", "bolsa")
	discontinued := seedCatalogProduct(t, db, account, "Cemento blanco", "bolsa")
	for _, id := range []uuid.UUID{carried, elsewhere, discontinued} {
		writeEmbedding(t, db, account, id, alignedVector(1))
	}
	stockBranch(t, db, account, centro, carried, true)
	stockBranch(t, db, account, norte, elsewhere, true)
	// Listed at this branch but no longer sold there.
	stockBranch(t, db, account, centro, discontinued, false)

	got := nameOf(searchCandidates(t, db, account, centro, "cemento", 10))
	if len(got) != 1 || got[0] != "Cemento Portland 50kg" {
		t.Errorf("candidates for the Centro branch = %v, want only [Cemento Portland 50kg]", got)
	}
}

// The lexical half is what resolves trade terms, and it reads the synonym table: nobody should
// reach for a different embedding model to fix a vocabulary problem.
func TestProductRepository_SearchCandidatesFindsATradeTermThroughASynonym(t *testing.T) {
	db := testDB(t)
	account := seedAccount(t, db, "Corralon Sinonimos")
	branch := branchOf(t, db, account)

	product := seedCatalogProduct(t, db, account, "Membrana asfáltica 4mm", "rollo de 10m")
	stockBranch(t, db, account, branch, product, true)
	// Deliberately unembedded: only the lexical half can reach this row.
	seedSynonym(t, db, account, product, "telagoma")

	candidates := searchCandidates(t, db, account, branch, "telagoma", 10)
	if len(candidates) != 1 {
		t.Fatalf("candidates for a trade term = %v, want the one product", nameOf(candidates))
	}
	if candidates[0].ProductID != product {
		t.Errorf("candidate = %v, want %v", candidates[0].ProductID, product)
	}
	if candidates[0].LexicalScore == nil {
		t.Error("the synonym hit carries no lexical score, so the fusion has nothing to rank it by")
	}
	if candidates[0].Distance != nil {
		t.Errorf("distance = %v, want nil: the product carries no embedding",
			*candidates[0].Distance)
	}
}

// Informal RFQ text drops accents, which the stock spanish configuration would treat as a
// different word.
func TestProductRepository_SearchCandidatesIgnoresAccents(t *testing.T) {
	db := testDB(t)
	account := seedAccount(t, db, "Corralon Acentos")
	branch := branchOf(t, db, account)

	product := seedCatalogProduct(t, db, account, "Hormigón elaborado H21", "metro cúbico")
	stockBranch(t, db, account, branch, product, true)

	if got := nameOf(searchCandidates(t, db, account, branch, "hormigon", 10)); len(got) != 1 {
		t.Errorf("candidates for the unaccented term = %v, want the one product", got)
	}
}

// A product of another account is unreachable even when its vector is the closest one there is.
func TestProductRepository_SearchCandidatesStopsAtTheAccountBoundary(t *testing.T) {
	db := testDB(t)
	mine := seedAccount(t, db, "Corralon Propio")
	theirs := seedAccount(t, db, "Corralon Ajeno")
	myBranch := branchOf(t, db, mine)
	theirBranch := branchOf(t, db, theirs)

	theirProduct := seedCatalogProduct(t, db, theirs, "Cemento Portland 50kg", "bolsa")
	writeEmbedding(t, db, theirs, theirProduct, alignedVector(1))
	stockBranch(t, db, theirs, theirBranch, theirProduct, true)

	if got := searchCandidates(t, db, mine, myBranch, "cemento", 10); len(got) != 0 {
		t.Errorf("candidates from another account = %v, want none", nameOf(got))
	}
	// Naming their branch changes nothing: the policy is what refuses, not the predicate alone.
	if got := searchCandidates(t, db, mine, theirBranch, "cemento", 10); len(got) != 0 {
		t.Errorf("candidates when asking for their branch = %v, want none", nameOf(got))
	}
}

// The backfill only re-reads what it has to: a product it has embedded is done until it is
// edited, and the cursor is what moves a run forward.
func TestProductRepository_ListPendingEmbeddingTracksStaleness(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	account := seedAccount(t, db, "Corralon Pendientes")
	tenant := domain.Tenant{AccountID: account}
	repo := NewProductRepository()

	product := seedCatalogProduct(t, db, account, "Cemento Portland 50kg", "bolsa")

	pending := func(cursor uuid.UUID, refreshAll bool) []domain.ProductEmbeddingInput {
		t.Helper()
		var found []domain.ProductEmbeddingInput
		if err := db.InTenantTx(ctx, tenant, func(q Querier) error {
			var err error
			found, err = repo.ListPendingEmbedding(ctx, q, account, cursor, refreshAll, 50)
			return err
		}); err != nil {
			t.Fatalf("ListPendingEmbedding() = %v, want no error", err)
		}
		return found
	}

	if got := pending(uuid.Nil, false); len(got) != 1 || got[0].ProductID != product {
		t.Fatalf("pending before embedding = %d rows, want the one product", len(got))
	}

	writeEmbedding(t, db, account, product, alignedVector(1))
	if got := pending(uuid.Nil, false); len(got) != 0 {
		t.Errorf("pending after embedding = %d rows, want none", len(got))
	}
	if got := pending(uuid.Nil, true); len(got) != 1 {
		t.Errorf("pending with refreshAll = %d rows, want the product again", len(got))
	}
	// The cursor is what stops a refreshAll run from reading the same page forever.
	if got := pending(product, true); len(got) != 0 {
		t.Errorf("pending past the cursor = %d rows, want none", len(got))
	}

	if _, err := db.CrossAccount().Exec(ctx,
		`UPDATE product SET canonical_name = 'Cemento Portland 50kg reforzado' WHERE id = $1`,
		product); err != nil {
		t.Fatalf("edit product: %v", err)
	}
	got := pending(uuid.Nil, false)
	if len(got) != 1 {
		t.Fatalf("pending after an edit = %d rows, want the product back", len(got))
	}
	if got[0].CanonicalName != "Cemento Portland 50kg reforzado" {
		t.Errorf("pending name = %q, want the edited one", got[0].CanonicalName)
	}
}

// The acceptance criterion that the index exists and the semantic half uses it. Scans and sorts
// are switched off first: a fixture catalog is far too small for an approximate index to beat
// sorting it outright, so leaving the planner free would prove nothing either way. What the plan
// then settles is that the ordering the search asks for matches the index's operator class —
// build it for a different distance and the index becomes unusable with no other symptom.
func TestProductRepository_SearchUsesTheVectorIndex(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	account := seedAccount(t, db, "Corralon Indice")
	branch := branchOf(t, db, account)

	product := seedCatalogProduct(t, db, account, "Cemento Portland 50kg", "bolsa")
	stockBranch(t, db, account, branch, product, true)
	writeEmbedding(t, db, account, product, alignedVector(1))

	if _, err := db.CrossAccount().Exec(ctx,
		`CREATE INDEX idx_product_embedding ON product
		 USING ivfflat (embedding vector_cosine_ops) WITH (lists = 1)`); err != nil {
		t.Fatalf("create vector index: %v", err)
	}
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(), `DROP INDEX IF EXISTS idx_product_embedding`)
	})

	var plan strings.Builder
	if err := db.InTenantTx(ctx, domain.Tenant{AccountID: account}, func(q Querier) error {
		if _, err := q.Exec(ctx, `SET LOCAL enable_seqscan = off; SET LOCAL enable_sort = off`); err != nil {
			return err
		}
		rows, err := q.Query(ctx, "EXPLAIN "+searchCandidatesQuery,
			account, branch, "cemento", queryVector(), 10)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				return err
			}
			plan.WriteString(line)
			plan.WriteString("\n")
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("EXPLAIN the search = %v, want no error", err)
	}

	if !strings.Contains(plan.String(), "idx_product_embedding") {
		t.Errorf("the search plan does not read the vector index:\n%s", plan.String())
	}
}
