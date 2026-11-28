package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const inboundChannelMessageColumns = `id, account_id, branch_id, channel_id, rfq_id,
	quote_message_id, kind, external_message_id, sender_id, sender_label, body, payload,
	provider_received_at, dismissed_at, created_at`

// InboundChannelMessageRepository owns provider-event persistence for inbound channel messages.
type InboundChannelMessageRepository struct{}

// NewInboundChannelMessageRepository builds an InboundChannelMessageRepository.
func NewInboundChannelMessageRepository() *InboundChannelMessageRepository {
	return &InboundChannelMessageRepository{}
}

// FindByExternalID finds the event already reserved for one branch and provider message.
func (r *InboundChannelMessageRepository) FindByExternalID(ctx context.Context, q Querier,
	accountID, branchID, channelID uuid.UUID, externalID string,
) (*domain.InboundChannelMessage, error) {
	message, err := scanInboundChannelMessage(q.QueryRow(ctx,
		`SELECT `+inboundChannelMessageColumns+`
		 FROM inbound_channel_message
		 WHERE account_id = $1 AND branch_id = $2 AND channel_id = $3 AND external_message_id = $4`,
		accountID, branchID, channelID, externalID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return message, err
}

// Reserve creates an inbound event, or returns the one an earlier delivery of the same provider
// message reserved; a returned ID other than in.ID means this delivery is a retry.
func (r *InboundChannelMessageRepository) Reserve(ctx context.Context, q Querier, accountID,
	branchID, channelID uuid.UUID, in domain.NewInboundChannelMessage,
) (*domain.InboundChannelMessage, error) {
	return scanInboundChannelMessage(q.QueryRow(ctx,
		`INSERT INTO inbound_channel_message
			(id, account_id, branch_id, channel_id, rfq_id, quote_message_id, kind,
			 external_message_id, sender_id, sender_label, body, payload, provider_received_at,
			 dismissed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb, $13,
			 CASE WHEN $14::boolean THEN now() END)
		 ON CONFLICT (channel_id, external_message_id) DO UPDATE
		 SET external_message_id = EXCLUDED.external_message_id
		 RETURNING `+inboundChannelMessageColumns,
		in.ID, accountID, branchID, channelID, in.RFQID, in.QuoteMessageID, in.Kind,
		in.ExternalMessageID, in.SenderID, in.SenderLabel, in.Body, in.Payload,
		in.ProviderReceivedAt, in.Dismissed))
}

// ListOpenNonOrders lists a branch's messages that opened no order and no seller put away,
// newest first.
func (r *InboundChannelMessageRepository) ListOpenNonOrders(ctx context.Context, q Querier,
	accountID, branchID uuid.UUID, limit int,
) ([]domain.InboundChannelMessage, error) {
	rows, err := q.Query(ctx,
		`SELECT `+inboundChannelMessageColumns+`
		 FROM inbound_channel_message
		 WHERE account_id = $1 AND branch_id = $2 AND kind IS NOT NULL AND dismissed_at IS NULL
		 ORDER BY provider_received_at DESC, id
		 LIMIT $3`,
		accountID, branchID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	messages := []domain.InboundChannelMessage{}
	for rows.Next() {
		message, err := scanInboundChannelMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, *message)
	}
	return messages, rows.Err()
}

// DetachNonOrder turns the message reserved for rfqID into a non-order of kind, so the RFQ it was
// waiting on can be removed. It returns nil when no message names that RFQ.
func (r *InboundChannelMessageRepository) DetachNonOrder(ctx context.Context, q Querier,
	accountID, branchID, rfqID uuid.UUID, kind domain.InboundMessageKind,
) (*domain.InboundChannelMessage, error) {
	message, err := scanInboundChannelMessage(q.QueryRow(ctx,
		`UPDATE inbound_channel_message SET rfq_id = NULL, kind = $4
		 WHERE account_id = $1 AND branch_id = $2 AND rfq_id = $3
		 RETURNING `+inboundChannelMessageColumns,
		accountID, branchID, rfqID, kind))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return message, err
}

// ClaimNonOrder points one non-order message at rfqID, the order a seller opens from it. It
// returns ErrNotFound when the message is not a non-order of the branch.
func (r *InboundChannelMessageRepository) ClaimNonOrder(ctx context.Context, q Querier,
	accountID, branchID, id, rfqID uuid.UUID,
) (*domain.InboundChannelMessage, error) {
	message, err := scanInboundChannelMessage(q.QueryRow(ctx,
		`UPDATE inbound_channel_message SET rfq_id = $4, kind = NULL, dismissed_at = NULL
		 WHERE account_id = $1 AND branch_id = $2 AND id = $3 AND kind IS NOT NULL
		 RETURNING `+inboundChannelMessageColumns,
		accountID, branchID, id, rfqID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return message, err
}

// Dismiss puts away one open non-order message.
func (r *InboundChannelMessageRepository) Dismiss(ctx context.Context, q Querier,
	accountID, branchID, id uuid.UUID,
) error {
	tag, err := q.Exec(ctx,
		`UPDATE inbound_channel_message SET dismissed_at = now()
		 WHERE account_id = $1 AND branch_id = $2 AND id = $3 AND kind IS NOT NULL
		   AND dismissed_at IS NULL`,
		accountID, branchID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// FindSplitOrder returns the open order nearest in time that the same sender opened on the same
// channel within window of rfqID, or nil when there is none.
func (r *InboundChannelMessageRepository) FindSplitOrder(ctx context.Context, q Querier,
	accountID, branchID, rfqID uuid.UUID, window time.Duration,
) (*domain.SplitOrder, error) {
	var split domain.SplitOrder
	err := q.QueryRow(ctx,
		`SELECT other.id, other_quote.number, other.created_at
		 FROM inbound_channel_message message
		 JOIN rfq this ON this.account_id = message.account_id AND this.id = message.rfq_id
		 JOIN inbound_channel_message sibling ON sibling.account_id = message.account_id
		  AND sibling.branch_id = message.branch_id AND sibling.channel_id = message.channel_id
		  AND sibling.sender_id = message.sender_id AND sibling.rfq_id <> message.rfq_id
		 JOIN rfq other ON other.account_id = sibling.account_id AND other.branch_id = sibling.branch_id
		  AND other.id = sibling.rfq_id
		 LEFT JOIN quote other_quote ON other_quote.account_id = other.account_id
		  AND other_quote.rfq_id = other.id
		 WHERE message.account_id = $1 AND message.branch_id = $2 AND message.rfq_id = $3
		   AND message.sender_id <> '' AND other.archived_at IS NULL AND other_quote.archived_at IS NULL
		   AND abs(extract(epoch FROM other.created_at - this.created_at)) <= $4::double precision
		 ORDER BY abs(extract(epoch FROM other.created_at - this.created_at)), other.id
		 LIMIT 1`,
		accountID, branchID, rfqID, window.Seconds(),
	).Scan(&split.RFQID, &split.QuoteNumber, &split.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &split, nil
}

// FindSenderIDByRFQID returns the provider sender identifier retained for an inbound RFQ.
func (r *InboundChannelMessageRepository) FindSenderIDByRFQID(
	ctx context.Context, q Querier, accountID, branchID, rfqID uuid.UUID,
) (*string, error) {
	var senderID string
	err := q.QueryRow(ctx,
		`SELECT sender_id
		 FROM inbound_channel_message
		 WHERE account_id = $1 AND branch_id = $2 AND rfq_id = $3
		 ORDER BY provider_received_at DESC, created_at DESC
		 LIMIT 1`,
		accountID, branchID, rfqID,
	).Scan(&senderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &senderID, nil
}

func scanInboundChannelMessage(row channelScanner) (*domain.InboundChannelMessage, error) {
	var message domain.InboundChannelMessage
	err := row.Scan(&message.ID, &message.AccountID, &message.BranchID, &message.ChannelID,
		&message.RFQID, &message.QuoteMessageID, &message.Kind, &message.ExternalMessageID,
		&message.SenderID, &message.SenderLabel, &message.Body, &message.Payload,
		&message.ProviderReceivedAt, &message.DismissedAt, &message.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &message, nil
}
