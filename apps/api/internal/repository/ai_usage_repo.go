package repository

import (
	"context"
	"fmt"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// AIUsageRepository owns the append-only ledger of AI provider spend.
type AIUsageRepository struct{}

// NewAIUsageRepository builds an AIUsageRepository.
func NewAIUsageRepository() *AIUsageRepository {
	return &AIUsageRepository{}
}

// Create appends one provider call to the ledger of its account.
func (r *AIUsageRepository) Create(ctx context.Context, q Querier, usage domain.AIUsage) error {
	if _, err := q.Exec(ctx,
		`INSERT INTO ai_usage (
		   account_id, branch_id, rfq_id, operation, provider, model, succeeded, attempts,
		   elapsed_ms, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens,
		   audio_seconds
		 ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, round($14::numeric, 2))`,
		usage.AccountID, usage.BranchID, usage.RFQID, string(usage.Operation), usage.Provider,
		usage.Model, usage.Succeeded, usage.Attempts, usage.Elapsed.Milliseconds(),
		usage.InputTokens, usage.OutputTokens, usage.CacheReadTokens, usage.CacheWriteTokens,
		usage.AudioSeconds,
	); err != nil {
		return fmt.Errorf("insert ai usage: %w", err)
	}
	return nil
}
