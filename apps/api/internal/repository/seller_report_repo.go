package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const sellerReportTopLimit = 5

const sellerReportOwnedQuotes = `WITH owned_quotes AS (
	SELECT q.id AS quote_id, q.current_status, q.current_version_id, q.client_id,
	       r.id AS rfq_id, r.client_label, r.received_at
	FROM quote q
	JOIN rfq r ON r.account_id = q.account_id AND r.id = q.rfq_id
	WHERE q.account_id = $1
	  AND q.seller_id = $2
	  AND ($3::uuid[] IS NULL OR (
	      q.branch_id = ANY($3::uuid[]) AND r.branch_id = ANY($3::uuid[])
	  ))
	  AND ($4::date IS NULL OR r.received_at >= $4::date)
	  AND ($5::date IS NULL OR r.received_at < $5::date + INTERVAL '1 day')
) `

// SellerReportRepository owns persistence for individual seller reports.
type SellerReportRepository struct{}

// NewSellerReportRepository builds a SellerReportRepository.
func NewSellerReportRepository() *SellerReportRepository {
	return &SellerReportRepository{}
}

// Get returns seller metrics and rankings scoped to the account, seller, and reachable branches.
func (r *SellerReportRepository) Get(
	ctx context.Context,
	q Querier,
	accountID, sellerID uuid.UUID,
	branchIDs []uuid.UUID,
	filter domain.SellerReportFilter,
) (*domain.SellerReport, error) {
	report := &domain.SellerReport{
		Statuses:     make([]domain.SellerReportStatus, 0),
		TopMaterials: make([]domain.SellerReportMaterial, 0),
		TopClients:   make([]domain.SellerReportClient, 0),
	}
	args := []any{accountID, sellerID, branchIDs, filter.DateFrom, filter.DateTo}
	if err := r.readSummary(ctx, q, report, args); err != nil {
		return nil, err
	}
	if err := r.readStatuses(ctx, q, report, args); err != nil {
		return nil, err
	}
	if err := r.readTopMaterials(ctx, q, report, args); err != nil {
		return nil, err
	}
	if err := r.readTopClients(ctx, q, report, args); err != nil {
		return nil, err
	}
	return report, nil
}

func (r *SellerReportRepository) readSummary(
	ctx context.Context, q Querier, report *domain.SellerReport, args []any,
) error {
	var average pgtype.Int8
	err := q.QueryRow(ctx, sellerReportOwnedQuotes+`
		SELECT COUNT(*),
		       COUNT(*) FILTER (WHERE EXISTS (
		           SELECT 1 FROM quote_status_change sent
		           WHERE sent.account_id = $1 AND sent.quote_id = owned_quotes.quote_id
		             AND sent.new_status = 'SENT'
		       )),
		       COUNT(*) FILTER (WHERE current_status = 'ACCEPTED'),
		       ROUND(AVG(EXTRACT(EPOCH FROM first_sent.sent_at - received_at))
		           FILTER (WHERE first_sent.sent_at >= received_at))::bigint
		FROM owned_quotes
		LEFT JOIN LATERAL (
			SELECT MIN(changed_at) AS sent_at
			FROM quote_status_change
			WHERE account_id = $1 AND quote_id = owned_quotes.quote_id
			  AND new_status = 'SENT'
		) first_sent ON TRUE`,
		args...,
	).Scan(&report.OrdersReceived, &report.QuotesSent, &report.QuotesAccepted, &average)
	if err != nil {
		return err
	}
	if average.Valid {
		report.AverageQuoteTimeSeconds = &average.Int64
	}
	return nil
}

func (r *SellerReportRepository) readStatuses(
	ctx context.Context, q Querier, report *domain.SellerReport, args []any,
) error {
	rows, err := q.Query(ctx, sellerReportOwnedQuotes+`
		SELECT current_status::text, COUNT(*)
		FROM owned_quotes
		GROUP BY current_status
		ORDER BY current_status`,
		args...,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var item domain.SellerReportStatus
		if err := rows.Scan(&item.Status, &item.Count); err != nil {
			return err
		}
		report.Statuses = append(report.Statuses, item)
	}
	return rows.Err()
}

func (r *SellerReportRepository) readTopMaterials(
	ctx context.Context, q Querier, report *domain.SellerReport, args []any,
) error {
	rows, err := q.Query(ctx, sellerReportOwnedQuotes+`
		SELECT product.canonical_name, COUNT(DISTINCT owned_quotes.quote_id)
		FROM owned_quotes
		JOIN quote_item item
		  ON item.account_id = $1 AND item.version_id = owned_quotes.current_version_id
		JOIN product
		  ON product.account_id = $1 AND product.id = item.product_id
		GROUP BY product.id, product.canonical_name
		ORDER BY COUNT(DISTINCT owned_quotes.quote_id) DESC, product.canonical_name
		LIMIT $6`,
		append(args, sellerReportTopLimit)...,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var item domain.SellerReportMaterial
		if err := rows.Scan(&item.Name, &item.OrderCount); err != nil {
			return err
		}
		report.TopMaterials = append(report.TopMaterials, item)
	}
	return rows.Err()
}

func (r *SellerReportRepository) readTopClients(
	ctx context.Context, q Querier, report *domain.SellerReport, args []any,
) error {
	rows, err := q.Query(ctx, sellerReportOwnedQuotes+`
		SELECT COALESCE(NULLIF(client.name, ''), NULLIF(owned_quotes.client_label, ''), 'Sin identificar'),
		       COUNT(*)
		FROM owned_quotes
		LEFT JOIN client
		  ON client.account_id = $1 AND client.id = owned_quotes.client_id
		GROUP BY COALESCE(NULLIF(client.name, ''), NULLIF(owned_quotes.client_label, ''), 'Sin identificar')
		ORDER BY COUNT(*) DESC, 1
		LIMIT $6`,
		append(args, sellerReportTopLimit)...,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var item domain.SellerReportClient
		if err := rows.Scan(&item.Name, &item.OrderCount); err != nil {
			return err
		}
		report.TopClients = append(report.TopClients, item)
	}
	return rows.Err()
}
