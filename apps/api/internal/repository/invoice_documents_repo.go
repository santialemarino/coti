package repository

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// GetSnapshot loads the approved commercial document through tenant and branch boundaries.
func (r *InvoicingRepository) GetSnapshot(ctx context.Context, q Querier, accountID, branchID, quoteID, versionID uuid.UUID) (domain.QuoteRepresentationPayload, error) {
	representation, err := NewQuoteRepresentationRepository().GetByVersion(ctx, q, accountID, branchID, quoteID, versionID)
	if err != nil {
		return domain.QuoteRepresentationPayload{}, err
	}
	return representation.Payload, nil
}

// GetByID loads a fiscal document within the active branch.
func (r *InvoicingRepository) GetByID(ctx context.Context, q Querier, accountID, branchID, id uuid.UUID) (*domain.Invoice, error) {
	return scanInvoice(q.QueryRow(ctx, `SELECT `+invoiceColumns+` FROM invoice WHERE account_id = $1 AND branch_id = $2 AND id = $3`, accountID, branchID, id))
}

// List returns fiscal documents in a bounded branch-scoped page.
func (r *InvoicingRepository) List(ctx context.Context, q Querier, accountID, branchID uuid.UUID, limit, offset int) ([]domain.Invoice, int, error) {
	var count int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM invoice WHERE account_id = $1 AND branch_id = $2`, accountID, branchID).Scan(&count); err != nil {
		return nil, 0, err
	}
	rows, err := q.Query(ctx, `SELECT `+invoiceColumns+` FROM invoice WHERE account_id = $1 AND branch_id = $2 ORDER BY created_at DESC, id DESC LIMIT $3 OFFSET $4`, accountID, branchID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]domain.Invoice, 0)
	for rows.Next() {
		inv, err := scanInvoice(rows)
		if err != nil {
			return nil, 0, err
		}
		result = append(result, *inv)
	}
	return result, count, rows.Err()
}

// ClaimRecovery leases a stale pending document before consulting its uncertain outcome.
func (r *InvoicingRepository) ClaimRecovery(ctx context.Context, q Querier, accountID, branchID, id uuid.UUID, before time.Time) (bool, error) {
	tag, err := q.Exec(ctx, `UPDATE invoice SET updated_at = now() WHERE account_id = $1 AND branch_id = $2 AND id = $3 AND status = 'PENDING' AND updated_at < $4`, accountID, branchID, id, before)
	return tag.RowsAffected() == 1, err
}

func mustInvoiceSnapshot(snapshot domain.QuoteRepresentationPayload) []byte {
	raw, _ := json.Marshal(snapshot)
	return raw
}
