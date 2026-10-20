package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// GmailMailboxRepository owns Gmail credentials, authorization states, and cursors.
type GmailMailboxRepository struct{}

// NewGmailMailboxRepository builds a mailbox repository.
func NewGmailMailboxRepository() *GmailMailboxRepository { return &GmailMailboxRepository{} }

// ListConnectedCrossAccount discovers active mailboxes for the synchronization worker.
func (r *GmailMailboxRepository) ListConnectedCrossAccount(ctx context.Context, q Querier) ([]domain.GmailMailbox, error) {
	rows, err := q.Query(ctx,
		`SELECT m.account_id, m.branch_id, m.channel_id, m.email_address,
		        m.refresh_token_ciphertext, m.history_id, m.connected_at, m.initial_sync_pending, m.last_synced_at
		 FROM gmail_mailbox m
		 JOIN channel c ON c.id = m.channel_id AND c.account_id = m.account_id AND c.branch_id = m.branch_id
		 JOIN branch b ON b.id = m.branch_id AND b.account_id = m.account_id
		 JOIN account a ON a.id = m.account_id
		 WHERE c.is_active AND c.type = 'EMAIL' AND lower(c.identifier) = lower(m.email_address)
		   AND b.is_active AND a.is_active
		 ORDER BY m.last_synced_at NULLS FIRST, m.channel_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var mailboxes []domain.GmailMailbox
	for rows.Next() {
		var m domain.GmailMailbox
		if err := rows.Scan(&m.Route.AccountID, &m.Route.BranchID, &m.Route.ChannelID, &m.Address,
			&m.RefreshTokenCiphertext, &m.HistoryID, &m.ConnectedAt, &m.InitialSyncPending, &m.LastSyncedAt); err != nil {
			return nil, err
		}
		m.Route.ChannelType = domain.ChannelTypeEmail
		mailboxes = append(mailboxes, m)
	}
	return mailboxes, rows.Err()
}

// GetConnected returns the current credential and cursor for a branch's matching active channel.
func (r *GmailMailboxRepository) GetConnected(ctx context.Context, q Querier,
	accountID, branchID, channelID uuid.UUID,
) (*domain.GmailMailbox, error) {
	var m domain.GmailMailbox
	err := q.QueryRow(ctx,
		`SELECT m.email_address, m.refresh_token_ciphertext, m.history_id, m.connected_at, m.initial_sync_pending, m.last_synced_at
		 FROM gmail_mailbox m
		 JOIN channel c ON c.id = m.channel_id AND c.account_id = m.account_id AND c.branch_id = m.branch_id
		 JOIN branch b ON b.id = m.branch_id AND b.account_id = m.account_id
		 JOIN account a ON a.id = m.account_id
		 WHERE m.account_id = $1 AND m.branch_id = $2 AND m.channel_id = $3
		   AND c.is_active AND c.type = 'EMAIL' AND lower(c.identifier) = lower(m.email_address)
		   AND b.is_active AND a.is_active`, accountID, branchID, channelID).
		Scan(&m.Address, &m.RefreshTokenCiphertext, &m.HistoryID, &m.ConnectedAt, &m.InitialSyncPending, &m.LastSyncedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	m.Route = domain.InboundChannelRoute{AccountID: accountID, BranchID: branchID,
		ChannelID: channelID, ChannelType: domain.ChannelTypeEmail}
	return &m, nil
}

// CreateAuthorization stores one OAuth state under the initiating tenant.
func (r *GmailMailboxRepository) CreateAuthorization(ctx context.Context, q Querier, in domain.GmailAuthorization) error {
	_, err := q.Exec(ctx,
		`WITH expired AS (
		   DELETE FROM gmail_authorization_state WHERE account_id = $2 AND expires_at <= now()
		 )
		 INSERT INTO gmail_authorization_state
		 (state_hash, account_id, branch_id, channel_id, user_id, session_epoch, verifier_ciphertext, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, in.StateHash, in.AccountID, in.BranchID,
		in.ChannelID, in.UserID, in.SessionEpoch, in.VerifierCiphertext, in.ExpiresAt)
	return err
}

// ConsumeAuthorization atomically consumes an unexpired opaque OAuth state.
func (r *GmailMailboxRepository) ConsumeAuthorization(ctx context.Context, q Querier,
	stateHash []byte, now time.Time,
) (*domain.GmailAuthorization, error) {
	var a domain.GmailAuthorization
	err := q.QueryRow(ctx,
		`DELETE FROM gmail_authorization_state
		 WHERE state_hash = $1 AND expires_at > $2
		 RETURNING account_id, branch_id, channel_id, user_id, session_epoch, verifier_ciphertext, expires_at`,
		stateHash, now).Scan(&a.AccountID, &a.BranchID, &a.ChannelID, &a.UserID,
		&a.SessionEpoch, &a.VerifierCiphertext, &a.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return &a, err
}

// SaveConnected stores an encrypted credential without discarding an existing mailbox cursor.
func (r *GmailMailboxRepository) SaveConnected(ctx context.Context, q Querier, m domain.GmailMailbox) error {
	tag, err := q.Exec(ctx,
		`INSERT INTO gmail_mailbox
		 (channel_id, account_id, branch_id, email_address, refresh_token_ciphertext, history_id, connected_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT (channel_id) DO UPDATE SET
		   email_address = EXCLUDED.email_address,
		   refresh_token_ciphertext = EXCLUDED.refresh_token_ciphertext,
		   history_id = CASE WHEN lower(gmail_mailbox.email_address) = lower(EXCLUDED.email_address)
		                     THEN gmail_mailbox.history_id ELSE EXCLUDED.history_id END,
		   connected_at = CASE WHEN lower(gmail_mailbox.email_address) = lower(EXCLUDED.email_address)
		                       THEN gmail_mailbox.connected_at ELSE EXCLUDED.connected_at END,
		   initial_sync_pending = CASE WHEN lower(gmail_mailbox.email_address) = lower(EXCLUDED.email_address)
		                               THEN gmail_mailbox.initial_sync_pending ELSE TRUE END,
		   updated_at = now()
		 WHERE gmail_mailbox.account_id = EXCLUDED.account_id AND gmail_mailbox.branch_id = EXCLUDED.branch_id`,
		m.Route.ChannelID, m.Route.AccountID, m.Route.BranchID, m.Address,
		m.RefreshTokenCiphertext, m.HistoryID, m.ConnectedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrConflict
	}
	return nil
}

// SaveCursor advances only the mailbox version that was actually synchronized.
func (r *GmailMailboxRepository) SaveCursor(ctx context.Context, q Querier,
	m domain.GmailMailbox, cursor string, now time.Time,
) error {
	tag, err := q.Exec(ctx,
		`UPDATE gmail_mailbox SET history_id = $5, initial_sync_pending = FALSE, last_synced_at = $6, updated_at = $6
		 WHERE account_id = $1 AND branch_id = $2 AND channel_id = $3
		   AND history_id = $4 AND refresh_token_ciphertext = $7 AND initial_sync_pending = $8`,
		m.Route.AccountID, m.Route.BranchID, m.Route.ChannelID, m.HistoryID, cursor, now,
		m.RefreshTokenCiphertext, m.InitialSyncPending)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return domain.ErrConflict
	}
	return nil
}
