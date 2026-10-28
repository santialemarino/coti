package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// GetContactMailbox reads the supplier identity a branch mailbox answers a contact with.
func (r *ClientContactRepository) GetContactMailbox(ctx context.Context, q Querier, accountID, branchID, channelID uuid.UUID) (*domain.ContactMailbox, error) {
	m := &domain.ContactMailbox{}
	err := q.QueryRow(ctx, `SELECT c.identifier,a.name,b.name FROM channel c JOIN branch b ON b.account_id=c.account_id AND b.id=c.branch_id AND b.is_active JOIN account a ON a.id=c.account_id AND a.is_active WHERE c.account_id=$1 AND c.branch_id=$2 AND c.id=$3 AND c.type='EMAIL' AND c.is_active AND c.identifier IS NOT NULL`, accountID, branchID, channelID).Scan(&m.Address, &m.SupplierName, &m.BranchName)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotConfigured
	}
	return m, err
}

// QueueNameRequest schedules one deterministic prompt for an unknown contact.
func (r *ClientContactRepository) QueueNameRequest(ctx context.Context, q Querier, route domain.InboundChannelRoute, kind domain.ChannelType, value string) error {
	_, err := q.Exec(ctx, `INSERT INTO client_contact_outbox(account_id,branch_id,channel_id,kind,value,purpose) VALUES($1,$2,$3,$4,$5,'REQUEST_NAME') ON CONFLICT(account_id,kind,value,purpose) DO UPDATE SET status='PENDING',channel_id=excluded.channel_id,branch_id=excluded.branch_id,created_at=now(),status_changed_at=now() WHERE client_contact_outbox.status='COMPLETED'`, route.AccountID, route.BranchID, route.ChannelID, kind, value)
	return err
}

// AttachContactObservations binds retained messages to the profile identified by their channel sender.
func (r *ClientContactRepository) AttachContactObservations(ctx context.Context, q Querier, accountID, clientID uuid.UUID, kind domain.ChannelType, value string) error {
	if _, err := q.Exec(ctx, `UPDATE client_contact_observation SET source_client_id=$4 WHERE account_id=$1 AND kind=$2 AND value=$3 AND source_client_id IS NULL`, accountID, kind, value, clientID); err != nil {
		return err
	}
	return nil
}

// AssociateObservedRequests attaches retained sender history without changing an existing selection.
func (r *ClientContactRepository) AssociateObservedRequests(ctx context.Context, q Querier, accountID uuid.UUID, kind domain.ChannelType, value string) error {
	if _, err := q.Exec(ctx, `UPDATE rfq r SET client_id=COALESCE(cl.merged_into,cl.id) FROM inbound_channel_message m JOIN client_contact_observation o ON o.account_id=m.account_id AND o.message_id=m.id JOIN client cl ON cl.account_id=o.account_id AND cl.id=o.source_client_id WHERE r.account_id=$1 AND m.account_id=$1 AND o.account_id=$1 AND o.kind=$2 AND o.value=$3 AND o.source_client_id IS NOT NULL AND r.id=m.rfq_id AND r.branch_id=m.branch_id AND r.client_id IS NULL`, accountID, kind, value); err != nil {
		return err
	}
	_, err := q.Exec(ctx, `UPDATE quote qt SET client_id=r.client_id FROM rfq r WHERE qt.account_id=$1 AND r.account_id=$1 AND qt.rfq_id=r.id AND qt.branch_id=r.branch_id AND qt.client_id IS NULL AND r.client_id IS NOT NULL AND EXISTS(SELECT 1 FROM client_contact_observation o JOIN inbound_channel_message m ON m.account_id=o.account_id AND m.id=o.message_id WHERE o.account_id=$1 AND o.kind=$2 AND o.value=$3 AND m.rfq_id=r.id)`, accountID, kind, value)
	return err
}

// ClaimNameRequest reserves one prompt for delivery outside the ingestion transaction. A claim
// older than lease is taken again, so a process that died mid-delivery does not strand it.
func (r *ClientContactRepository) ClaimNameRequest(ctx context.Context, q Querier, route domain.InboundChannelRoute, kind domain.ChannelType, value string, lease time.Duration) (*uuid.UUID, error) {
	var id uuid.UUID
	err := q.QueryRow(ctx, `UPDATE client_contact_outbox SET status='SENDING',status_changed_at=now() WHERE account_id=$1 AND branch_id=$2 AND channel_id=$3 AND kind=$4 AND value=$5 AND (status IN ('PENDING','FAILED') OR ($6::double precision>0 AND status='SENDING' AND status_changed_at<now()-make_interval(secs=>$6::double precision))) RETURNING id`, route.AccountID, route.BranchID, route.ChannelID, kind, value, lease.Seconds()).Scan(&id)
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
	_, err := q.Exec(ctx, `UPDATE client_contact_outbox SET status=$4,provider_reference=$5,status_changed_at=now() WHERE account_id=$1 AND branch_id=$2 AND id=$3 AND status='SENDING'`, route.AccountID, route.BranchID, id, status, reference)
	return err
}

// MentionsCatalog reports whether any word of text names an active product or synonym of the account.
func (r *ClientContactRepository) MentionsCatalog(ctx context.Context, q Querier, accountID uuid.UUID, text string) (bool, error) {
	var found bool
	// The lexemes are already stemmed, so they are cast rather than passed through to_tsquery again.
	err := q.QueryRow(ctx, `WITH ask AS (SELECT (SELECT string_agg(quote_literal(l),' | ') FROM unnest(tsvector_to_array(to_tsvector('spanish_unaccent'::regconfig,$2))) l)::tsquery AS query)
 SELECT EXISTS(SELECT 1 FROM product p, ask WHERE p.account_id=$1 AND p.is_active AND to_tsvector('spanish_unaccent'::regconfig,p.canonical_name) @@ ask.query)
 OR EXISTS(SELECT 1 FROM product_synonym s, ask WHERE s.account_id=$1 AND s.search_document @@ ask.query)`, accountID, text).Scan(&found)
	return found, err
}

// PendingNameRFQ locates a retained request whose sender was asked for a name within window and has
// had no exchange since: no later message of theirs and no quote sent to any of destinations.
func (r *ClientContactRepository) PendingNameRFQ(ctx context.Context, q Querier, route domain.InboundChannelRoute, kind domain.ChannelType, value string, destinations []string, window time.Duration) (*uuid.UUID, error) {
	var id uuid.UUID
	err := q.QueryRow(ctx, `SELECT r.id FROM client_contact_observation o JOIN inbound_channel_message m ON m.account_id=o.account_id AND m.id=o.message_id JOIN rfq r ON r.account_id=m.account_id AND r.branch_id=m.branch_id AND r.id=m.rfq_id WHERE o.account_id=$1 AND m.account_id=$1 AND r.account_id=$1 AND m.branch_id=$2 AND m.channel_id=$3 AND o.kind=$4 AND o.value=$5
 AND EXISTS(SELECT 1 FROM client_contact_outbox pending WHERE pending.account_id=o.account_id AND pending.kind=o.kind AND pending.value=o.value AND pending.status='SENT' AND pending.status_changed_at>now()-make_interval(secs=>$6::double precision)
  AND NOT EXISTS(SELECT 1 FROM client_contact_observation later JOIN inbound_channel_message lm ON lm.account_id=later.account_id AND lm.id=later.message_id WHERE later.account_id=$1 AND lm.account_id=$1 AND later.kind=$4 AND later.value=$5 AND lm.created_at>pending.status_changed_at)
  AND NOT EXISTS(SELECT 1 FROM quote_send s WHERE s.account_id=$1 AND lower(s.destination)=ANY($7) AND s.sent_at>pending.status_changed_at))
 ORDER BY m.created_at,m.id LIMIT 1`, route.AccountID, route.BranchID, route.ChannelID, kind, value, window.Seconds(), destinations).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &id, err
}

// CompleteNameRequest accepts a declared name only while this provisional sender awaits one.
func (r *ClientContactRepository) CompleteNameRequest(ctx context.Context, q Querier, accountID, clientID uuid.UUID, kind domain.ChannelType, value, name string) error {
	if _, err := q.Exec(ctx, `UPDATE client SET name=$5 WHERE account_id=$1 AND id=$2 AND tax_id IS NULL
 AND EXISTS(SELECT 1 FROM client_contact_outbox pending WHERE pending.account_id=$1 AND pending.kind=$3 AND pending.value=$4 AND pending.status<>'COMPLETED')`, accountID, clientID, kind, value, name); err != nil {
		return err
	}
	_, err := q.Exec(ctx, `UPDATE client_contact_outbox SET status='COMPLETED',status_changed_at=now() WHERE account_id=$1 AND kind=$2 AND value=$3`, accountID, kind, value)
	return err
}
