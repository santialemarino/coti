package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const messageBatchColumns = `id, account_id, quote_id, status, opened_at, closes_at, closed_at,
	processed_at, created_at, attempts, last_error, processing_started_at`

// messageBatchProcessingIndex keeps one PROCESSING batch per quote.
const messageBatchProcessingIndex = "uq_message_batch_processing"

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

// CloseOpen closes the quote's OPEN window now, on the database's clock like every close, and
// stamps every unbatched CLIENT message of the quote with it. The caller holds the quote's lock.
// Returns domain.ErrNotFound when the quote has no OPEN window.
func (r *MessageBatchRepository) CloseOpen(ctx context.Context, q Querier, accountID,
	quoteID uuid.UUID,
) (*domain.MessageBatch, error) {
	batch, err := scanMessageBatch(q.QueryRow(ctx, `WITH closed AS (
		  UPDATE message_batch
		  SET status = 'CLOSED', closes_at = LEAST(closes_at, now()), closed_at = now()
		  WHERE account_id = $1 AND quote_id = $2 AND status = 'OPEN'
		  RETURNING `+messageBatchColumns+`),
		stamped AS (
		  UPDATE quote_message SET batch_id = closed.id
		  FROM closed
		  WHERE quote_message.account_id = closed.account_id
		    AND quote_message.quote_id = closed.quote_id
		    AND quote_message.author_type = 'CLIENT' AND quote_message.batch_id IS NULL)
		SELECT `+messageBatchColumns+` FROM closed`,
		accountID, quoteID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return batch, err
}

// LockDueQuotesCrossAccount locks, across every account, the quotes of up to limit OPEN windows
// whose closes_at is at or before now. A quote another transaction holds is skipped. The quote is
// locked before its window, the order every writer of a conversation follows.
func (r *MessageBatchRepository) LockDueQuotesCrossAccount(ctx context.Context, q Querier,
	now time.Time, limit int,
) ([]uuid.UUID, error) {
	rows, err := q.Query(ctx, `WITH due AS MATERIALIZED (
		  SELECT account_id, quote_id FROM message_batch
		  WHERE status = 'OPEN' AND closes_at <= $1
		  ORDER BY closes_at, id
		  LIMIT $2)
		SELECT quote.id FROM quote
		JOIN due ON due.quote_id = quote.id AND due.account_id = quote.account_id
		FOR UPDATE OF quote SKIP LOCKED`,
		now, limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// CloseDueCrossAccount closes the OPEN windows of quoteIDs still due at now and stamps each one's
// unbatched CLIENT messages with it. closed_at is the database's clock, so FIFO order never
// depends on which instance closed a window. The caller holds the quotes' locks.
func (r *MessageBatchRepository) CloseDueCrossAccount(ctx context.Context, q Querier,
	quoteIDs []uuid.UUID, now time.Time,
) ([]domain.MessageBatch, error) {
	return queryMessageBatches(ctx, q, `WITH closed AS (
		  UPDATE message_batch SET status = 'CLOSED', closed_at = now()
		  WHERE quote_id = ANY($1::uuid[]) AND status = 'OPEN' AND closes_at <= $2
		  RETURNING `+messageBatchColumns+`),
		stamped AS (
		  UPDATE quote_message SET batch_id = closed.id
		  FROM closed
		  WHERE quote_message.account_id = closed.account_id
		    AND quote_message.quote_id = closed.quote_id
		    AND quote_message.author_type = 'CLIENT' AND quote_message.batch_id IS NULL)
		SELECT `+messageBatchColumns+` FROM closed ORDER BY closes_at, id`,
		quoteIDs, now)
}

// ReclaimStuckCrossAccount takes back up to limit batches claimed before startedBefore, across
// every account: each one counts the lost attempt and returns to CLOSED, or FAILED once attempts
// reaches maxAttempts.
func (r *MessageBatchRepository) ReclaimStuckCrossAccount(ctx context.Context, q Querier,
	startedBefore time.Time, maxAttempts, limit int, failure string,
) ([]domain.MessageBatch, error) {
	return queryMessageBatches(ctx, q, `WITH stuck AS MATERIALIZED (
		  SELECT id FROM message_batch
		  WHERE status = 'PROCESSING' AND processing_started_at < $1
		  ORDER BY processing_started_at, id
		  LIMIT $3
		  FOR UPDATE SKIP LOCKED)
		UPDATE message_batch
		SET attempts = message_batch.attempts + 1, last_error = $4,
		    status = CASE WHEN message_batch.attempts + 1 >= $2 THEN 'FAILED'::message_batch_status
		                  ELSE 'CLOSED'::message_batch_status END
		FROM stuck WHERE message_batch.id = stuck.id
		RETURNING `+qualifiedMessageBatchColumns("message_batch"),
		startedBefore, maxAttempts, limit, failure)
}

// ClaimNextCrossAccount moves the oldest claimable CLOSED batch to PROCESSING, across every
// account. A batch is claimable when it heads its quote's queue by (closed_at, id) and its quote
// has no PROCESSING batch; skip excludes batches already tried this pass. Returns nil when none is
// claimable, and domain.ErrConflict when a concurrent claim on the same quote won the race.
func (r *MessageBatchRepository) ClaimNextCrossAccount(ctx context.Context, q Querier,
	now time.Time, skip []uuid.UUID,
) (*domain.MessageBatch, error) {
	if skip == nil {
		skip = []uuid.UUID{}
	}
	batch, err := scanMessageBatch(q.QueryRow(ctx, `WITH next AS MATERIALIZED (
		  SELECT candidate.id FROM message_batch candidate
		  WHERE candidate.status = 'CLOSED' AND candidate.id <> ALL($2::uuid[])
		    AND NOT EXISTS (
		      SELECT 1 FROM message_batch busy
		      WHERE busy.quote_id = candidate.quote_id AND busy.status = 'PROCESSING')
		    AND NOT EXISTS (
		      SELECT 1 FROM message_batch earlier
		      WHERE earlier.quote_id = candidate.quote_id AND earlier.status = 'CLOSED'
		        AND (earlier.closed_at, earlier.id) < (candidate.closed_at, candidate.id))
		  ORDER BY candidate.closed_at, candidate.id
		  LIMIT 1
		  FOR UPDATE OF candidate SKIP LOCKED)
		UPDATE message_batch SET status = 'PROCESSING', processing_started_at = $1
		FROM next WHERE message_batch.id = next.id
		RETURNING `+qualifiedMessageBatchColumns("message_batch"),
		now, skip))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if isUniqueViolation(err, messageBatchProcessingIndex) {
		return nil, domain.ErrConflict
	}
	return batch, err
}

// LockClaim locks a batch still held by the claim batch.ProcessingStartedAt dates, for the rest of
// the caller's transaction. Returns domain.ErrConflict when the claim was lost.
func (r *MessageBatchRepository) LockClaim(ctx context.Context, q Querier,
	batch domain.MessageBatch,
) error {
	var id uuid.UUID
	err := q.QueryRow(ctx, `SELECT id FROM message_batch
		WHERE id = $1 AND account_id = $2 AND status = 'PROCESSING' AND processing_started_at = $3
		FOR UPDATE`,
		batch.ID, batch.AccountID, batch.ProcessingStartedAt).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: batch %s is no longer claimed", domain.ErrConflict, batch.ID)
	}
	return err
}

// MarkProcessedCrossAccount finishes a claim as PROCESSED. Returns false when the claim was lost:
// the batch was reclaimed, or claimed again since batch.ProcessingStartedAt.
func (r *MessageBatchRepository) MarkProcessedCrossAccount(ctx context.Context, q Querier,
	batch domain.MessageBatch, now time.Time,
) (bool, error) {
	tag, err := q.Exec(ctx, `UPDATE message_batch SET status = 'PROCESSED', processed_at = $4
		WHERE id = $1 AND account_id = $2 AND status = 'PROCESSING' AND processing_started_at = $3`,
		batch.ID, batch.AccountID, batch.ProcessingStartedAt, now)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// RecordFailureCrossAccount counts a failed attempt on a claim and returns the batch to CLOSED, or
// to FAILED once attempts reaches maxAttempts. Returns nil when the claim was lost.
func (r *MessageBatchRepository) RecordFailureCrossAccount(ctx context.Context, q Querier,
	batch domain.MessageBatch, maxAttempts int, failure string,
) (*domain.MessageBatch, error) {
	updated, err := scanMessageBatch(q.QueryRow(ctx, `UPDATE message_batch
		SET attempts = attempts + 1, last_error = $5,
		    status = CASE WHEN attempts + 1 >= $4 THEN 'FAILED'::message_batch_status
		                  ELSE 'CLOSED'::message_batch_status END
		WHERE id = $1 AND account_id = $2 AND status = 'PROCESSING' AND processing_started_at = $3
		RETURNING `+messageBatchColumns,
		batch.ID, batch.AccountID, batch.ProcessingStartedAt, maxAttempts, failure))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return updated, err
}

// ReleaseCrossAccount returns a claim to CLOSED without counting an attempt, for work interrupted
// by a shutdown. Returns false when the claim was lost.
func (r *MessageBatchRepository) ReleaseCrossAccount(ctx context.Context, q Querier,
	batch domain.MessageBatch,
) (bool, error) {
	tag, err := q.Exec(ctx, `UPDATE message_batch SET status = 'CLOSED'
		WHERE id = $1 AND account_id = $2 AND status = 'PROCESSING' AND processing_started_at = $3`,
		batch.ID, batch.AccountID, batch.ProcessingStartedAt)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func qualifiedMessageBatchColumns(table string) string {
	columns := strings.Split(messageBatchColumns, ",")
	for i, column := range columns {
		columns[i] = fmt.Sprintf("%s.%s", table, strings.TrimSpace(column))
	}
	return strings.Join(columns, ", ")
}

func queryMessageBatches(ctx context.Context, q Querier, sql string, args ...any,
) ([]domain.MessageBatch, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	batches := make([]domain.MessageBatch, 0)
	for rows.Next() {
		batch, err := scanMessageBatch(rows)
		if err != nil {
			return nil, err
		}
		batches = append(batches, *batch)
	}
	return batches, rows.Err()
}

func scanMessageBatch(row pgx.Row) (*domain.MessageBatch, error) {
	var batch domain.MessageBatch
	err := row.Scan(&batch.ID, &batch.AccountID, &batch.QuoteID, &batch.Status, &batch.OpenedAt,
		&batch.ClosesAt, &batch.ClosedAt, &batch.ProcessedAt, &batch.CreatedAt, &batch.Attempts,
		&batch.LastError, &batch.ProcessingStartedAt)
	if err != nil {
		return nil, err
	}
	return &batch, nil
}
