package services

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type pendingEmbeddingAccounts interface {
	ListAccountsPendingEmbedding(ctx context.Context, q repository.Querier) ([]uuid.UUID, error)
}

type catalogBackfiller interface {
	Backfill(ctx context.Context, tenant domain.Tenant,
		refreshAll bool) (domain.CatalogEmbeddingReport, error)
}

// CatalogEmbeddingJob embeds the products created or edited since the last run, so a product
// matches on its meaning from its first order rather than on its text alone.
type CatalogEmbeddingJob struct {
	accounts   pendingEmbeddingAccounts
	backfiller catalogBackfiller
	enabled    bool
}

// NewCatalogEmbeddingJob builds the scheduled catalog embedding. With embeddings switched off the
// search reads no vectors, so a disabled job does nothing rather than spend on them or fail.
func NewCatalogEmbeddingJob(accounts pendingEmbeddingAccounts, backfiller catalogBackfiller,
	enabled bool) *CatalogEmbeddingJob {
	return &CatalogEmbeddingJob{accounts: accounts, backfiller: backfiller, enabled: enabled}
}

// Name identifies the scheduled catalog embedding.
func (j *CatalogEmbeddingJob) Name() string { return "catalog-embedding" }

// Run embeds each pending account's catalog in turn: it scans accounts and changes products. An
// account that fails is left for the next firing and does not hold up the others.
func (j *CatalogEmbeddingJob) Run(ctx context.Context,
	q repository.Querier) (domain.JobReport, error) {
	if !j.enabled {
		return domain.JobReport{}, nil
	}
	accounts, err := j.accounts.ListAccountsPendingEmbedding(ctx, q)
	if err != nil {
		return domain.JobReport{}, err
	}
	report := domain.JobReport{Scanned: len(accounts)}
	var failures error
	for _, accountID := range accounts {
		embedded, err := j.backfiller.Backfill(ctx, domain.Tenant{AccountID: accountID}, false)
		report.Changed += embedded.Embedded
		if err != nil {
			failures = errors.Join(failures, fmt.Errorf("account %s: %w", accountID, err))
		}
	}
	return report, failures
}

var _ Job = (*CatalogEmbeddingJob)(nil)
