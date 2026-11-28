package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const clientActionColumns = `action.id, action.account_id, action.version_id,
	action.quote_send_id, action.quote_item_id, action.type, action.comment, action.created_at,
	action.message_batch_id, action.revoked_at`

// ClientActionRepository owns customer responses to published quotes and their seller-side reads.
type ClientActionRepository struct{}

// NewClientActionRepository builds a ClientActionRepository.
func NewClientActionRepository() *ClientActionRepository { return &ClientActionRepository{} }

// Create inserts one customer response bound to the exact version it answers. The version id
// arrives from a public token rather than from a session, so the insert proves it belongs to the
// account before it writes and returns domain.ErrNotFound when it does not; a batch, when named,
// must belong to the account too. A zero id lets the database generate the primary key.
func (r *ClientActionRepository) Create(ctx context.Context, q Querier, accountID uuid.UUID,
	in domain.NewClientAction) (*domain.ClientAction, error) {
	return scanClientAction(q.QueryRow(ctx, `INSERT INTO client_action AS action
		(id, account_id, version_id, quote_send_id, quote_item_id, type, comment, message_batch_id)
		SELECT COALESCE(NULLIF($1::uuid, '00000000-0000-0000-0000-000000000000'),
		       gen_random_uuid()), $2, $3, $4, NULL, $5, $6, $7
		WHERE EXISTS (SELECT 1 FROM quote_version WHERE id = $3 AND account_id = $2)
		  AND ($7::uuid IS NULL OR EXISTS (SELECT 1 FROM message_batch
		    WHERE id = $7 AND account_id = $2))
		RETURNING `+clientActionColumns,
		in.ID, accountID, in.VersionID, in.QuoteSendID, in.Type, in.Comment, in.MessageBatchID))
}

// GetBySend reads the decision shared by deliveries of the same version within one send cycle; one
// the seller revoked no longer answers the send.
func (r *ClientActionRepository) GetBySend(ctx context.Context, q Querier, accountID uuid.UUID,
	sendID uuid.UUID) (*domain.ClientAction, error) {
	return scanClientAction(q.QueryRow(ctx, `SELECT `+clientActionColumns+`
		FROM client_action action
		JOIN quote_send target ON target.account_id = action.account_id AND target.version_id = action.version_id
		JOIN quote_send origin ON origin.account_id = action.account_id AND origin.id = action.quote_send_id
		  AND origin.version_id = action.version_id
		JOIN quote_version version ON version.account_id = action.account_id AND version.id = action.version_id
		WHERE action.account_id = $1 AND target.id = $2
		  AND action.revoked_at IS NULL AND action.type IN ('ACCEPT', 'REJECT', 'REQUEST_CHANGE')
		  AND NOT EXISTS (
		    SELECT 1 FROM quote_status_change boundary
		    WHERE boundary.account_id = action.account_id AND boundary.quote_id = version.quote_id
		      AND boundary.new_status = 'QUOTED'
		      AND boundary.changed_at > LEAST(COALESCE(origin.sent_at, origin.created_at),
		                                     COALESCE(target.sent_at, target.created_at))
		      AND boundary.changed_at <= GREATEST(COALESCE(origin.sent_at, origin.created_at),
		                                         COALESCE(target.sent_at, target.created_at)))
		ORDER BY action.created_at, action.id LIMIT 1`,
		accountID, sendID))
}

// ListByQuote loads every standing customer response on a branch-scoped quote across its versions,
// oldest first. The version number comes from the version the action pinned, not from the current
// one.
func (r *ClientActionRepository) ListByQuote(ctx context.Context, q Querier, accountID, branchID,
	quoteID uuid.UUID) ([]domain.ClientAction, error) {
	rows, err := q.Query(ctx, `SELECT `+clientActionColumns+`, version.version_number
		FROM client_action action
		JOIN quote_version version ON version.account_id = action.account_id
		  AND version.id = action.version_id
		JOIN quote ON quote.account_id = action.account_id AND quote.id = version.quote_id
		WHERE quote.account_id = $1 AND quote.branch_id = $2 AND quote.id = $3
		  AND action.revoked_at IS NULL
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
			&action.MessageBatchID, &action.RevokedAt, &action.VersionNumber)
		if err != nil {
			return nil, err
		}
		actions = append(actions, action)
	}
	return actions, rows.Err()
}

// GetStandingAcceptance reads the acceptance that stands for a version, revoked ones aside.
func (r *ClientActionRepository) GetStandingAcceptance(ctx context.Context, q Querier,
	accountID, versionID uuid.UUID) (*domain.ClientAction, error) {
	return scanClientAction(q.QueryRow(ctx, `SELECT `+clientActionColumns+`
		FROM client_action action
		WHERE action.account_id = $1 AND action.version_id = $2 AND action.type = 'ACCEPT'
		  AND action.revoked_at IS NULL
		ORDER BY action.created_at DESC, action.id DESC LIMIT 1`, accountID, versionID))
}

// Revoke marks an action as undone by the seller, so it no longer answers its send.
func (r *ClientActionRepository) Revoke(ctx context.Context, q Querier, accountID,
	actionID uuid.UUID) error {
	tag, err := q.Exec(ctx, `UPDATE client_action SET revoked_at = now()
		WHERE account_id = $1 AND id = $2 AND revoked_at IS NULL`, accountID, actionID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrConflict
	}
	return nil
}

func scanClientAction(row pgx.Row) (*domain.ClientAction, error) {
	var action domain.ClientAction
	err := row.Scan(&action.ID, &action.AccountID, &action.VersionID, &action.QuoteSendID,
		&action.QuoteItemID, &action.Type, &action.Comment, &action.CreatedAt,
		&action.MessageBatchID, &action.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &action, nil
}
