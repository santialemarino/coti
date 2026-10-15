package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// HandlerDecisionRepository owns persistence for proposed customer-message changes.
type HandlerDecisionRepository struct{}

// NewHandlerDecisionRepository builds a HandlerDecisionRepository.
func NewHandlerDecisionRepository() *HandlerDecisionRepository { return &HandlerDecisionRepository{} }

// ListReviewsByQuote returns the handler run on each batch of a quote's conversation, keyed by batch.
func (r *HandlerDecisionRepository) ListReviewsByQuote(ctx context.Context, q Querier,
	accountID, quoteID uuid.UUID,
) (map[uuid.UUID]domain.HandlerDecisionReview, error) {
	rows, err := q.Query(ctx,
		`SELECT decision.message_batch_id, decision.quote_version_id,
		        COALESCE(decision.ai_interpretation, ''),
		        decision.ai_proposal::jsonb ->> 'outcome',
		        decision.ai_proposal::jsonb ->> 'no_match_reason',
		        decision.seller_decision, decision.decided_at, decision.created_at
		 FROM handler_decision decision
		 JOIN quote_version version
		   ON version.account_id = decision.account_id AND version.id = decision.quote_version_id
		 WHERE decision.account_id = $1 AND version.quote_id = $2
		   AND decision.message_batch_id IS NOT NULL
		 ORDER BY decision.created_at, decision.id`,
		accountID, quoteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reviews := make(map[uuid.UUID]domain.HandlerDecisionReview)
	for rows.Next() {
		var batchID uuid.UUID
		var review domain.HandlerDecisionReview
		var outcome *string
		if err := rows.Scan(&batchID, &review.VersionID, &review.Interpretation, &outcome,
			&review.NoMatchReason,
			&review.SellerDecision, &review.DecidedAt, &review.CreatedAt); err != nil {
			return nil, err
		}
		review.Outcome = domain.HandlerOutcomeApplied
		if outcome != nil {
			review.Outcome = domain.HandlerOutcome(*outcome)
		}
		reviews[batchID] = review
	}
	return reviews, rows.Err()
}

// ListTracesByVersion returns every run recorded on a version, in the order they happened.
func (r *HandlerDecisionRepository) ListTracesByVersion(ctx context.Context, q Querier,
	accountID, versionID uuid.UUID,
) ([]domain.HandlerDecisionTrace, error) {
	rows, err := q.Query(ctx,
		`SELECT id, ai_proposal::jsonb ->> 'outcome', seller_decision, draft_before, draft_after
		 FROM handler_decision
		 WHERE account_id = $1 AND quote_version_id = $2
		 ORDER BY created_at, id`,
		accountID, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	traces := make([]domain.HandlerDecisionTrace, 0)
	for rows.Next() {
		var trace domain.HandlerDecisionTrace
		var outcome *string
		var before, after []byte
		if err := rows.Scan(&trace.ID, &outcome, &trace.SellerDecision, &before, &after); err != nil {
			return nil, err
		}
		trace.Outcome = domain.HandlerOutcomeApplied
		if outcome != nil {
			trace.Outcome = domain.HandlerOutcome(*outcome)
		}
		trace.DraftBefore, trace.DraftAfter = before, after
		traces = append(traces, trace)
	}
	return traces, rows.Err()
}

// GetLatestByVersionID returns the newest decision recorded on a version. Returns
// domain.ErrNotFound when the version has none.
func (r *HandlerDecisionRepository) GetLatestByVersionID(ctx context.Context, q Querier,
	accountID, versionID uuid.UUID,
) (*domain.HandlerDecisionNote, error) {
	var note domain.HandlerDecisionNote
	var outcome *string
	err := q.QueryRow(ctx,
		`SELECT COALESCE(client_input, ''), COALESCE(ai_interpretation, ''),
		        ai_proposal::jsonb ->> 'outcome', ai_proposal::jsonb ->> 'no_match_reason',
		        seller_decision, created_at
		 FROM handler_decision
		 WHERE account_id = $1 AND quote_version_id = $2
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1`,
		accountID, versionID).Scan(&note.ClientInput, &note.Interpretation, &outcome,
		&note.NoMatchReason, &note.SellerDecision, &note.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	note.Outcome = domain.HandlerOutcomeApplied
	if outcome != nil {
		note.Outcome = domain.HandlerOutcome(*outcome)
	}
	return &note, nil
}

// Create records an AI proposal before a seller accepts, edits, or rejects it. A batch, when named,
// must belong to the version's quote.
func (r *HandlerDecisionRepository) Create(ctx context.Context, q Querier, accountID uuid.UUID,
	in domain.NewHandlerDecision) error {
	before, err := json.Marshal(in.DraftBefore)
	if err != nil {
		return err
	}
	after, err := json.Marshal(in.DraftAfter)
	if err != nil {
		return err
	}
	tag, err := q.Exec(ctx,
		`INSERT INTO handler_decision (
		   account_id, quote_version_id, message_batch_id, client_input, ai_interpretation,
		   ai_proposal, state_at_decision, draft_before, draft_after
		 )
		 SELECT $1, version.id, $3, $4, $5, $6, $7::quote_status, $8::jsonb, $9::jsonb
		 FROM quote_version version
		 WHERE version.account_id = $1 AND version.id = $2
		   AND ($3::uuid IS NULL OR EXISTS (
		     SELECT 1 FROM message_batch batch
		     WHERE batch.account_id = $1 AND batch.id = $3 AND batch.quote_id = version.quote_id))`,
		accountID, in.QuoteVersionID, in.MessageBatchID, in.ClientInput, in.AIInterpretation,
		in.AIProposal, string(in.StateAtDecision), string(before), string(after))
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrNotFound
	}
	return nil
}

// Decide completes still-undecided runs on a version with the seller's decision, each keyed by run.
// A run decided meanwhile keeps its decision. Returns how many runs it completed.
func (r *HandlerDecisionRepository) Decide(ctx context.Context, q Querier, accountID, versionID,
	userID uuid.UUID, decisions map[uuid.UUID]domain.HandlerSellerDecision,
) (int, error) {
	if len(decisions) == 0 {
		return 0, nil
	}
	ids := make([]uuid.UUID, 0, len(decisions))
	values := make([]string, 0, len(decisions))
	for id, decision := range decisions {
		ids = append(ids, id)
		values = append(values, string(decision))
	}
	tag, err := q.Exec(ctx,
		`UPDATE handler_decision decision
		 SET seller_decision = settled.decision::handler_seller_decision, user_id = $3,
		     decided_at = now()
		 FROM unnest($4::uuid[], $5::text[]) AS settled(id, decision)
		 WHERE decision.account_id = $1 AND decision.quote_version_id = $2
		   AND decision.id = settled.id AND decision.seller_decision IS NULL`,
		accountID, versionID, userID, ids, values)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}
