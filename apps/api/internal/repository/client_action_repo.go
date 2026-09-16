package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const clientActionColumns = `id, account_id, version_id, quote_send_id, quote_item_id, type,
	comment, created_at`

// ClientActionRepository owns persistence for the answers a customer gives to a quote.
type ClientActionRepository struct{}

// NewClientActionRepository builds a ClientActionRepository.
func NewClientActionRepository() *ClientActionRepository {
	return &ClientActionRepository{}
}

// Create records one customer answer against the version it was given on. The version id arrives
// from a public token rather than from a session, so the insert proves it belongs to the account
// before it writes and returns domain.ErrNotFound when it does not.
func (r *ClientActionRepository) Create(
	ctx context.Context, q Querier, accountID uuid.UUID, in domain.NewClientAction,
) (*domain.ClientAction, error) {
	return scanClientAction(q.QueryRow(ctx,
		`INSERT INTO client_action (account_id, version_id, quote_send_id, type, comment)
		 SELECT $1, $2, $3, $4, $5
		 WHERE EXISTS (SELECT 1 FROM quote_version WHERE id = $2 AND account_id = $1)
		 RETURNING `+clientActionColumns,
		accountID, in.VersionID, in.QuoteSendID, in.Type, in.Comment))
}

func scanClientAction(row pgx.Row) (*domain.ClientAction, error) {
	var a domain.ClientAction
	err := row.Scan(&a.ID, &a.AccountID, &a.VersionID, &a.QuoteSendID, &a.QuoteItemID, &a.Type,
		&a.Comment, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}
