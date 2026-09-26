package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const clientColumns = `id, account_id, name, phone, email, origin_channel, notes,
	created_at, updated_at`

// ClientRepository owns account-scoped client profiles and their accepted-sale projection.
type ClientRepository struct{}

// NewClientRepository builds a ClientRepository.
func NewClientRepository() *ClientRepository { return &ClientRepository{} }

// ListSummaries returns every account client with accepted-sale activity visible to the caller.
func (r *ClientRepository) ListSummaries(
	ctx context.Context, q Querier, tenant domain.Tenant,
) ([]domain.ClientSummary, error) {
	rows, err := q.Query(ctx, `SELECT client.id, client.account_id, client.name, client.phone,
		client.email, client.origin_channel, client.notes, client.created_at, client.updated_at,
		count(quote.id)::integer,
		max(COALESCE(accepted.accepted_at, quote.updated_at))
		FROM client
		LEFT JOIN quote ON quote.account_id = client.account_id
		  AND quote.client_id = client.id
		  AND quote.current_status = 'ACCEPTED'
		  AND ($2::uuid[] IS NULL OR quote.branch_id = ANY($2::uuid[]))
		  AND ($3::uuid IS NULL OR quote.seller_id = $3 OR quote.seller_id IS NULL)
		LEFT JOIN LATERAL (
		  SELECT max(change.changed_at) AS accepted_at
		  FROM quote_status_change change
		  WHERE change.account_id = quote.account_id AND change.quote_id = quote.id
		    AND change.new_status = 'ACCEPTED'
		) accepted ON quote.id IS NOT NULL
		WHERE client.account_id = $1
		GROUP BY client.id
		ORDER BY max(COALESCE(accepted.accepted_at, quote.updated_at)) DESC NULLS LAST,
		  lower(COALESCE(client.name, client.email, client.phone, '')), client.id`,
		tenant.AccountID, tenant.BranchFilter(), sellerParam(tenant))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	summaries := make([]domain.ClientSummary, 0)
	for rows.Next() {
		var summary domain.ClientSummary
		var origin *string
		if err := rows.Scan(&summary.ID, &summary.AccountID, &summary.Name, &summary.Phone,
			&summary.Email, &origin, &summary.Notes, &summary.CreatedAt, &summary.UpdatedAt,
			&summary.AcceptedQuoteCount, &summary.LastAcceptedAt); err != nil {
			return nil, err
		}
		setClientOrigin(&summary.Client, origin)
		summaries = append(summaries, summary)
	}
	return summaries, rows.Err()
}

// GetByID loads one account-scoped client.
func (r *ClientRepository) GetByID(ctx context.Context, q Querier, accountID,
	clientID uuid.UUID) (*domain.Client, error) {
	return scanClient(q.QueryRow(ctx, `SELECT `+clientColumns+`
		FROM client WHERE account_id = $1 AND id = $2`, accountID, clientID))
}

// FindByContacts returns exact phone or email matches for seller confirmation.
func (r *ClientRepository) FindByContacts(
	ctx context.Context, q Querier, accountID uuid.UUID, phone, email *string,
) ([]domain.Client, error) {
	rows, err := q.Query(ctx, `SELECT `+clientColumns+`
		FROM client
		WHERE account_id = $1 AND (
		  ($2::text IS NOT NULL AND regexp_replace(COALESCE(phone, ''), '[^0-9+]', '', 'g') = $2)
		  OR ($3::text IS NOT NULL AND lower(btrim(COALESCE(email, ''))) = $3)
		)
		ORDER BY updated_at DESC, id
		LIMIT 10`, accountID, phone, email)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	clients := make([]domain.Client, 0)
	for rows.Next() {
		client, scanErr := scanClient(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		clients = append(clients, *client)
	}
	return clients, rows.Err()
}

// ListAcceptedSales returns a client's accepted quotes inside the caller's branch and seller reach.
func (r *ClientRepository) ListAcceptedSales(
	ctx context.Context, q Querier, tenant domain.Tenant, clientID uuid.UUID,
) ([]domain.ClientSale, error) {
	rows, err := q.Query(ctx, `SELECT quote.id, quote.rfq_id, quote.number, quote.branch_id,
		branch.name, version.total,
		COALESCE(accepted.accepted_at, quote.updated_at)
		FROM quote
		JOIN branch ON branch.account_id = quote.account_id AND branch.id = quote.branch_id
		JOIN quote_version version ON version.account_id = quote.account_id
		  AND version.id = quote.current_version_id
		LEFT JOIN LATERAL (
		  SELECT max(change.changed_at) AS accepted_at
		  FROM quote_status_change change
		  WHERE change.account_id = quote.account_id AND change.quote_id = quote.id
		    AND change.new_status = 'ACCEPTED'
		) accepted ON TRUE
		WHERE quote.account_id = $1 AND quote.client_id = $2
		  AND quote.current_status = 'ACCEPTED'
		  AND ($3::uuid[] IS NULL OR quote.branch_id = ANY($3::uuid[]))
		  AND ($4::uuid IS NULL OR quote.seller_id = $4 OR quote.seller_id IS NULL)
		ORDER BY COALESCE(accepted.accepted_at, quote.updated_at) DESC, quote.id`,
		tenant.AccountID, clientID, tenant.BranchFilter(), sellerParam(tenant))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	sales := make([]domain.ClientSale, 0)
	for rows.Next() {
		var sale domain.ClientSale
		var total decimal.Decimal
		if err := rows.Scan(&sale.QuoteID, &sale.RFQID, &sale.Number, &sale.BranchID,
			&sale.BranchName, &total, &sale.AcceptedAt); err != nil {
			return nil, err
		}
		sale.Total = total
		sales = append(sales, sale)
	}
	return sales, rows.Err()
}

// Create adds a client only after a seller confirms the profile association.
func (r *ClientRepository) Create(ctx context.Context, q Querier, accountID uuid.UUID,
	in domain.NewClient) (*domain.Client, error) {
	return scanClient(q.QueryRow(ctx, `INSERT INTO client
		(account_id, name, phone, email, origin_channel)
		VALUES ($1, $2, $3, $4, $5) RETURNING `+clientColumns,
		accountID, in.Name, in.Phone, in.Email, in.OriginChannel))
}

func scanClient(row pgx.Row) (*domain.Client, error) {
	var client domain.Client
	var origin *string
	err := row.Scan(&client.ID, &client.AccountID, &client.Name, &client.Phone, &client.Email,
		&origin, &client.Notes, &client.CreatedAt, &client.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	setClientOrigin(&client, origin)
	return &client, nil
}

func setClientOrigin(client *domain.Client, origin *string) {
	if origin == nil {
		return
	}
	value := domain.ClientOrigin(*origin)
	client.OriginChannel = &value
}
