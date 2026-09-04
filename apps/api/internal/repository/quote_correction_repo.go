package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/pgvector/pgvector-go"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// QuoteCorrectionRepository persists and retrieves bounded account-local learning evidence.
type QuoteCorrectionRepository struct{}

// NewQuoteCorrectionRepository builds a QuoteCorrectionRepository.
func NewQuoteCorrectionRepository() *QuoteCorrectionRepository { return &QuoteCorrectionRepository{} }

// Enqueue stores each correction idempotently and returns memories that still need embeddings.
func (r *QuoteCorrectionRepository) Enqueue(ctx context.Context, q Querier, accountID,
	evaluationID uuid.UUID, patterns []domain.NewQuoteCorrectionMemory, maxPatterns int,
) ([]domain.QuoteCorrectionMemory, error) {
	pending := make([]domain.QuoteCorrectionMemory, 0, len(patterns))
	for _, pattern := range patterns {
		normalized := normalizeCorrectionSource(pattern.SourceText)
		payload, err := json.Marshal(pattern.CorrectedItems)
		if err != nil {
			return nil, err
		}
		var row pgx.Row
		if pattern.Kind == domain.QuoteCorrectionMemoryInterpretation {
			row = q.QueryRow(ctx, `INSERT INTO quote_correction_memory
			  (account_id, kind, source_text, normalized_source, corrected_items, support_count)
			 VALUES ($1, $2, $3, $4, $5, 0)
			 ON CONFLICT (account_id, normalized_source) WHERE kind = 'INTERPRETATION'
			 DO UPDATE SET source_text = EXCLUDED.source_text,
			   corrected_items = EXCLUDED.corrected_items, last_seen_at = now()
			 RETURNING id, account_id, kind, source_text, corrected_items, product_id,
			   embedding, created_at`, accountID, pattern.Kind, pattern.SourceText, normalized, payload)
		} else {
			row = q.QueryRow(ctx, `INSERT INTO quote_correction_memory
			  (account_id, kind, source_text, normalized_source, product_id, support_count)
			 VALUES ($1, $2, $3, $4, $5, 0)
			 ON CONFLICT (account_id, normalized_source, product_id) WHERE kind = 'CATALOG'
			 DO UPDATE SET source_text = EXCLUDED.source_text, last_seen_at = now()
			 RETURNING id, account_id, kind, source_text, corrected_items, product_id,
			   embedding, created_at`, accountID, pattern.Kind, pattern.SourceText, normalized,
				pattern.ProductID)
		}
		memory, err := scanCorrectionMemory(row)
		if err != nil {
			return nil, err
		}
		var inserted int
		err = q.QueryRow(ctx, `WITH added AS (
		  INSERT INTO quote_correction_memory_source (account_id, evaluation_id, memory_id, source_key)
		  VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING RETURNING 1
		) SELECT count(*) FROM added`, accountID, evaluationID, memory.ID, pattern.SourceKey).Scan(&inserted)
		if err != nil {
			return nil, err
		}
		if inserted == 1 {
			if _, err = q.Exec(ctx, `UPDATE quote_correction_memory SET support_count = support_count + 1
			 WHERE account_id = $1 AND id = $2`, accountID, memory.ID); err != nil {
				return nil, err
			}
		}
		if memory.Embedding == nil {
			pending = append(pending, *memory)
		}
	}
	_, err := q.Exec(ctx, `DELETE FROM quote_correction_memory memory
	 WHERE memory.account_id = $1 AND memory.id IN (
	   SELECT id FROM quote_correction_memory WHERE account_id = $1
	   ORDER BY support_count DESC, COALESCE(last_used_at, last_seen_at) DESC, id
	   OFFSET $2
	 )`, accountID, maxPatterns)
	return pending, err
}

// ListPending returns unvectorized memories across accounts for the owner-run retry job.
func (r *QuoteCorrectionRepository) ListPending(ctx context.Context, q Querier,
	limit int) ([]domain.QuoteCorrectionMemory, error) {
	rows, err := q.Query(ctx, `SELECT id, account_id, kind, source_text, corrected_items,
	 product_id, embedding, created_at FROM quote_correction_memory
	 WHERE status = 'PENDING' ORDER BY created_at, id LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var memories []domain.QuoteCorrectionMemory
	for rows.Next() {
		memory, scanErr := scanCorrectionMemory(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		memories = append(memories, *memory)
	}
	return memories, rows.Err()
}

// MarkReady makes one successfully embedded correction available to retrieval.
func (r *QuoteCorrectionRepository) MarkReady(ctx context.Context, q Querier, accountID,
	id uuid.UUID, embedding pgvector.Vector) error {
	tag, err := q.Exec(ctx, `UPDATE quote_correction_memory SET embedding = $3,
	 status = 'READY', last_error = NULL WHERE account_id = $1 AND id = $2`, accountID, id,
		embedding)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrNotFound
	}
	return nil
}

// RecordFailure preserves the pending state and its latest operational failure.
func (r *QuoteCorrectionRepository) RecordFailure(ctx context.Context, q Querier, accountID,
	id uuid.UUID, message string) error {
	_, err := q.Exec(ctx, `UPDATE quote_correction_memory SET last_error = $3
	 WHERE account_id = $1 AND id = $2 AND status = 'PENDING'`, accountID, id, message)
	return err
}

// HasReadyInterpretation avoids a provider call before an account has anything to retrieve.
func (r *QuoteCorrectionRepository) HasReadyInterpretation(ctx context.Context, q Querier,
	accountID uuid.UUID) (bool, error) {
	var found bool
	err := q.QueryRow(ctx, `SELECT EXISTS (
	 SELECT 1 FROM quote_correction_memory WHERE account_id = $1
	   AND kind = 'INTERPRETATION' AND status = 'READY'
	)`, accountID).Scan(&found)
	return found, err
}

// FindInterpretationExamples returns close, ready examples from the same account.
func (r *QuoteCorrectionRepository) FindInterpretationExamples(ctx context.Context, q Querier,
	accountID uuid.UUID, embedding pgvector.Vector, maxDistance float64,
	limit int) ([]domain.RFQInterpretationExample, error) {
	rows, err := q.Query(ctx, `WITH selected AS MATERIALIZED (
	 SELECT id, source_text, corrected_items, embedding <=> $2 AS distance
	 FROM quote_correction_memory WHERE account_id = $1 AND kind = 'INTERPRETATION'
	   AND status = 'READY' AND embedding <=> $2 <= $3
	 ORDER BY embedding <=> $2, support_count DESC LIMIT $4
	), touched AS (
	 UPDATE quote_correction_memory memory SET use_count = use_count + 1, last_used_at = now()
	 FROM selected WHERE memory.account_id = $1 AND memory.id = selected.id RETURNING memory.id
	)
	SELECT selected.source_text, selected.corrected_items FROM selected
	JOIN touched ON touched.id = selected.id ORDER BY selected.distance`, accountID, embedding,
		maxDistance, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var examples []domain.RFQInterpretationExample
	for rows.Next() {
		var example domain.RFQInterpretationExample
		var payload []byte
		if err := rows.Scan(&example.SourceText, &payload); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(payload, &example.CorrectedItems); err != nil {
			return nil, err
		}
		examples = append(examples, example)
	}
	return examples, rows.Err()
}

func normalizeCorrectionSource(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func scanCorrectionMemory(row pgx.Row) (*domain.QuoteCorrectionMemory, error) {
	var memory domain.QuoteCorrectionMemory
	var payload []byte
	err := row.Scan(&memory.ID, &memory.AccountID, &memory.Kind, &memory.SourceText, &payload,
		&memory.ProductID, &memory.Embedding, &memory.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(payload) > 0 {
		if err := json.Unmarshal(payload, &memory.CorrectedItems); err != nil {
			return nil, err
		}
	}
	return &memory, nil
}
