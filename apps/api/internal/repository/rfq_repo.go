package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const rfqColumns = `id, account_id, branch_id, client_id, channel_id, raw_text, status, work_type,
	received_at, created_at, updated_at, client_label`

const rfqStatusChangeColumns = `id, account_id, rfq_id, previous_status, new_status, user_id,
	changed_at, created_at`

// RFQRepository owns persistence for RFQs, their status history, and the quote a
// manual entry materializes.
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

// GetByRFQID returns a single RFQ scoped to the account, including its associated quote
// data when present. This is the detail view backend.
func (r *RFQRepository) GetByRFQID(
	ctx context.Context, q Querier, accountID, rfqID uuid.UUID,
) (*domain.RfqListItem, error) {
	var item domain.RfqListItem
	var total decimal.Decimal
	err := q.QueryRow(ctx,
		`SELECT r.id, r.client_id,
		        COALESCE(cl.name, r.client_label), r.created_at,
		        lower(c.type::text),
		        q.seller_id,
		        COALESCE(u.name, ''),
		        r.branch_id,
		        b.name,
		        COALESCE(qt.total, 0),
		        CASE WHEN q.id IS NULL THEN r.status::text
		             ELSE q.current_status::text
		        END,
		        q.archived_at,
		        COALESCE(q.needs_followup, FALSE),
		        (SELECT count(*)
		         FROM quote_item qi
		         JOIN quote_version qv ON qv.id = qi.version_id AND qv.account_id = qi.account_id
		         JOIN quote q2 ON q2.id = qv.quote_id AND q2.account_id = qv.account_id
		         WHERE qi.account_id = r.account_id AND q2.rfq_id = r.id)
		 FROM rfq r
		 JOIN branch b ON b.id = r.branch_id AND b.account_id = r.account_id
		 LEFT JOIN client cl ON cl.id = r.client_id AND cl.account_id = r.account_id
		 LEFT JOIN quote q ON q.rfq_id = r.id AND q.account_id = r.account_id
		 LEFT JOIN channel c ON c.id = r.channel_id AND c.account_id = r.account_id
		 LEFT JOIN app_user u ON u.id = q.seller_id AND u.account_id = r.account_id
		 LEFT JOIN quote_version qt ON qt.id = q.current_version_id AND qt.account_id = r.account_id
		 WHERE r.account_id = $1 AND r.id = $2`,
		accountID, rfqID,
	).Scan(
		&item.ID, &item.ClientID,
		&item.ClientLabel, &item.CreatedAt,
		&item.Channel, &item.SellerID, &item.SellerName,
		&item.BranchID, &item.BranchName,
		&total, &item.Status, &item.ArchivedAt, &item.NeedsFollowup,
		&item.ItemCount,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if total.IsZero() {
		item.Total = nil
	} else {
		s := total.String()
		item.Total = &s
	}
	return &item, nil
}

// ListByTenant returns the RFQ list the Backoffice dashboard consumes. Archived quotes are
// hidden, needs_followup rows sort first, and the client display name falls back to
// rfq.client_label when no ficha client is linked.
func (r *RFQRepository) ListByTenant(
	ctx context.Context, q Querier, tenant domain.Tenant,
) ([]domain.RfqListItem, error) {
	rows, err := q.Query(ctx,
		`SELECT r.id, r.client_id,
		        COALESCE(cl.name, r.client_label), r.created_at,
		        lower(c.type::text),
		        q.seller_id,
		        COALESCE(u.name, ''),
		        r.branch_id,
		        b.name,
		        COALESCE(qt.total, 0),
		        CASE WHEN q.id IS NULL THEN r.status::text
		             ELSE q.current_status::text
		        END,
		        q.archived_at,
		        COALESCE(q.needs_followup, FALSE),
		        (SELECT count(*)
		         FROM quote_item qi
		         JOIN quote_version qv ON qv.id = qi.version_id AND qv.account_id = qi.account_id
		         JOIN quote q2 ON q2.id = qv.quote_id AND q2.account_id = qv.account_id
		         WHERE qi.account_id = r.account_id AND q2.rfq_id = r.id)
		 FROM rfq r
		 JOIN branch b ON b.id = r.branch_id AND b.account_id = r.account_id
		 LEFT JOIN client cl ON cl.id = r.client_id AND cl.account_id = r.account_id
		 LEFT JOIN quote q ON q.rfq_id = r.id AND q.account_id = r.account_id
		 LEFT JOIN channel c ON c.id = r.channel_id AND c.account_id = r.account_id
		 LEFT JOIN app_user u ON u.id = q.seller_id AND u.account_id = r.account_id
		 LEFT JOIN quote_version qt ON qt.id = q.current_version_id AND qt.account_id = r.account_id
		 WHERE r.account_id = $1
		   AND ($2::uuid = '00000000-0000-0000-0000-000000000000'::uuid OR r.branch_id = $2)
		   AND (q.id IS NULL OR q.archived_at IS NULL)
		 ORDER BY COALESCE(q.needs_followup, FALSE) DESC, r.created_at DESC`,
		tenant.AccountID, tenant.BranchID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.RfqListItem
	for rows.Next() {
		var item domain.RfqListItem
		var total decimal.Decimal
		if err := rows.Scan(
			&item.ID, &item.ClientID,
			&item.ClientLabel, &item.CreatedAt,
			&item.Channel, &item.SellerID, &item.SellerName,
			&item.BranchID, &item.BranchName,
			&total, &item.Status, &item.ArchivedAt, &item.NeedsFollowup,
			&item.ItemCount,
		); err != nil {
			return nil, err
		}
		if total.IsZero() {
			item.Total = nil
		} else {
			s := total.String()
			item.Total = &s
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// GetManualEntryChannelID returns the branch's manual-entry channel.
func (r *RFQRepository) GetManualEntryChannelID(
	ctx context.Context, q Querier, accountID, branchID uuid.UUID,
) (uuid.UUID, error) {
	var id uuid.UUID
	err := q.QueryRow(ctx,
		`SELECT id
		 FROM channel
		 WHERE account_id = $1 AND branch_id = $2 AND type = 'MANUAL_ENTRY' AND identifier IS NULL
		 ORDER BY created_at
		 LIMIT 1`,
		accountID, branchID,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, domain.ErrNotFound
	}
	if err != nil {
		return uuid.Nil, err
	}
	return id, nil
}

// CountProductsInAccount reports how many of the given ids are catalog products of the account.
func (r *RFQRepository) CountProductsInAccount(
	ctx context.Context, q Querier, accountID uuid.UUID, productIDs []uuid.UUID,
) (int, error) {
	var count int
	err := q.QueryRow(ctx,
		`SELECT count(*)
		 FROM product
		 WHERE account_id = $1 AND id = ANY($2)`,
		accountID, productIDs,
	).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// CreateManualEntry persists a manual RFQ and everything it needs in one transaction.
func (r *RFQRepository) CreateManualEntry(
	ctx context.Context, q Querier, tenant domain.Tenant, channelID uuid.UUID,
	in domain.NewRfq, now time.Time,
) (*domain.RfqCreation, error) {
	var creation domain.RfqCreation

	err := q.QueryRow(ctx,
		`INSERT INTO rfq (account_id, branch_id, client_label, channel_id, raw_text, status, work_type, received_at)
		 VALUES ($1, $2, $3, $4, $5, 'GENERATED', $6, $7)
		 RETURNING id, received_at, created_at, updated_at`,
		tenant.AccountID, tenant.BranchID, in.ClientLabel, channelID, in.RawText, in.WorkType, now,
	).Scan(&creation.Rfq.ID, &creation.Rfq.ReceivedAt, &creation.Rfq.CreatedAt, &creation.Rfq.UpdatedAt)
	if err != nil {
		return nil, err
	}
	creation.Rfq.AccountID = tenant.AccountID
	creation.Rfq.BranchID = tenant.BranchID
	creation.Rfq.ClientLabel = in.ClientLabel
	creation.Rfq.ChannelID = channelID
	creation.Rfq.RawText = in.RawText
	creation.Rfq.Status = domain.RFQStatusGenerated
	creation.Rfq.WorkType = in.WorkType

	err = q.QueryRow(ctx,
		`INSERT INTO quote (account_id, branch_id, rfq_id, seller_id, current_status)
		 VALUES ($1, $2, $3, $4, 'DRAFT')
		 RETURNING id, created_at, updated_at`,
		tenant.AccountID, tenant.BranchID, creation.Rfq.ID, tenant.UserID,
	).Scan(&creation.Quote.ID, &creation.Quote.CreatedAt, &creation.Quote.UpdatedAt)
	if err != nil {
		return nil, err
	}
	creation.Quote.AccountID = tenant.AccountID
	creation.Quote.BranchID = tenant.BranchID
	creation.Quote.RFQID = creation.Rfq.ID
	creation.Quote.SellerID = &tenant.UserID
	creation.Quote.CurrentStatus = domain.QuoteStatusDraft

	err = q.QueryRow(ctx,
		`INSERT INTO quote_version (account_id, quote_id, author_id, version_number, is_immutable)
		 VALUES ($1, $2, $3, 1, FALSE)
		 RETURNING id, created_at`,
		tenant.AccountID, creation.Quote.ID, tenant.UserID,
	).Scan(&creation.Version.ID, &creation.Version.CreatedAt)
	if err != nil {
		return nil, err
	}
	creation.Version.QuoteID = creation.Quote.ID
	creation.Version.AuthorID = &tenant.UserID
	creation.Version.VersionNumber = 1
	creation.Version.Total = decimal.Zero

	if _, err := q.Exec(ctx,
		`UPDATE quote SET current_version_id = $1
		 WHERE id = $2 AND account_id = $3`,
		creation.Version.ID, creation.Quote.ID, tenant.AccountID,
	); err != nil {
		return nil, err
	}
	creation.Quote.CurrentVersionID = &creation.Version.ID

	if err := insertManualItems(ctx, q, tenant.AccountID, creation.Version.ID, in.Items); err != nil {
		return nil, err
	}

	if _, err := q.Exec(ctx,
		`INSERT INTO rfq_status_change (account_id, rfq_id, previous_status, new_status, user_id)
		 VALUES ($1, $2, 'RECEIVED', 'GENERATED', $3)`,
		tenant.AccountID, creation.Rfq.ID, tenant.UserID,
	); err != nil {
		return nil, err
	}

	if _, err := q.Exec(ctx,
		`INSERT INTO quote_status_change (account_id, quote_id, previous_status, new_status, user_id)
		 VALUES ($1, $2, NULL, 'DRAFT', $3)`,
		tenant.AccountID, creation.Quote.ID, tenant.UserID,
	); err != nil {
		return nil, err
	}

	return &creation, nil
}

func insertManualItems(
	ctx context.Context, q Querier, accountID, versionID uuid.UUID, items []domain.NewRfqItem,
) error {
	products := make([]string, len(items))
	descriptions := make([]string, len(items))
	quantities := make([]string, len(items))
	units := make([]string, len(items))
	for i, it := range items {
		if it.ProductID != nil {
			products[i] = it.ProductID.String()
		}
		descriptions[i] = it.RequestedDescription
		quantities[i] = it.Quantity.String()
		if it.Unit != nil {
			units[i] = *it.Unit
		}
	}

	_, err := q.Exec(ctx,
		`INSERT INTO quote_item
		   (account_id, version_id, product_id, requested_description, quantity, unit, match_status)
		 SELECT $1, $2,
		        NULLIF(p, '')::uuid,
		        d, qty::numeric, NULLIF(u, ''),
		        CASE WHEN p <> '' THEN 'MATCHED'::item_match_status ELSE 'NO_MATCH'::item_match_status END
		 FROM unnest($3::text[], $4::text[], $5::text[], $6::text[]) AS t(p, d, qty, u)`,
		accountID, versionID, products, descriptions, quantities, units)
	return err
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
