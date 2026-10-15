package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const messageBatchColumns = `id, account_id, quote_id, status, opened_at, closes_at, closed_at,
	processed_at, created_at`

// MessageBatchRepository owns conversation windows and their processing queue.
type MessageBatchRepository struct{}

// NewMessageBatchRepository builds a MessageBatchRepository.
func NewMessageBatchRepository() *MessageBatchRepository { return &MessageBatchRepository{} }

// OpenOrExtend opens the quote's window at now, or pushes its close to now+reset without passing
// opened_at+limit. uq_message_batch_open makes concurrent callers share one OPEN window. Returns
// domain.ErrNotFound when the quote is not in the branch.
func (r *MessageBatchRepository) OpenOrExtend(ctx context.Context, q Querier, accountID,
	branchID, quoteID uuid.UUID, now time.Time, reset, limit time.Duration,
) (*domain.MessageBatch, error) {
	batch, err := scanMessageBatch(q.QueryRow(ctx, `INSERT INTO message_batch
		(account_id, quote_id, status, opened_at, closes_at)
		SELECT quote.account_id, quote.id, 'OPEN', $4::timestamptz,
		  LEAST($4::timestamptz + $5::interval, $4::timestamptz + $6::interval)
		FROM quote
		WHERE quote.account_id = $1 AND quote.branch_id = $2 AND quote.id = $3
		ON CONFLICT (quote_id) WHERE status = 'OPEN' DO UPDATE
		SET closes_at = LEAST($4::timestamptz + $5::interval, message_batch.opened_at + $6::interval)
		RETURNING `+messageBatchColumns,
		accountID, branchID, quoteID, now, reset, limit))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return batch, err
}

func scanMessageBatch(row pgx.Row) (*domain.MessageBatch, error) {
	var batch domain.MessageBatch
	err := row.Scan(&batch.ID, &batch.AccountID, &batch.QuoteID, &batch.Status, &batch.OpenedAt,
		&batch.ClosesAt, &batch.ClosedAt, &batch.ProcessedAt, &batch.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &batch, nil
}
