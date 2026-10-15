package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// HandlerDecisionRepository owns persistence for proposed customer-message changes.
type HandlerDecisionRepository struct{}

// NewHandlerDecisionRepository builds a HandlerDecisionRepository.
func NewHandlerDecisionRepository() *HandlerDecisionRepository { return &HandlerDecisionRepository{} }

// GetLatestByVersionID returns the newest decision recorded on a version. Returns
// domain.ErrNotFound when the version has none.
func (r *HandlerDecisionRepository) GetLatestByVersionID(ctx context.Context, q Querier,
	accountID, versionID uuid.UUID,
) (*domain.HandlerDecisionNote, error) {
	var note domain.HandlerDecisionNote
	var outcome *string
	err := q.QueryRow(ctx,
		`SELECT COALESCE(client_input, ''), COALESCE(ai_interpretation, ''),
		        ai_proposal::jsonb ->> 'outcome', ai_proposal::jsonb ->> 'no_match_reason', created_at
		 FROM handler_decision
		 WHERE account_id = $1 AND quote_version_id = $2
		 ORDER BY created_at DESC, id DESC
		 LIMIT 1`,
		accountID, versionID).Scan(&note.ClientInput, &note.Interpretation, &outcome,
		&note.NoMatchReason, &note.CreatedAt)
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
	tag, err := q.Exec(ctx,
		`INSERT INTO handler_decision (
		   account_id, quote_version_id, message_batch_id, client_input, ai_interpretation,
		   ai_proposal, state_at_decision
		 )
		 SELECT $1, version.id, $3, $4, $5, $6, $7::quote_status
		 FROM quote_version version
		 WHERE version.account_id = $1 AND version.id = $2
		   AND ($3::uuid IS NULL OR EXISTS (
		     SELECT 1 FROM message_batch batch
		     WHERE batch.account_id = $1 AND batch.id = $3 AND batch.quote_id = version.quote_id))`,
		accountID, in.QuoteVersionID, in.MessageBatchID, in.ClientInput, in.AIInterpretation,
		in.AIProposal, string(in.StateAtDecision))
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrNotFound
	}
	return nil
}
