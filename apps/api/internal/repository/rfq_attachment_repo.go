package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

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

/*
 * ClaimPending takes the next attachments of the given types and marks them PROCESSING in the same
 * statement, so a row is never handed to two workers. It crosses every account by design: a sweep
 * runs as the owner. A claim expires after reclaimAfter, or a run killed mid-work would park its
 * rows at PROCESSING forever — the same leak it exists to drain.
 *
 * The selection is a CTE rather than `id IN (SELECT … LIMIT n)`, and that is load-bearing: the
 * planner reads the IN form as a semi-join whose inner side it may re-execute per outer row, so
 * the LIMIT stops bounding anything and one firing claims the whole queue.
 */
func (r *RFQAttachmentRepository) ClaimPending(
	ctx context.Context, q Querier, types []domain.AttachmentType, limit int,
	reclaimAfter time.Duration, now time.Time,
) ([]domain.ClaimedAttachment, error) {
	// pgx has no encode plan for a slice of the named type, so the enum array crosses as strings.
	wanted := make([]string, 0, len(types))
	for _, t := range types {
		wanted = append(wanted, string(t))
	}

	rows, err := q.Query(ctx,
		`WITH claimed AS (
		        SELECT a.id
		          FROM rfq_attachment a
		         WHERE a.type = ANY($4::attachment_type[])
		           AND (a.processing_status = 'PENDING'
		                OR (a.processing_status = 'PROCESSING'
		                    AND a.processing_started_at < $2::timestamptz - $3::interval))
		         ORDER BY a.created_at, a.id
		         LIMIT $1
		           FOR UPDATE SKIP LOCKED)
		 UPDATE rfq_attachment
		    SET processing_status = 'PROCESSING', processing_started_at = $2
		   FROM claimed
		  WHERE rfq_attachment.id = claimed.id
		 RETURNING rfq_attachment.id, rfq_attachment.account_id, rfq_attachment.rfq_id,
		           rfq_attachment.type, COALESCE(rfq_attachment.file_url, '')`,
		limit, now, reclaimAfter.String(), wanted)
	if err != nil {
		return nil, err
	}

	claimed := make([]domain.ClaimedAttachment, 0)
	for rows.Next() {
		var a domain.ClaimedAttachment
		if err := rows.Scan(&a.ID, &a.AccountID, &a.RFQID, &a.Type, &a.StorageKey); err != nil {
			rows.Close()
			return nil, err
		}
		claimed = append(claimed, a)
	}
	// Closed before the next query rather than deferred: both run on one connection, and pgx
	// refuses a second query while the first result set is still open.
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return r.fillBranches(ctx, q, claimed)
}

// MarkProcessed records what the multi-format engine read out of one attachment and closes it
// out. An empty text is stored as NULL: an image yields none, and the file is the record.
func (r *RFQAttachmentRepository) MarkProcessed(
	ctx context.Context, q Querier, accountID, attachmentID uuid.UUID, extractedText *string,
	status domain.AttachmentProcessingStatus, processedAt time.Time,
) error {
	tag, err := q.Exec(ctx,
		`UPDATE rfq_attachment
		    SET extracted_text = $3, processing_status = $4, processed_at = $5
		  WHERE account_id = $1 AND id = $2`,
		accountID, attachmentID, extractedText, status, processedAt)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

/*
 * fillBranches reads each claimed attachment's branch off its RFQ in one round trip, because the
 * attachment row carries none and every service the sweep calls is branch-scoped.
 *
 * It matches on the account as well as the id even though the sweep already runs as the owner: an
 * attachment whose RFQ belongs to another account is a corrupt row, and handing that RFQ's branch
 * downstream would carry the mistake into everything the sweep then does. Unmatched means refused,
 * not defaulted.
 */
func (r *RFQAttachmentRepository) fillBranches(
	ctx context.Context, q Querier, claimed []domain.ClaimedAttachment,
) ([]domain.ClaimedAttachment, error) {
	if len(claimed) == 0 {
		return claimed, nil
	}
	rfqIDs := make([]uuid.UUID, 0, len(claimed))
	accountIDs := make([]uuid.UUID, 0, len(claimed))
	for _, a := range claimed {
		rfqIDs = append(rfqIDs, a.RFQID)
		accountIDs = append(accountIDs, a.AccountID)
	}
	rows, err := q.Query(ctx,
		`SELECT r.id, r.branch_id
		   FROM rfq r
		   JOIN unnest($1::uuid[], $2::uuid[]) AS wanted(rfq_id, account_id)
		     ON r.id = wanted.rfq_id AND r.account_id = wanted.account_id`,
		rfqIDs, accountIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	branches := make(map[uuid.UUID]uuid.UUID, len(claimed))
	for rows.Next() {
		var rfqID, branchID uuid.UUID
		if err := rows.Scan(&rfqID, &branchID); err != nil {
			return nil, err
		}
		branches[rfqID] = branchID
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range claimed {
		branchID, ok := branches[claimed[i].RFQID]
		if !ok {
			return nil, fmt.Errorf("%w: attachment %s names an rfq its account does not own",
				domain.ErrNotFound, claimed[i].ID)
		}
		claimed[i].BranchID = branchID
	}
	return claimed, nil
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
