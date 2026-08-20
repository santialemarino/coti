package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const rfqAttachmentColumns = `id, account_id, rfq_id, type, file_url, extracted_text,
	processing_status, created_at, processed_at`

// RFQAttachmentRepository owns persistence for the files an RFQ arrived with.
type RFQAttachmentRepository struct{}

// NewRFQAttachmentRepository builds an RFQAttachmentRepository.
func NewRFQAttachmentRepository() *RFQAttachmentRepository {
	return &RFQAttachmentRepository{}
}

// ListByRFQ returns one RFQ's attachments, oldest first. branchID scopes the parent RFQ, which
// row level security does not: it guards the account boundary and nothing inside it.
func (r *RFQAttachmentRepository) ListByRFQ(
	ctx context.Context, q Querier, accountID, branchID, rfqID uuid.UUID,
) ([]domain.RFQAttachment, error) {
	rows, err := q.Query(ctx,
		`SELECT a.id, a.account_id, a.rfq_id, a.type, a.file_url, a.extracted_text,
		        a.processing_status, a.created_at, a.processed_at
		 FROM rfq_attachment a
		 JOIN rfq r ON r.id = a.rfq_id
		 WHERE a.account_id = $1 AND r.account_id = $1 AND r.branch_id = $2 AND a.rfq_id = $3
		 ORDER BY a.created_at, a.id`,
		accountID, branchID, rfqID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	attachments := make([]domain.RFQAttachment, 0)
	for rows.Next() {
		var a domain.RFQAttachment
		if err := rows.Scan(&a.ID, &a.AccountID, &a.RFQID, &a.Type, &a.StorageKey,
			&a.ExtractedText, &a.ProcessingStatus, &a.CreatedAt, &a.ProcessedAt); err != nil {
			return nil, err
		}
		attachments = append(attachments, a)
	}
	return attachments, rows.Err()
}

// GetByID loads one attachment. Returns domain.ErrNotFound when it belongs to another account,
// to another branch, or does not exist — which under row level security are indistinguishable.
func (r *RFQAttachmentRepository) GetByID(
	ctx context.Context, q Querier, accountID, branchID, id uuid.UUID,
) (*domain.RFQAttachment, error) {
	return scanRFQAttachment(q.QueryRow(ctx,
		`SELECT a.id, a.account_id, a.rfq_id, a.type, a.file_url, a.extracted_text,
		        a.processing_status, a.created_at, a.processed_at
		 FROM rfq_attachment a
		 JOIN rfq r ON r.id = a.rfq_id
		 WHERE a.account_id = $1 AND r.account_id = $1 AND r.branch_id = $2 AND a.id = $3`,
		accountID, branchID, id))
}

// Create records a stored file against an RFQ. The RFQ id arrives from the request, and its
// foreign key would accept another account's or another branch's row, so the insert proves
// both before it writes. Returns domain.ErrNotFound when it cannot.
func (r *RFQAttachmentRepository) Create(
	ctx context.Context, q Querier, accountID, branchID uuid.UUID, in domain.NewRFQAttachment,
) (*domain.RFQAttachment, error) {
	return scanRFQAttachment(q.QueryRow(ctx,
		`INSERT INTO rfq_attachment (id, account_id, rfq_id, type, file_url)
		 SELECT $1, $2, $3, $4, $5
		 WHERE EXISTS (
		     SELECT 1 FROM rfq WHERE id = $3 AND account_id = $2 AND branch_id = $6
		 )
		 RETURNING `+rfqAttachmentColumns,
		in.ID, accountID, in.RFQID, in.Type, in.StorageKey, branchID))
}

func scanRFQAttachment(row pgx.Row) (*domain.RFQAttachment, error) {
	var a domain.RFQAttachment
	err := row.Scan(&a.ID, &a.AccountID, &a.RFQID, &a.Type, &a.StorageKey, &a.ExtractedText,
		&a.ProcessingStatus, &a.CreatedAt, &a.ProcessedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}
