package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const rfqColumns = `id, account_id, branch_id, client_id, channel_id, raw_text, status, work_type,
	received_at, created_at, updated_at, client_label`

const rfqStatusChangeColumns = `id, account_id, rfq_id, previous_status, new_status, user_id,
	changed_at, created_at`

const rfqClarificationColumns = `id, account_id, rfq_id, quote_item_id, issue_type,
	requested_description, question, reason, status, approved_question, decided_by, decided_at,
	sent_at, answer, answered_at, created_at`

// RFQRepository owns persistence for RFQs and their status history.
type RFQRepository struct{}

// NewRFQRepository builds an RFQRepository.
func NewRFQRepository() *RFQRepository {
	return &RFQRepository{}
}

// Create inserts an RFQ source record.
func (r *RFQRepository) Create(
	ctx context.Context, q Querier, accountID uuid.UUID, in domain.NewRFQ,
) (*domain.RFQ, error) {
	if in.Status == "" {
		in.Status = domain.RFQStatusReceived
	}
	return scanRFQ(q.QueryRow(ctx,
		`INSERT INTO rfq (account_id, branch_id, client_id, channel_id, raw_text, status,
		                  work_type, client_label)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
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

// CreateClarifications inserts proposed RFQ questions in one statement.
func (r *RFQRepository) CreateClarifications(
	ctx context.Context, q Querier, accountID, rfqID uuid.UUID,
	clarifications []domain.NewRFQClarification,
) ([]domain.RFQClarification, error) {
	if len(clarifications) == 0 {
		return []domain.RFQClarification{}, nil
	}
	payload, err := json.Marshal(rfqClarificationPayloads(clarifications))
	if err != nil {
		return nil, err
	}

	rows, err := q.Query(ctx,
		`WITH incoming AS (
		   SELECT *
		   FROM jsonb_to_recordset($3::jsonb) AS x(
		     quote_item_id uuid,
		     issue_type text,
		     requested_description text,
		     question text,
		     reason text
		   )
		 )
		 INSERT INTO rfq_clarification (
		   account_id, rfq_id, quote_item_id, issue_type, requested_description, question, reason
		 )
		 SELECT $1, $2, incoming.quote_item_id,
		        incoming.issue_type::rfq_clarification_issue_type,
		        incoming.requested_description, incoming.question, incoming.reason
		 FROM incoming
		 JOIN rfq source ON source.account_id = $1 AND source.id = $2
		 LEFT JOIN quote_item item
		   ON item.account_id = $1 AND item.id = incoming.quote_item_id
		 LEFT JOIN quote_version version
		   ON version.account_id = $1 AND version.id = item.version_id
		 LEFT JOIN quote item_quote
		   ON item_quote.account_id = $1 AND item_quote.id = version.quote_id
		  AND item_quote.rfq_id = $2
		 WHERE incoming.quote_item_id IS NULL OR item_quote.id IS NOT NULL
		 RETURNING `+rfqClarificationColumns,
		accountID, rfqID, payload)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	created := make([]domain.RFQClarification, 0, len(clarifications))
	for rows.Next() {
		clarification, scanErr := scanRFQClarification(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		created = append(created, *clarification)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(created) != len(clarifications) {
		return nil, domain.ErrNotFound
	}
	return created, nil
}

type rfqClarificationPayload struct {
	QuoteItemID          *uuid.UUID `json:"quote_item_id"`
	IssueType            string     `json:"issue_type"`
	RequestedDescription string     `json:"requested_description"`
	Question             string     `json:"question"`
	Reason               string     `json:"reason"`
}

func rfqClarificationPayloads(
	clarifications []domain.NewRFQClarification,
) []rfqClarificationPayload {
	payloads := make([]rfqClarificationPayload, 0, len(clarifications))
	for _, clarification := range clarifications {
		payloads = append(payloads, rfqClarificationPayload{
			QuoteItemID:          clarification.QuoteItemID,
			IssueType:            string(clarification.IssueType),
			RequestedDescription: clarification.RequestedDescription,
			Question:             clarification.Question,
			Reason:               clarification.Reason,
		})
	}
	return payloads
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

func scanRFQClarification(row pgx.Row) (*domain.RFQClarification, error) {
	var clarification domain.RFQClarification
	err := row.Scan(&clarification.ID, &clarification.AccountID, &clarification.RFQID,
		&clarification.QuoteItemID, &clarification.IssueType,
		&clarification.RequestedDescription, &clarification.Question, &clarification.Reason,
		&clarification.Status, &clarification.ApprovedQuestion, &clarification.DecidedBy,
		&clarification.DecidedAt, &clarification.SentAt, &clarification.Answer,
		&clarification.AnsweredAt, &clarification.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &clarification, nil
}
