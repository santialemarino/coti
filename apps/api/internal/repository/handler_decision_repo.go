package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// HandlerDecisionRepository owns persistence for proposed customer-message changes.
type HandlerDecisionRepository struct{}

// NewHandlerDecisionRepository builds a HandlerDecisionRepository.
func NewHandlerDecisionRepository() *HandlerDecisionRepository { return &HandlerDecisionRepository{} }

// Create records an AI proposal before a seller accepts, edits, or rejects it.
func (r *HandlerDecisionRepository) Create(ctx context.Context, q Querier, accountID uuid.UUID,
	in domain.NewHandlerDecision) error {
	tag, err := q.Exec(ctx,
		`INSERT INTO handler_decision (
		   account_id, quote_version_id, client_input, ai_interpretation, ai_proposal,
		   state_at_decision
		 )
		 SELECT $1, version.id, $3, $4, $5, $6::quote_status
		 FROM quote_version version
		 WHERE version.account_id = $1 AND version.id = $2`,
		accountID, in.QuoteVersionID, in.ClientInput, in.AIInterpretation, in.AIProposal,
		string(in.StateAtDecision))
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrNotFound
	}
	return nil
}
