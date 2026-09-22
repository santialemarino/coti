package repository

import (
	"context"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// QuoteMessageRepository stores messages attached to a quote's conversation.
type QuoteMessageRepository struct{}

// NewQuoteMessageRepository builds a QuoteMessageRepository.
func NewQuoteMessageRepository() *QuoteMessageRepository { return &QuoteMessageRepository{} }

// CreateClientRequest links a customer's change request to its public action and quote.
func (r *QuoteMessageRepository) CreateClientRequest(ctx context.Context, q Querier,
	accountID, branchID, quoteID, channelID, actionID uuid.UUID, body string) error {
	tag, err := q.Exec(ctx, `INSERT INTO quote_message
		(account_id, quote_id, channel_id, client_action_id, author_type, body)
		SELECT $1, quote.id, channel.id, action.id, 'CLIENT', $6
		FROM quote
		JOIN channel ON channel.account_id = quote.account_id
		  AND channel.branch_id = quote.branch_id AND channel.id = $4
		JOIN client_action action ON action.account_id = quote.account_id AND action.id = $5
		JOIN quote_send send ON send.account_id = quote.account_id
		  AND send.id = action.quote_send_id AND send.channel_id = channel.id
		JOIN quote_version version ON version.account_id = quote.account_id
		  AND version.id = action.version_id AND version.quote_id = quote.id
		  AND send.version_id = version.id
		WHERE quote.account_id = $1 AND quote.branch_id = $2 AND quote.id = $3`,
		accountID, branchID, quoteID, channelID, actionID, body)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrNotFound
	}
	return nil
}
