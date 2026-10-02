//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// seedEvaluation writes the AI generation and quality evaluation a correction is learned from.
func seedEvaluation(t *testing.T, db *DB, accountID, quoteID, versionID uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	generationID, evaluationID := uuid.New(), uuid.New()
	if _, err := db.CrossAccount().Exec(ctx, `INSERT INTO quote_ai_generation
	  (id, account_id, quote_id, quote_version_id, provider, model, prompt_version, schema_version,
	   input_tokens, output_tokens, cache_read_tokens, cache_write_tokens)
	 VALUES ($1, $2, $3, $4, 'test', 'test', 'v1', 'v1', 0, 0, 0, 0)`,
		generationID, accountID, quoteID, versionID); err != nil {
		t.Fatalf("seed generation: %v", err)
	}
	if _, err := db.CrossAccount().Exec(ctx, `INSERT INTO quote_quality_evaluation
	  (id, account_id, generation_id, final_quote_version_id, evaluator_version,
	   whole_quote_correct, same_item_count, all_items_equivalent, all_items_matched,
	   all_items_priced, all_subtotals_valid, total_valid)
	 VALUES ($1, $2, $3, $4, 'test', FALSE, TRUE, FALSE, TRUE, TRUE, TRUE, TRUE)`,
		evaluationID, accountID, generationID, versionID); err != nil {
		t.Fatalf("seed evaluation: %v", err)
	}
	t.Cleanup(func() {
		mustCleanup(t, db.CrossAccount(),
			`DELETE FROM quote_correction_memory WHERE account_id = $1`, accountID)
		mustCleanup(t, db.CrossAccount(),
			`DELETE FROM quote_quality_evaluation WHERE id = $1`, evaluationID)
		mustCleanup(t, db.CrossAccount(), `DELETE FROM quote_ai_generation WHERE id = $1`,
			generationID)
	})
	return evaluationID
}

// A seller who undoes a correction, as a substitution for stock is undone once the product is back,
// leaves one answer for the phrase: two would contest it forever.
func TestQuoteCorrectionRepository_Enqueue_KeepsOnlyTheLatestAnswerForAPhrase(t *testing.T) {
	db := testDB(t)
	account := seedAccount(t, db, "Corralon Ultima Palabra")
	branch := branchOf(t, db, account)
	substitute := seedCatalogProduct(t, db, account, "Cemento Avellaneda 50kg", "")
	original := seedCatalogProduct(t, db, account, "Cemento Loma Negra 50kg", "")
	quoteID, versionID, _ := seedQuoteChain(t, db, account, branch, original)
	evaluationID := seedEvaluation(t, db, account, quoteID, versionID)
	repo := NewQuoteCorrectionRepository()
	ctx := context.Background()

	teach := func(product uuid.UUID, key string) {
		t.Helper()
		if err := db.InTenantTx(ctx, domain.Tenant{AccountID: account}, func(q Querier) error {
			_, err := repo.Enqueue(ctx, q, account, evaluationID, []domain.NewQuoteCorrectionMemory{{
				Kind: domain.QuoteCorrectionMemoryCatalog, SourceText: "Cemento Loma Negra",
				ProductID: &product, SourceKey: key,
			}}, 100)
			return err
		}); err != nil {
			t.Fatalf("Enqueue() = %v", err)
		}
	}
	teach(substitute, "catalog:first")
	teach(original, "catalog:second")

	var products []uuid.UUID
	rows, err := db.CrossAccount().Query(ctx, `SELECT product_id FROM quote_correction_memory
	 WHERE account_id = $1 AND kind = 'CATALOG'`, account)
	if err != nil {
		t.Fatalf("read memories: %v", err)
	}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan memory: %v", err)
		}
		products = append(products, id)
	}
	rows.Close()
	if len(products) != 1 || products[0] != original {
		t.Errorf("taught products = %v, want only the latest, %v", products, original)
	}
}
