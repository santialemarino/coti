package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const inboundMessageColumns = `id, account_id, branch_id, channel_id, rfq_id,
	external_message_id, external_sender_id, body, raw_payload, received_at, created_at`

const rfqColumns = `id, account_id, branch_id, client_id, channel_id, raw_text, status, work_type,
	received_at, created_at, updated_at, client_label`

// RFQRepository owns persistence for RFQs and their original inbound messages.
type RFQRepository struct{}

// NewRFQRepository builds an RFQRepository.
func NewRFQRepository() *RFQRepository {
	return &RFQRepository{}
}

// Create inserts the original client request before extraction or matching.
func (r *RFQRepository) Create(
	ctx context.Context, q Querier, in domain.NewRFQ,
) (*domain.RFQ, error) {
	rawText := in.RawText
	return scanRFQ(q.QueryRow(ctx,
		`INSERT INTO rfq (account_id, branch_id, client_id, channel_id, raw_text,
		                  received_at, client_label)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING `+rfqColumns,
		in.AccountID, in.BranchID, in.ClientID, in.ChannelID, &rawText, in.ReceivedAt,
		in.ClientLabel))
}

// CreateInboundMessage records one external message, returning false for webhook retries.
func (r *RFQRepository) CreateInboundMessage(
	ctx context.Context, q Querier, in domain.NewInboundChannelMessage,
) (*domain.InboundChannelMessage, bool, error) {
	msg, err := scanInboundMessage(q.QueryRow(ctx,
		`INSERT INTO inbound_channel_message (
		    account_id, branch_id, channel_id, external_message_id, external_sender_id,
		    body, raw_payload, received_at
		 )
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (channel_id, external_message_id) DO NOTHING
		 RETURNING `+inboundMessageColumns,
		in.AccountID, in.BranchID, in.ChannelID, in.ExternalMessageID, in.ExternalSenderID,
		in.Body, []byte(in.RawPayload), in.ReceivedAt))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return msg, true, nil
}

// AttachInboundMessageToRFQ links the persisted original message to the RFQ it created.
func (r *RFQRepository) AttachInboundMessageToRFQ(
	ctx context.Context, q Querier, accountID, messageID, rfqID uuid.UUID,
) error {
	tag, err := q.Exec(ctx,
		`UPDATE inbound_channel_message
		 SET rfq_id = $3
		 WHERE account_id = $1 AND id = $2`,
		accountID, messageID, rfqID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func scanInboundMessage(row pgx.Row) (*domain.InboundChannelMessage, error) {
	var m domain.InboundChannelMessage
	var rawPayload []byte
	if err := row.Scan(&m.ID, &m.AccountID, &m.BranchID, &m.ChannelID, &m.RFQID,
		&m.ExternalMessageID, &m.ExternalSenderID, &m.Body, &rawPayload, &m.ReceivedAt,
		&m.CreatedAt); err != nil {
		return nil, err
	}
	if rawPayload != nil {
		m.RawPayload = json.RawMessage(rawPayload)
	}
	return &m, nil
}

func scanRFQ(row pgx.Row) (*domain.RFQ, error) {
	var r domain.RFQ
	var status string
	if err := row.Scan(&r.ID, &r.AccountID, &r.BranchID, &r.ClientID, &r.ChannelID,
		&r.RawText, &status, &r.WorkType, &r.ReceivedAt, &r.CreatedAt, &r.UpdatedAt,
		&r.ClientLabel); err != nil {
		return nil, err
	}
	r.Status = domain.RFQStatus(status)
	return &r, nil
}
