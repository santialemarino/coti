package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const customerOrderFrom = ` FROM rfq r
 JOIN branch b ON b.account_id=r.account_id AND b.id=r.branch_id
 LEFT JOIN quote qt ON qt.account_id=r.account_id AND qt.branch_id=r.branch_id AND qt.rfq_id=r.id
 LEFT JOIN client cl ON cl.account_id=r.account_id AND cl.id=COALESCE(qt.client_id,r.client_id)
 LEFT JOIN LATERAL (
   SELECT s.public_token FROM quote_send s
   JOIN quote_version v ON v.account_id=s.account_id AND v.id=s.version_id AND v.is_immutable
   JOIN channel ch ON ch.account_id=s.account_id AND ch.branch_id=r.branch_id AND ch.id=s.channel_id
   WHERE s.account_id=r.account_id AND v.quote_id=qt.id AND s.tracking_status IN ('SENT','DELIVERED','VIEWED')
   ORDER BY v.version_number DESC,s.sent_at DESC,s.id DESC LIMIT 1
 ) published ON TRUE
 WHERE r.account_id=$1 AND (cl.tax_id=$2 OR EXISTS (
   SELECT 1 FROM inbound_channel_message m
   JOIN channel ch ON ch.account_id=m.account_id AND ch.branch_id=m.branch_id AND ch.id=m.channel_id AND ch.type='WEBAPP'
   WHERE m.account_id=r.account_id AND m.branch_id=r.branch_id AND m.rfq_id=r.id AND m.payload->'contact'->>'tax_id'=$2
 ))`

const customerOrderColumns = `r.id,r.branch_id,b.name,r.received_at,
 CASE WHEN published.public_token IS NULL THEN 'RECEIVED' WHEN qt.current_status IN ('DRAFT','QUOTED') THEN 'SENT' ELSE qt.current_status::text END,
 COALESCE(r.raw_text,''),published.public_token`

// CustomerOrderRepository reads CUIT/CUIL-matched requests within one supplier.
type CustomerOrderRepository struct{}

// NewCustomerOrderRepository builds the customer history repository.
func NewCustomerOrderRepository() *CustomerOrderRepository { return &CustomerOrderRepository{} }

// List returns a bounded page without loading internal quote drafts.
func (r *CustomerOrderRepository) List(ctx context.Context, q Querier, accountID uuid.UUID, taxID string, limit, offset int) ([]domain.CustomerOrder, error) {
	rows, err := q.Query(ctx, `SELECT `+customerOrderColumns+customerOrderFrom+` ORDER BY r.received_at DESC,r.id DESC LIMIT $3 OFFSET $4`, accountID, taxID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.CustomerOrder, 0)
	for rows.Next() {
		item, err := scanCustomerOrder(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *item)
	}
	return result, rows.Err()
}

// Get checks ownership independently of the caller's knowledge of a request ID.
func (r *CustomerOrderRepository) Get(ctx context.Context, q Querier, accountID uuid.UUID, taxID string, id uuid.UUID) (*domain.CustomerOrder, error) {
	return scanCustomerOrder(q.QueryRow(ctx, `SELECT `+customerOrderColumns+customerOrderFrom+` AND r.id=$3`, accountID, taxID, id))
}

// GetByToken authorizes both active and expired legacy links without exposing their contents.
func (r *CustomerOrderRepository) GetByToken(ctx context.Context, q Querier, accountID uuid.UUID, taxID, token string) (*domain.CustomerOrder, error) {
	return scanCustomerOrder(q.QueryRow(ctx, `SELECT `+customerOrderColumns+customerOrderFrom+` AND EXISTS (
 SELECT 1 FROM quote_send s JOIN quote_version v ON v.account_id=s.account_id AND v.id=s.version_id AND v.is_immutable
 JOIN channel ch ON ch.account_id=s.account_id AND ch.branch_id=r.branch_id AND ch.id=s.channel_id
 WHERE s.account_id=r.account_id AND v.quote_id=qt.id AND s.public_token=$3 AND s.tracking_status IN ('SENT','DELIVERED','VIEWED'))`, accountID, taxID, token))
}

func scanCustomerOrder(row pgx.Row) (*domain.CustomerOrder, error) {
	var item domain.CustomerOrder
	err := row.Scan(&item.ID, &item.BranchID, &item.BranchName, &item.ReceivedAt, &item.Status, &item.OriginalText, &item.DeliveryToken)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}
