package services

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// catalogEmbeddingAccounts is the account lookup that tells a mistyped id from a catalog with
// nothing left to embed — under row level security both read as zero rows otherwise.
type catalogEmbeddingAccounts interface {
	GetByID(ctx context.Context, q repository.Querier, accountID uuid.UUID) (*domain.Account, error)
}

// catalogEmbeddingRepository is the persistence surface the backfill needs. Defined here, in
// the consumer, so a test can fake it without a database.
type catalogEmbeddingRepository interface {
	ListPendingEmbedding(ctx context.Context, q repository.Querier, accountID, cursor uuid.UUID,
		refreshAll bool, limit int) ([]domain.ProductEmbeddingInput, error)
	SetEmbeddings(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		embeddings []domain.ProductEmbedding) (int, error)
}

// CatalogEmbeddingService vectorizes an account's catalog so the semantic half of the search
// has something to compare against.
//
// It runs off the request path. A whole catalog is thousands of texts, each provider call is
// bounded per attempt rather than per chain, and the two together outrun any HTTP response
// budget — so this is driven by the catalog-embed command, never by a route.
type CatalogEmbeddingService struct {
	db       tenantTxRunner
	accounts catalogEmbeddingAccounts
	products catalogEmbeddingRepository
	embedder domain.Embedder
	cfg      config.CatalogConfig
}

// NewCatalogEmbeddingService builds a CatalogEmbeddingService.
func NewCatalogEmbeddingService(
	db tenantTxRunner, accounts catalogEmbeddingAccounts, products catalogEmbeddingRepository,
	embedder domain.Embedder, cfg config.CatalogConfig,
) *CatalogEmbeddingService {
	return &CatalogEmbeddingService{
		db: db, accounts: accounts, products: products, embedder: embedder, cfg: cfg,
	}
}

// Backfill embeds every product of the account that has no vector or was edited after the one
// it has. A true refreshAll re-embeds the whole catalog, which is what a change of embedding
// model needs.
//
// The work is paged by product id: each round reads a page, embeds it outside any transaction,
// and writes the vectors back in a short one. A round that fails leaves the pages before it
// stored, so a re-run resumes rather than starting over.
func (s *CatalogEmbeddingService) Backfill(
	ctx context.Context, tenant domain.Tenant, refreshAll bool,
) (domain.CatalogEmbeddingReport, error) {
	var report domain.CatalogEmbeddingReport
	// Checked before any work: an unknown account has nothing pending, which would otherwise
	// report as a catalog already fully embedded and send the operator on to the index command.
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		_, getErr := s.accounts.GetByID(ctx, q, tenant.AccountID)
		return getErr
	}); err != nil {
		return report, err
	}

	cursor := uuid.Nil
	for {
		var pending []domain.ProductEmbeddingInput
		err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
			var readErr error
			pending, readErr = s.products.ListPendingEmbedding(ctx, q, tenant.AccountID, cursor,
				refreshAll, s.cfg.EmbeddingBatchSize)
			return readErr
		})
		if err != nil {
			return report, err
		}
		if len(pending) == 0 {
			return report, nil
		}

		texts := make([]string, len(pending))
		for i, p := range pending {
			texts[i] = p.EmbeddingText()
		}
		vectors, err := s.embedder.Embed(ctx, texts)
		if err != nil {
			return report, err
		}
		if len(vectors) != len(pending) {
			return report, fmt.Errorf("embedder returned %d vectors for %d products",
				len(vectors), len(pending))
		}

		embeddings := make([]domain.ProductEmbedding, len(pending))
		for i, p := range pending {
			embeddings[i] = domain.ProductEmbedding{
				ProductID: p.ProductID, Vector: vectors[i], UpdatedAt: p.UpdatedAt,
			}
		}
		var written int
		err = s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
			var writeErr error
			written, writeErr = s.products.SetEmbeddings(ctx, q, tenant.AccountID, embeddings)
			return writeErr
		})
		if err != nil {
			return report, err
		}
		// Counted after the commit: a rolled-back round stored nothing.
		report.Embedded += written
		report.Rounds++
		// Paging by id rather than by the pending predicate: a refreshAll run keeps matching
		// the rows it just wrote, so only the cursor moves the read forward.
		cursor = pending[len(pending)-1].ProductID
	}
}
