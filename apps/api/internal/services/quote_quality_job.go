package services

import (
	"context"
	"errors"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type pendingQuoteEvaluationRepository interface {
	ListPendingEvaluations(ctx context.Context, q repository.Querier, evaluatorVersion string,
		limit int) ([]domain.PendingQuoteEvaluation, error)
}

// QuoteQualityJob retries deterministic evaluations missing after a committed delivery.
type QuoteQualityJob struct {
	repo      pendingQuoteEvaluationRepository
	evaluator QuoteQualityEvaluator
	cfg       config.QuoteQualityConfig
}

// NewQuoteQualityJob builds the durable post-send evaluation retry.
func NewQuoteQualityJob(repo pendingQuoteEvaluationRepository, evaluator QuoteQualityEvaluator,
	cfg config.QuoteQualityConfig) *QuoteQualityJob {
	return &QuoteQualityJob{repo: repo, evaluator: evaluator, cfg: cfg}
}

// Name identifies the scheduled quote-quality retry.
func (j *QuoteQualityJob) Name() string { return "quote-quality-evaluation" }

// Run evaluates one bounded batch and leaves failures eligible for a later firing.
func (j *QuoteQualityJob) Run(ctx context.Context,
	q repository.Querier) (domain.JobReport, error) {
	candidates, err := j.repo.ListPendingEvaluations(ctx, q, WholeQuoteEvaluatorVersion,
		j.cfg.ProcessingBatchSize)
	if err != nil {
		return domain.JobReport{}, err
	}
	report := domain.JobReport{Scanned: len(candidates)}
	var failures error
	for _, candidate := range candidates {
		tenant := domain.Tenant{AccountID: candidate.AccountID, BranchID: candidate.BranchID}
		if _, err := j.evaluator.EvaluateFinalQuote(ctx, tenant, candidate.QuoteID,
			candidate.VersionID); err != nil {
			failures = errors.Join(failures, err)
			continue
		}
		report.Changed++
	}
	return report, failures
}

var _ Job = (*QuoteQualityJob)(nil)
