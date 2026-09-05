package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const rfqColumns = `id, account_id, branch_id, client_id, channel_id, raw_text, status, work_type,
	received_at, created_at, updated_at, client_label`

const rfqStatusChangeColumns = `id, account_id, rfq_id, previous_status, new_status, user_id,
	changed_at, created_at`

// RFQRepository owns persistence for RFQs and their status history.
type RFQRepository struct{}

// NewRFQRepository builds an RFQRepository.
func NewRFQRepository() *RFQRepository {
	return &RFQRepository{}
}

// Create inserts an RFQ source record. Returns domain.ErrNotFound when in.ClientID names a
// client this account does not have.
func (r *RFQRepository) Create(
	ctx context.Context, q Querier, accountID uuid.UUID, in domain.NewRFQ,
) (*domain.RFQ, error) {
	if in.Status == "" {
		in.Status = domain.RFQStatusReceived
	}
	// The client is the one reference that arrives from the request body, and the foreign key
	// would accept another account's: client hangs off account_id, so the row has to be proved
	// to be this account's before it can be pointed at.
	return scanRFQ(q.QueryRow(ctx,
		`INSERT INTO rfq (account_id, branch_id, client_id, channel_id, raw_text, status,
		                  work_type, client_label)
		 SELECT $1, $2, $3, $4, $5, $6, $7, $8
		 WHERE $3::uuid IS NULL
		    OR EXISTS (SELECT 1 FROM client WHERE account_id = $1 AND id = $3::uuid)
		 RETURNING `+rfqColumns,
		accountID, in.BranchID, in.ClientID, in.ChannelID, in.RawText, in.Status, in.WorkType,
		in.ClientLabel))
}

// UpdateStatus writes the RFQ status cache and returns the stored row.
func (r *RFQRepository) UpdateStatus(
	ctx context.Context, q Querier, accountID, id uuid.UUID, status domain.RFQStatus,
) (*domain.RFQ, error) {
	return scanRFQ(q.QueryRow(ctx,
		`UPDATE rfq
		 SET status = $3
		 WHERE account_id = $1 AND id = $2
		 RETURNING `+rfqColumns,
		accountID, id, status))
}

// SetClient attaches an account-scoped client to the quote's source RFQ.
func (r *RFQRepository) SetClient(
	ctx context.Context, q Querier, accountID, branchID, rfqID, clientID uuid.UUID,
) error {
	tag, err := q.Exec(ctx, `UPDATE rfq SET client_id = $4
		WHERE account_id = $1 AND branch_id = $2 AND id = $3
		  AND EXISTS (SELECT 1 FROM client WHERE account_id = $1 AND id = $4)`,
		accountID, branchID, rfqID, clientID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// AppendStatusChange records an RFQ lifecycle transition.
func (r *RFQRepository) AppendStatusChange(
	ctx context.Context, q Querier, accountID, rfqID uuid.UUID, previousStatus *domain.RFQStatus,
	newStatus domain.RFQStatus, userID *uuid.UUID,
) (*domain.RFQStatusChange, error) {
	return scanRFQStatusChange(q.QueryRow(ctx,
		`INSERT INTO rfq_status_change (account_id, rfq_id, previous_status, new_status, user_id)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+rfqStatusChangeColumns,
		accountID, rfqID, previousStatus, newStatus, userID))
}

func scanRFQ(row pgx.Row) (*domain.RFQ, error) {
	var rfq domain.RFQ
	err := row.Scan(&rfq.ID, &rfq.AccountID, &rfq.BranchID, &rfq.ClientID, &rfq.ChannelID,
		&rfq.RawText, &rfq.Status, &rfq.WorkType, &rfq.ReceivedAt, &rfq.CreatedAt,
		&rfq.UpdatedAt, &rfq.ClientLabel)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &rfq, nil
}

func scanRFQStatusChange(row pgx.Row) (*domain.RFQStatusChange, error) {
	var change domain.RFQStatusChange
	err := row.Scan(&change.ID, &change.AccountID, &change.RFQID, &change.PreviousStatus,
		&change.NewStatus, &change.UserID, &change.ChangedAt, &change.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &change, nil
}
