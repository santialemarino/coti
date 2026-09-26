package services

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

var _ domain.AIUsageRecorder = (*AIUsageService)(nil)

type aiUsageRepository interface {
	Create(ctx context.Context, q repository.Querier, usage domain.AIUsage) error
}

// AIUsageService keeps the ledger of AI provider spend.
type AIUsageService struct {
	db           tenantTxRunner
	repo         aiUsageRepository
	writeTimeout time.Duration
	log          *slog.Logger
}

// NewAIUsageService builds an AIUsageService.
func NewAIUsageService(db tenantTxRunner, repo aiUsageRepository, writeTimeout time.Duration,
	log *slog.Logger,
) *AIUsageService {
	if log == nil {
		log = slog.Default()
	}
	return &AIUsageService{db: db, repo: repo, writeTimeout: writeTimeout, log: log}
}

// Record appends one call to its account's ledger, in a transaction of its own: a pipeline that
// rolls back still spent what it spent, and a caller that gave up still paid for the call.
func (s *AIUsageService) Record(ctx context.Context, usage domain.AIUsage) {
	if usage.AccountID == uuid.Nil || usage.Operation == "" {
		// Every call site names both, so this is a missing attribution: loud, never silent.
		s.log.ErrorContext(ctx, "ai usage without attribution",
			slog.String("account_id", usage.AccountID.String()),
			slog.String("operation", string(usage.Operation)),
			slog.String("model", usage.Model))
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.writeTimeout)
	defer cancel()
	if err := s.db.InTenantTx(ctx, domain.Tenant{AccountID: usage.AccountID},
		func(q repository.Querier) error {
			return s.repo.Create(ctx, q, usage)
		}); err != nil {
		s.log.ErrorContext(ctx, "could not record ai usage",
			slog.String("account_id", usage.AccountID.String()),
			slog.String("operation", string(usage.Operation)),
			slog.Any("error", err))
	}
}
