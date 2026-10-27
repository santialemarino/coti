package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// QueueNameRequest schedules one deterministic prompt for an unknown contact.
func (r *ClientContactRepository) QueueNameRequest(ctx context.Context, q Querier, route domain.InboundChannelRoute, kind domain.ChannelType, value string) error {
	_, err := q.Exec(ctx, `INSERT INTO client_contact_outbox(account_id,branch_id,channel_id,kind,value,purpose) VALUES($1,$2,$3,$4,$5,'REQUEST_NAME') ON CONFLICT(account_id,kind,value,purpose) DO UPDATE SET status='PENDING',channel_id=excluded.channel_id,branch_id=excluded.branch_id,created_at=now() WHERE client_contact_outbox.status='COMPLETED'`, route.AccountID, route.BranchID, route.ChannelID, kind, value)
	return err
}

// AttachContactObservations binds retained messages to the profile identified by their channel sender.
func (r *ClientContactRepository) AttachContactObservations(ctx context.Context, q Querier, accountID, clientID uuid.UUID, kind domain.ChannelType, value string) error {
	if _, err := q.Exec(ctx, `UPDATE client_contact_observation SET source_client_id=$4 WHERE account_id=$1 AND kind=$2 AND value=$3 AND source_client_id IS NULL`, accountID, kind, value, clientID); err != nil {
		return err
	}
	_, err := q.Exec(ctx, `UPDATE client_contact_outbox SET status='COMPLETED' WHERE account_id=$1 AND kind=$2 AND value=$3`, accountID, kind, value)
	return err
}

// AssociateObservedRequests attaches retained sender history without changing an existing selection.
func (r *ClientContactRepository) AssociateObservedRequests(ctx context.Context, q Querier, accountID uuid.UUID, kind domain.ChannelType, value string) error {
	if _, err := q.Exec(ctx, `UPDATE rfq r SET client_id=COALESCE(cl.merged_into,cl.id) FROM inbound_channel_message m JOIN client_contact_observation o ON o.account_id=m.account_id AND o.message_id=m.id JOIN client cl ON cl.account_id=o.account_id AND cl.id=o.source_client_id WHERE r.account_id=$1 AND m.account_id=$1 AND o.account_id=$1 AND o.kind=$2 AND o.value=$3 AND o.source_client_id IS NOT NULL AND r.id=m.rfq_id AND r.branch_id=m.branch_id AND r.client_id IS NULL`, accountID, kind, value); err != nil {
		return err
	}
	_, err := q.Exec(ctx, `UPDATE quote qt SET client_id=r.client_id FROM rfq r WHERE qt.account_id=$1 AND r.account_id=$1 AND qt.rfq_id=r.id AND qt.branch_id=r.branch_id AND qt.client_id IS NULL AND r.client_id IS NOT NULL AND EXISTS(SELECT 1 FROM client_contact_observation o JOIN inbound_channel_message m ON m.account_id=o.account_id AND m.id=o.message_id WHERE o.account_id=$1 AND o.kind=$2 AND o.value=$3 AND m.rfq_id=r.id)`, accountID, kind, value)
	return err
}

// ClaimNameRequest reserves one prompt for delivery outside the ingestion transaction.
func (r *ClientContactRepository) ClaimNameRequest(ctx context.Context, q Querier, route domain.InboundChannelRoute, kind domain.ChannelType, value string) (*uuid.UUID, error) {
	var id uuid.UUID
	err := q.QueryRow(ctx, `UPDATE client_contact_outbox SET status='SENDING' WHERE account_id=$1 AND branch_id=$2 AND channel_id=$3 AND kind=$4 AND value=$5 AND status IN ('PENDING','FAILED') RETURNING id`, route.AccountID, route.BranchID, route.ChannelID, kind, value).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &id, err
}

// FinishNameRequest retains the acknowledged delivery or leaves a failed prompt retryable.
func (r *ClientContactRepository) FinishNameRequest(ctx context.Context, q Querier, route domain.InboundChannelRoute, id uuid.UUID, reference string, success bool) error {
	status := "FAILED"
	if success {
		status = "SENT"
	}
	_, err := q.Exec(ctx, `UPDATE client_contact_outbox SET status=$4,provider_reference=$5 WHERE account_id=$1 AND branch_id=$2 AND id=$3 AND status='SENDING'`, route.AccountID, route.BranchID, id, status, reference)
	return err
}

// PendingNameRFQ locates a retained request awaiting the same sender's declared name.
func (r *ClientContactRepository) PendingNameRFQ(ctx context.Context, q Querier, route domain.InboundChannelRoute, kind domain.ChannelType, value string) (*uuid.UUID, error) {
	var id uuid.UUID
	err := q.QueryRow(ctx, `SELECT r.id FROM client_contact_observation o JOIN inbound_channel_message m ON m.account_id=o.account_id AND m.id=o.message_id JOIN rfq r ON r.account_id=m.account_id AND r.branch_id=m.branch_id AND r.id=m.rfq_id WHERE o.account_id=$1 AND m.account_id=$1 AND r.account_id=$1 AND m.branch_id=$2 AND m.channel_id=$3 AND o.kind=$4 AND o.value=$5 AND o.source_client_id IS NULL ORDER BY m.created_at,m.id LIMIT 1`, route.AccountID, route.BranchID, route.ChannelID, kind, value).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &id, err
}
