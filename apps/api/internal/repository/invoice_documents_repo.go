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

// GetSnapshotSource reads a quote version without requiring a generated commercial document.
func (r *InvoicingRepository) GetSnapshotSource(ctx context.Context, q Querier, accountID, branchID, quoteID, versionID uuid.UUID) (*domain.QuoteRepresentationSource, error) {
	return NewQuoteRepresentationRepository().LoadSource(ctx, q, accountID, branchID, quoteID, versionID)
}

// GetByID loads a fiscal document within the active branch.
func (r *InvoicingRepository) GetByID(ctx context.Context, q Querier, accountID, branchID, id uuid.UUID) (*domain.Invoice, error) {
	return scanInvoice(q.QueryRow(ctx, `SELECT `+invoiceColumns+` FROM invoice WHERE account_id = $1 AND branch_id = $2 AND id = $3`, accountID, branchID, id))
}

// List returns a bounded page of the account's fiscal documents, newest first, narrowed to
// branchIDs unless it is nil.
func (r *InvoicingRepository) List(ctx context.Context, q Querier, accountID uuid.UUID, branchIDs []uuid.UUID, limit, offset int) ([]domain.Invoice, int, error) {
	var count int
	if err := q.QueryRow(ctx, `SELECT count(*) FROM invoice
	 WHERE account_id = $1 AND ($2::uuid[] IS NULL OR branch_id = ANY($2::uuid[]))`, accountID, branchIDs).Scan(&count); err != nil {
		return nil, 0, err
	}
	rows, err := q.Query(ctx, `SELECT `+invoiceColumns+` FROM invoice
	 WHERE account_id = $1 AND ($2::uuid[] IS NULL OR branch_id = ANY($2::uuid[]))
	 ORDER BY created_at DESC, id DESC LIMIT $3 OFFSET $4`, accountID, branchIDs, limit, offset)
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

// saleAwaitingInvoiceScope narrows the account's quotes to accepted, unarchived sales nothing
// invoices, holds or waives yet, in an account that invoices, inside the branch ($2) and seller ($3)
// reach. It expects the quote aliased q and its account a, with saleInvoiceFacts joined.
const saleAwaitingInvoiceScope = `WHERE q.account_id = $1
		   AND ($2::uuid[] IS NULL OR q.branch_id = ANY($2::uuid[]))
		   AND ($3::uuid IS NULL OR q.seller_id = $3 OR q.seller_id IS NULL)
		   AND a.invoicing_enabled
		   AND q.current_status = 'ACCEPTED' AND q.archived_at IS NULL AND q.invoice_waived_at IS NULL
		   AND NOT closure.invoiced AND NOT closure.pending`

// ListAwaitingInvoice returns a bounded page of the sales still waiting for an invoice and how many
// there are: paid first, then the longest accepted, since those have waited longest. The server's
// environment (env) decides which invoices count.
func (r *InvoicingRepository) ListAwaitingInvoice(
	ctx context.Context, q Querier, tenant domain.Tenant, env domain.ARCAEnvironment, limit, offset int,
) ([]domain.SaleAwaitingInvoice, int, error) {
	var count int
	if err := q.QueryRow(ctx, `SELECT count(*)
		 FROM quote q
		 JOIN account a ON a.id = q.account_id
		 `+saleInvoiceFacts("$4")+`
		 `+saleAwaitingInvoiceScope,
		tenant.AccountID, tenant.BranchFilter(), sellerParam(tenant), env).Scan(&count); err != nil {
		return nil, 0, err
	}
	rows, err := q.Query(ctx, `SELECT q.id, q.rfq_id, q.number, q.branch_id, b.name, q.client_id,
		        COALESCE(cl.name, r.client_label, ''), version.total,
		        COALESCE(accepted.accepted_at, q.updated_at), q.paid_at
		 FROM quote q
		 JOIN account a ON a.id = q.account_id
		 JOIN rfq r ON r.account_id = q.account_id AND r.id = q.rfq_id
		 JOIN branch b ON b.account_id = q.account_id AND b.id = q.branch_id
		 JOIN quote_version version ON version.account_id = q.account_id AND version.id = q.current_version_id
		 LEFT JOIN client cl ON cl.account_id = q.account_id AND cl.id = q.client_id
		 LEFT JOIN LATERAL (
		   SELECT max(change.changed_at) AS accepted_at
		   FROM quote_status_change change
		   WHERE change.account_id = q.account_id AND change.quote_id = q.id
		     AND change.new_status = 'ACCEPTED'
		 ) accepted ON TRUE
		 `+saleInvoiceFacts("$4")+`
		 `+saleAwaitingInvoiceScope+`
		 ORDER BY q.paid_at IS NULL, COALESCE(accepted.accepted_at, q.updated_at), q.number
		 LIMIT $5 OFFSET $6`,
		tenant.AccountID, tenant.BranchFilter(), sellerParam(tenant), env, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	sales := make([]domain.SaleAwaitingInvoice, 0)
	for rows.Next() {
		var sale domain.SaleAwaitingInvoice
		if err := rows.Scan(&sale.QuoteID, &sale.RFQID, &sale.Number, &sale.BranchID, &sale.BranchName,
			&sale.ClientID, &sale.ClientLabel, &sale.Total, &sale.AcceptedAt, &sale.PaidAt); err != nil {
			return nil, 0, err
		}
		sales = append(sales, sale)
	}
	return sales, count, rows.Err()
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
