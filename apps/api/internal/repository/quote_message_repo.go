package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// QuoteMessageRepository stores messages attached to a quote's conversation.
type QuoteMessageRepository struct{}

// NewQuoteMessageRepository builds a QuoteMessageRepository.
func NewQuoteMessageRepository() *QuoteMessageRepository { return &QuoteMessageRepository{} }

const quoteMessageColumns = `id, account_id, quote_id, channel_id, batch_id, client_action_id,
	author_type, author_user_id, body, received_at, other_live_quotes, created_at`

// ListByBatch returns a batch's messages in the order they were received.
func (r *QuoteMessageRepository) ListByBatch(ctx context.Context, q Querier, accountID, quoteID,
	batchID uuid.UUID,
) ([]domain.QuoteMessage, error) {
	return queryQuoteMessages(ctx, q, `SELECT `+quoteMessageColumns+`
		FROM quote_message
		WHERE account_id = $1 AND quote_id = $2 AND batch_id = $3
		ORDER BY received_at, created_at, id`,
		accountID, quoteID, batchID)
}

// ListClientSince returns the customer's messages received at or after since, oldest first.
func (r *QuoteMessageRepository) ListClientSince(ctx context.Context, q Querier, accountID,
	quoteID uuid.UUID, since time.Time,
) ([]domain.QuoteMessage, error) {
	return queryQuoteMessages(ctx, q, `SELECT `+quoteMessageColumns+`
		FROM quote_message
		WHERE account_id = $1 AND quote_id = $2 AND author_type = 'CLIENT' AND received_at >= $3
		ORDER BY received_at, created_at, id`,
		accountID, quoteID, since)
}

// ListContext returns the limit newest messages of any author received up to before outside a
// batch, oldest first.
func (r *QuoteMessageRepository) ListContext(ctx context.Context, q Querier, accountID, quoteID,
	batchID uuid.UUID, before time.Time, limit int,
) ([]domain.QuoteMessage, error) {
	return queryQuoteMessages(ctx, q, `SELECT `+quoteMessageColumns+` FROM (
		  SELECT `+quoteMessageColumns+` FROM quote_message
		  WHERE account_id = $1 AND quote_id = $2 AND batch_id IS DISTINCT FROM $3
		    AND received_at <= $4
		  ORDER BY received_at DESC, created_at DESC, id DESC
		  LIMIT $5) recent
		ORDER BY received_at, created_at, id`,
		accountID, quoteID, batchID, before, limit)
}

// Create appends a message to a branch-scoped quote's conversation. A channel, when named, must
// belong to the quote's branch. Returns domain.ErrNotFound when either does not.
func (r *QuoteMessageRepository) Create(ctx context.Context, q Querier, accountID,
	branchID uuid.UUID, in domain.NewQuoteMessage,
) (*domain.QuoteMessage, error) {
	if in.ID == uuid.Nil {
		return nil, errors.New("quote message: the caller chooses the ID")
	}
	message, err := scanQuoteMessage(q.QueryRow(ctx, `INSERT INTO quote_message
		(id, account_id, quote_id, channel_id, author_type, author_user_id, body, received_at,
		 other_live_quotes)
		SELECT $4, quote.account_id, quote.id, $5, $6, $7, $8, $9, $10
		FROM quote
		WHERE quote.account_id = $1 AND quote.branch_id = $2 AND quote.id = $3
		  AND ($5::uuid IS NULL OR EXISTS (
		    SELECT 1 FROM channel
		    WHERE channel.account_id = quote.account_id AND channel.branch_id = quote.branch_id
		      AND channel.id = $5))
		RETURNING `+quoteMessageColumns,
		accountID, branchID, in.QuoteID, in.ID, in.ChannelID, in.AuthorType, in.AuthorUserID,
		in.Body, in.ReceivedAt, in.OtherLiveQuotes))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return message, err
}

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

func queryQuoteMessages(ctx context.Context, q Querier, sql string, args ...any,
) ([]domain.QuoteMessage, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := make([]domain.QuoteMessage, 0)
	for rows.Next() {
		message, err := scanQuoteMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, *message)
	}
	return messages, rows.Err()
}

func scanQuoteMessage(row pgx.Row) (*domain.QuoteMessage, error) {
	var message domain.QuoteMessage
	err := row.Scan(&message.ID, &message.AccountID, &message.QuoteID, &message.ChannelID,
		&message.BatchID, &message.ClientActionID, &message.AuthorType, &message.AuthorUserID,
		&message.Body, &message.ReceivedAt, &message.OtherLiveQuotes, &message.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &message, nil
}
