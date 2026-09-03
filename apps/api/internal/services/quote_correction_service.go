package services

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/pgvector/pgvector-go"

	"github.com/santialemarino/coti/apps/api/internal/config"
	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type quoteCorrectionRepository interface {
	Enqueue(ctx context.Context, q repository.Querier, accountID,
		evaluationID uuid.UUID, patterns []domain.NewQuoteCorrectionMemory,
		maxPatterns int) ([]domain.QuoteCorrectionMemory, error)
	ListPending(ctx context.Context, q repository.Querier, limit int) ([]domain.QuoteCorrectionMemory, error)
	MarkReady(ctx context.Context, q repository.Querier, accountID,
		id uuid.UUID, embedding pgvector.Vector) error
	RecordFailure(ctx context.Context, q repository.Querier, accountID,
		id uuid.UUID, message string) error
	FindInterpretationExamples(ctx context.Context, q repository.Querier, accountID uuid.UUID,
		embedding pgvector.Vector, maxDistance float64,
		limit int) ([]domain.RFQInterpretationExample, error)
	HasReadyInterpretation(ctx context.Context, q repository.Querier,
		accountID uuid.UUID) (bool, error)
}

// QuoteCorrectionService materializes seller corrections and retrieves local examples.
type QuoteCorrectionService struct {
	db       tenantTxRunner
	repo     quoteCorrectionRepository
	embedder domain.Embedder
	cfg      config.QuoteCorrectionConfig
	log      *slog.Logger
}

// Enqueue persists correction evidence inside the caller's evaluation transaction.
func (s *QuoteCorrectionService) Enqueue(ctx context.Context, q repository.Querier,
	accountID, evaluationID uuid.UUID,
	patterns []domain.NewQuoteCorrectionMemory) ([]domain.QuoteCorrectionMemory, error) {
	return s.repo.Enqueue(ctx, q, accountID, evaluationID, patterns,
		s.cfg.MaxPatternsPerAccount)
}

// NewQuoteCorrectionService builds the account-local learning service.
func NewQuoteCorrectionService(db tenantTxRunner, repo quoteCorrectionRepository,
	embedder domain.Embedder, cfg config.QuoteCorrectionConfig, log *slog.Logger,
) *QuoteCorrectionService {
	if log == nil {
		log = slog.Default()
	}
	return &QuoteCorrectionService{db: db, repo: repo, embedder: embedder, cfg: cfg, log: log}
}

// FindInterpretationExamples finds previous seller corrections similar to one new order.
func (s *QuoteCorrectionService) FindInterpretationExamples(ctx context.Context,
	tenant domain.Tenant, raw string) ([]domain.RFQInterpretationExample, error) {
	var available bool
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var checkErr error
		available, checkErr = s.repo.HasReadyInterpretation(ctx, q, tenant.AccountID)
		return checkErr
	}); err != nil || !available {
		return nil, err
	}
	vectors, err := s.embedder.Embed(ctx, []string{raw})
	if err != nil {
		return nil, err
	}
	if len(vectors) != 1 {
		return nil, fmt.Errorf("embedder returned %d vectors for one order", len(vectors))
	}
	var examples []domain.RFQInterpretationExample
	err = s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var findErr error
		examples, findErr = s.repo.FindInterpretationExamples(ctx, q, tenant.AccountID,
			vectors[0], 1-float64(s.cfg.SimilarityPercent)/100,
			s.cfg.MaxInterpretationExamples)
		return findErr
	})
	return examples, err
}

// Process materializes pending memories without making the completed send depend on success.
func (s *QuoteCorrectionService) Process(ctx context.Context, tenant domain.Tenant,
	memories []domain.QuoteCorrectionMemory) {
	if len(memories) == 0 {
		return
	}
	texts := make([]string, len(memories))
	for i := range memories {
		texts[i] = memories[i].SourceText
	}
	vectors, err := s.embedder.Embed(ctx, texts)
	if err != nil || len(vectors) != len(memories) {
		message := fmt.Sprintf("embedding failed: %v", err)
		if err == nil {
			message = fmt.Sprintf("embedder returned %d vectors for %d corrections", len(vectors), len(memories))
		}
		_ = s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
			for _, memory := range memories {
				if recordErr := s.repo.RecordFailure(ctx, q, tenant.AccountID, memory.ID, message); recordErr != nil {
					return recordErr
				}
			}
			return nil
		})
		s.log.WarnContext(ctx, "quote correction remains pending", slog.String("reason", message))
		return
	}
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		for i, memory := range memories {
			if markErr := s.repo.MarkReady(ctx, q, tenant.AccountID, memory.ID, vectors[i]); markErr != nil {
				return markErr
			}
		}
		return nil
	}); err != nil {
		s.log.ErrorContext(ctx, "could not publish quote correction memory", slog.Any("error", err))
	}
}

// QuoteCorrectionJob retries pending correction embeddings across accounts.
type QuoteCorrectionJob struct {
	repo     quoteCorrectionRepository
	embedder domain.Embedder
	cfg      config.QuoteCorrectionConfig
}

// NewQuoteCorrectionJob builds the durable retry job.
func NewQuoteCorrectionJob(repo quoteCorrectionRepository, embedder domain.Embedder,
	cfg config.QuoteCorrectionConfig) *QuoteCorrectionJob {
	return &QuoteCorrectionJob{repo: repo, embedder: embedder, cfg: cfg}
}

// Name identifies the scheduled correction-learning retry.
func (j *QuoteCorrectionJob) Name() string { return "quote-correction-learning" }

// Run vectorizes one bounded batch; later firings continue where this one stopped.
func (j *QuoteCorrectionJob) Run(ctx context.Context, q repository.Querier) (domain.JobReport, error) {
	memories, err := j.repo.ListPending(ctx, q, j.cfg.ProcessingBatchSize)
	if err != nil {
		return domain.JobReport{}, err
	}
	report := domain.JobReport{Scanned: len(memories)}
	if len(memories) == 0 {
		return report, nil
	}
	texts := make([]string, len(memories))
	for i := range memories {
		texts[i] = memories[i].SourceText
	}
	vectors, err := j.embedder.Embed(ctx, texts)
	if err != nil {
		for _, memory := range memories {
			_ = j.repo.RecordFailure(ctx, q, memory.AccountID, memory.ID, err.Error())
		}
		return report, err
	}
	if len(vectors) != len(memories) {
		return report, fmt.Errorf("embedder returned %d vectors for %d corrections", len(vectors), len(memories))
	}
	for i, memory := range memories {
		if err := j.repo.MarkReady(ctx, q, memory.AccountID, memory.ID, vectors[i]); err != nil {
			return report, err
		}
		report.Changed++
	}
	return report, nil
}

var _ Job = (*QuoteCorrectionJob)(nil)
