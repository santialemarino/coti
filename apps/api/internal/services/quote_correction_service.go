package services

import (
	"context"
	"errors"
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
	if s.cfg.MaxInterpretationExamples == 0 {
		return nil, nil
	}
	var available bool
	if err := s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
		var checkErr error
		available, checkErr = s.repo.HasReadyInterpretation(ctx, q, tenant.AccountID)
		return checkErr
	}); err != nil || !available {
		return nil, err
	}
	vectors, err := s.embedder.Embed(domain.WithAIOperation(domain.WithAIAccount(ctx, tenant),
		domain.AIOperationInterpretationLookup), []string{raw})
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
	vectors, err := s.embedder.Embed(domain.WithAIOperation(domain.WithAIAccount(ctx, tenant),
		domain.AIOperationCorrectionLearning), texts)
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
	var failures error
	// One provider call per account, so every call's spend belongs to exactly one account.
	for _, batch := range byAccount(memories) {
		changed, err := j.embedAccount(ctx, q, batch)
		report.Changed += changed
		failures = errors.Join(failures, err)
	}
	return report, failures
}

// embedAccount vectorizes one account's pending memories and publishes them.
func (j *QuoteCorrectionJob) embedAccount(ctx context.Context, q repository.Querier,
	memories []domain.QuoteCorrectionMemory) (int, error) {
	accountID := memories[0].AccountID
	texts := make([]string, len(memories))
	for i := range memories {
		texts[i] = memories[i].SourceText
	}
	vectors, err := j.embedder.Embed(domain.WithAIOperation(
		domain.WithAIAccount(ctx, domain.Tenant{AccountID: accountID}),
		domain.AIOperationCorrectionLearning), texts)
	if err != nil {
		for _, memory := range memories {
			_ = j.repo.RecordFailure(ctx, q, accountID, memory.ID, err.Error())
		}
		return 0, fmt.Errorf("account %s: %w", accountID, err)
	}
	if len(vectors) != len(memories) {
		return 0, fmt.Errorf("account %s: embedder returned %d vectors for %d corrections",
			accountID, len(vectors), len(memories))
	}
	for i, memory := range memories {
		if err := j.repo.MarkReady(ctx, q, accountID, memory.ID, vectors[i]); err != nil {
			return i, fmt.Errorf("account %s: %w", accountID, err)
		}
	}
	return len(memories), nil
}

// byAccount splits memories into one batch per account, each in the order it was listed.
func byAccount(memories []domain.QuoteCorrectionMemory) [][]domain.QuoteCorrectionMemory {
	index := make(map[uuid.UUID]int)
	var batches [][]domain.QuoteCorrectionMemory
	for _, memory := range memories {
		i, ok := index[memory.AccountID]
		if !ok {
			i = len(batches)
			index[memory.AccountID] = i
			batches = append(batches, nil)
		}
		batches[i] = append(batches[i], memory)
	}
	return batches
}

var _ Job = (*QuoteCorrectionJob)(nil)
