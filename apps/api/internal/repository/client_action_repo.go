package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const clientActionColumns = `action.id, action.account_id, action.version_id,
	action.quote_send_id, action.quote_item_id, action.type, action.comment, action.created_at`

// ClientActionRepository owns customer responses to published quotes and their seller-side reads.
type ClientActionRepository struct{}

// NewClientActionRepository builds a ClientActionRepository.
func NewClientActionRepository() *ClientActionRepository { return &ClientActionRepository{} }

// Create inserts one customer response bound to the exact version it answers. The version id
// arrives from a public token rather than from a session, so the insert proves it belongs to the
// account before it writes and returns domain.ErrNotFound when it does not. A zero id lets the
// database generate the primary key.
func (r *ClientActionRepository) Create(ctx context.Context, q Querier, accountID uuid.UUID,
	in domain.NewClientAction) (*domain.ClientAction, error) {
	return scanClientAction(q.QueryRow(ctx, `INSERT INTO client_action
		(id, account_id, version_id, quote_send_id, quote_item_id, type, comment)
		SELECT COALESCE(NULLIF($1::uuid, '00000000-0000-0000-0000-000000000000'),
		       gen_random_uuid()), $2, $3, $4, NULL, $5, $6
		WHERE EXISTS (SELECT 1 FROM quote_version WHERE id = $3 AND account_id = $2)
		RETURNING id, account_id, version_id, quote_send_id, quote_item_id, type, comment, created_at`,
		in.ID, accountID, in.VersionID, in.QuoteSendID, in.Type, in.Comment))
}

// GetBySend loads the one customer response recorded for a public delivery, if any. Rows whose
// quote_send_id is NULL never match, which is the point: only a link-carried answer is a response.
func (r *ClientActionRepository) GetBySend(ctx context.Context, q Querier, accountID uuid.UUID,
	sendID uuid.UUID) (*domain.ClientAction, error) {
	return scanClientAction(q.QueryRow(ctx, `SELECT `+clientActionColumns+`
		FROM client_action action
		WHERE action.account_id = $1 AND action.quote_send_id = $2`,
		accountID, sendID))
}

// ListByQuote loads every customer response on a branch-scoped quote across its versions, oldest
// first. The version number comes from the version the action pinned, not from the current one.
func (r *ClientActionRepository) ListByQuote(ctx context.Context, q Querier, accountID, branchID,
	quoteID uuid.UUID) ([]domain.ClientAction, error) {
	rows, err := q.Query(ctx, `SELECT `+clientActionColumns+`, version.version_number
		FROM client_action action
		JOIN quote_version version ON version.account_id = action.account_id
		  AND version.id = action.version_id
		JOIN quote ON quote.account_id = action.account_id AND quote.id = version.quote_id
		WHERE quote.account_id = $1 AND quote.branch_id = $2 AND quote.id = $3
		ORDER BY action.created_at, action.id`, accountID, branchID, quoteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	actions := make([]domain.ClientAction, 0)
	for rows.Next() {
		var action domain.ClientAction
		// The version number trails the row's own columns; only this list join answers it.
		err := rows.Scan(&action.ID, &action.AccountID, &action.VersionID, &action.QuoteSendID,
			&action.QuoteItemID, &action.Type, &action.Comment, &action.CreatedAt,
			&action.VersionNumber)
		if err != nil {
			return nil, err
		}
		actions = append(actions, action)
	}
	return actions, rows.Err()
}

func scanClientAction(row pgx.Row) (*domain.ClientAction, error) {
	var action domain.ClientAction
	err := row.Scan(&action.ID, &action.AccountID, &action.VersionID, &action.QuoteSendID,
		&action.QuoteItemID, &action.Type, &action.Comment, &action.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &action, nil
}
