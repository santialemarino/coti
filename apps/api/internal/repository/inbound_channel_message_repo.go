package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const inboundChannelMessageColumns = `id, account_id, branch_id, channel_id, rfq_id,
	external_message_id, sender_id, sender_label, body, payload, provider_received_at, created_at`

// InboundChannelMessageRepository owns provider-event persistence for inbound channel messages.
type InboundChannelMessageRepository struct{}

// NewInboundChannelMessageRepository builds an InboundChannelMessageRepository.
func NewInboundChannelMessageRepository() *InboundChannelMessageRepository {
	return &InboundChannelMessageRepository{}
}

// Reserve creates an inbound event or returns the RFQ reservation from an earlier provider retry.
func (r *InboundChannelMessageRepository) Reserve(ctx context.Context, q Querier, accountID,
	branchID, channelID uuid.UUID, in domain.NewInboundChannelMessage,
) (*domain.InboundChannelMessage, error) {
	return scanInboundChannelMessage(q.QueryRow(ctx,
		`INSERT INTO inbound_channel_message
			(id, account_id, branch_id, channel_id, rfq_id, external_message_id, sender_id,
			 sender_label, body, payload, provider_received_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb, $11)
		 ON CONFLICT (channel_id, external_message_id) DO UPDATE
		 SET external_message_id = EXCLUDED.external_message_id
		 RETURNING `+inboundChannelMessageColumns,
		in.ID, accountID, branchID, channelID, in.RFQID, in.ExternalMessageID, in.SenderID,
		in.SenderLabel, in.Body, in.Payload, in.ProviderReceivedAt))
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
		&message.RFQID, &message.ExternalMessageID, &message.SenderID, &message.SenderLabel,
		&message.Body, &message.Payload, &message.ProviderReceivedAt, &message.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &message, nil
}
