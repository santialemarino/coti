package repository

import (
	"context"
	"sort"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

var reportStatusOrder = map[domain.QuoteStatus]int{
	domain.QuoteStatusDraft:           0,
	domain.QuoteStatusQuoted:          1,
	domain.QuoteStatusSent:            2,
	domain.QuoteStatusChangeRequested: 3,
	domain.QuoteStatusAccepted:        4,
	domain.QuoteStatusRejected:        5,
}

// scopedRfqsCTE narrows RFQ-level metrics to the caller's reach and the report filters.
const scopedRfqsCTE = `scoped_rfqs AS (
		   SELECT r.id, r.received_at, c.type::text AS channel_type
		   FROM rfq r
		   JOIN channel c ON c.account_id = r.account_id AND c.id = r.channel_id
		   LEFT JOIN quote q ON q.account_id = r.account_id AND q.rfq_id = r.id
		   WHERE r.account_id = $1
		     AND ($2::uuid[] IS NULL OR r.branch_id = ANY($2::uuid[]))
		     AND ($3::uuid IS NULL OR q.seller_id = $3 OR q.seller_id IS NULL)
		     AND ($6::text[] IS NULL OR q.current_status::text = ANY($6::text[]))
		     AND ($7::text IS NULL OR c.type::text = $7)
		     AND ($8::uuid IS NULL OR r.client_id = $8)
		     AND ($9::uuid IS NULL OR q.seller_id = $9)
		     AND ($10::uuid[] IS NULL OR EXISTS (
		       SELECT 1 FROM client_tag ct
		       WHERE ct.account_id = r.account_id AND ct.client_id = r.client_id
		         AND ct.tag_id = ANY($10::uuid[])))
		 )`

// acceptedVersionsCTE resolves which version each acceptance in the period charged.
const acceptedVersionsCTE = `WITH ` + scopedRfqsCTE + `, accepted AS (
		   SELECT change.quote_id, max(change.changed_at) AS accepted_at
		   FROM quote_status_change change
		   JOIN quote q ON q.account_id = change.account_id AND q.id = change.quote_id
		   JOIN scoped_rfqs scoped ON scoped.id = q.rfq_id
		   WHERE change.account_id = $1 AND change.new_status = 'ACCEPTED'
		     AND change.changed_at >= $4 AND change.changed_at < $5
		   GROUP BY change.quote_id
		 ), resolved AS (
		   SELECT accepted.quote_id,
		          COALESCE(
		            (SELECT action.version_id
		             FROM client_action action
		             WHERE action.account_id = $1 AND action.type = 'ACCEPT'
		               AND action.version_id IN (SELECT version.id FROM quote_version version
		                                         WHERE version.account_id = $1
		                                           AND version.quote_id = accepted.quote_id)
		               AND action.created_at >= accepted.accepted_at - interval '5 minutes'
		               AND action.created_at <= accepted.accepted_at + interval '5 minutes'
		             ORDER BY action.created_at DESC, action.id DESC LIMIT 1),
		            (SELECT send.version_id
		             FROM quote_send send
		             WHERE send.account_id = $1 AND send.sent_at <= accepted.accepted_at
		               AND send.tracking_status IN ('SENT', 'DELIVERED', 'VIEWED')
		               AND send.version_id IN (SELECT version.id FROM quote_version version
		                                       WHERE version.account_id = $1
		                                         AND version.quote_id = accepted.quote_id)
		             ORDER BY send.sent_at DESC, send.created_at DESC LIMIT 1),
		            CASE WHEN NOT EXISTS (
		              SELECT 1 FROM quote_version later
		              WHERE later.account_id = $1 AND later.quote_id = accepted.quote_id
		                AND later.created_at > accepted.accepted_at
		            ) THEN quote.current_version_id END
		          ) AS version_id
		   FROM accepted
		   JOIN quote ON quote.account_id = $1 AND quote.id = accepted.quote_id
		 )`

// SellerReportRepository owns the aggregate reads behind the seller dashboard.
type SellerReportRepository struct{}

// NewSellerReportRepository builds a SellerReportRepository.
func NewSellerReportRepository() *SellerReportRepository {
	return &SellerReportRepository{}
}

// Build reads every section of one report inside the caller's tenant transaction.
func (r *SellerReportRepository) Build(
	ctx context.Context, q Querier, in domain.SellerReportQuery,
) (*domain.SellerReport, error) {
	report := &domain.SellerReport{
		Branches:           []domain.SellerReportBranch{},
		QuotesByStatus:     []domain.SellerReportStatusCount{},
		Timeline:           []domain.SellerReportTimelinePoint{},
		TopProducts:        []domain.SellerReportProduct{},
		TopRevenueProducts: []domain.SellerReportRevenueProduct{},
		TopClients:         []domain.SellerReportClient{},
		Filters: domain.SellerReportFilters{
			Statuses: in.Statuses,
			Channel:  in.Channel,
			SellerID: in.SellerFilter,
			ClientID: in.ClientFilter,
			Branches: []domain.SellerReportFilterOption{},
			Tags:     []domain.SellerReportFilterOption{},
		},
	}

	if in.SellerFilter != nil || in.ClientFilter != nil {
		if err := r.readFilterNames(ctx, q, in, &report.Filters); err != nil {
			return nil, err
		}
	}
	if err := r.readFilterOptions(ctx, q, `SELECT id, name FROM branch
		 WHERE account_id = $1 AND id = ANY($2::uuid[]) ORDER BY name, id`,
		in.AccountID, in.RequestedBranches, &report.Filters.Branches); err != nil {
		return nil, err
	}
	if err := r.readFilterOptions(ctx, q, `SELECT id, name FROM tag
		 WHERE account_id = $1 AND id = ANY($2::uuid[]) ORDER BY name, id`,
		in.AccountID, in.TagFilter, &report.Filters.Tags); err != nil {
		return nil, err
	}
	if err := r.readKPIs(ctx, q, in, &report.KPIs); err != nil {
		return nil, err
	}
	if err := r.readAccepted(ctx, q, in, &report.KPIs); err != nil {
		return nil, err
	}
	if report.KPIs.QuotesSent > 0 {
		rate := decimal.NewFromInt(int64(report.KPIs.SentAccepted)).
			Div(decimal.NewFromInt(int64(report.KPIs.QuotesSent)))
		report.KPIs.ConversionRate = &rate
	}
	if err := r.readResponseTime(ctx, q, in, &report.KPIs.ResponseTime); err != nil {
		return nil, err
	}
	if err := r.readQuotesByStatus(ctx, q, in, &report.QuotesByStatus); err != nil {
		return nil, err
	}
	if err := r.readTimeline(ctx, q, in, &report.Timeline); err != nil {
		return nil, err
	}
	if err := r.readTopProducts(ctx, q, in, &report.TopProducts); err != nil {
		return nil, err
	}
	if err := r.readTopRevenueProducts(ctx, q, in, &report.TopRevenueProducts); err != nil {
		return nil, err
	}
	if err := r.readTopClients(ctx, q, in, &report.TopClients); err != nil {
		return nil, err
	}
	if err := r.readAIMatch(ctx, q, in, &report.AIMatch); err != nil {
		return nil, err
	}
	if in.MultiBranch {
		if err := r.readBranches(ctx, q, in, &report.Branches); err != nil {
			return nil, err
		}
	}
	return report, nil
}

func (r *SellerReportRepository) readFilterNames(
	ctx context.Context, q Querier, in domain.SellerReportQuery, out *domain.SellerReportFilters,
) error {
	return q.QueryRow(ctx,
		`SELECT COALESCE((SELECT name FROM app_user
		                  WHERE account_id = $1 AND id = $2), ''),
		        COALESCE((SELECT name FROM client
		                  WHERE account_id = $1 AND id = $3), '')`,
		in.AccountID, in.SellerFilter, in.ClientFilter,
	).Scan(&out.SellerName, &out.ClientName)
}

// readFilterOptions echoes the selected branches or tags with their names; nothing selected reads nothing.
func (r *SellerReportRepository) readFilterOptions(
	ctx context.Context, q Querier, query string, accountID uuid.UUID, ids []uuid.UUID,
	out *[]domain.SellerReportFilterOption,
) error {
	if len(ids) == 0 {
		return nil
	}
	rows, err := q.Query(ctx, query, accountID, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var option domain.SellerReportFilterOption
		if err := rows.Scan(&option.ID, &option.Name); err != nil {
			return err
		}
		*out = append(*out, option)
	}
	return rows.Err()
}

func reportArgs(in domain.SellerReportQuery) []any {
	var statuses []string
	if len(in.Statuses) > 0 {
		statuses = make([]string, 0, len(in.Statuses))
		for _, status := range in.Statuses {
			statuses = append(statuses, string(status))
		}
	}
	var channel *string
	if in.Channel != nil {
		value := string(*in.Channel)
		channel = &value
	}
	return []any{
		in.AccountID, in.BranchFilter, in.SellerID, in.StartAt, in.EndAt,
		statuses, channel, in.ClientFilter, in.SellerFilter, in.TagFilter,
	}
}

func (r *SellerReportRepository) readKPIs(
	ctx context.Context, q Querier, in domain.SellerReportQuery, out *domain.SellerReportKPIs,
) error {
	return q.QueryRow(ctx,
		`WITH `+scopedRfqsCTE+`, first_sent AS (
		   SELECT change.quote_id, min(change.changed_at) AS sent_at
		   FROM quote_status_change change
		   JOIN quote q ON q.account_id = change.account_id AND q.id = change.quote_id
		   JOIN scoped_rfqs scoped ON scoped.id = q.rfq_id
		   WHERE change.account_id = $1 AND change.new_status = 'SENT'
		   GROUP BY change.quote_id
		 )
		 SELECT (SELECT count(*) FROM scoped_rfqs
		         WHERE received_at >= $4 AND received_at < $5),
		        (SELECT count(*) FROM first_sent WHERE sent_at >= $4 AND sent_at < $5),
		        (SELECT count(*) FROM first_sent
		         WHERE sent_at >= $4 AND sent_at < $5
		           AND EXISTS (SELECT 1 FROM quote_status_change accepted
		                       WHERE accepted.account_id = $1
		                         AND accepted.quote_id = first_sent.quote_id
		                         AND accepted.new_status = 'ACCEPTED'
		                         AND accepted.changed_at >= first_sent.sent_at))`,
		reportArgs(in)...,
	).Scan(&out.RFQsReceived, &out.QuotesSent, &out.SentAccepted)
}

func (r *SellerReportRepository) readAccepted(
	ctx context.Context, q Querier, in domain.SellerReportQuery, out *domain.SellerReportKPIs,
) error {
	return q.QueryRow(ctx, acceptedVersionsCTE+`
		 SELECT count(*), COALESCE(sum(COALESCE(version.total, 0)), 0)
		 FROM resolved
		 LEFT JOIN quote_version version ON version.account_id = $1 AND version.id = resolved.version_id`,
		reportArgs(in)...,
	).Scan(&out.QuotesAccepted, &out.AcceptedAmount)
}

func (r *SellerReportRepository) readResponseTime(
	ctx context.Context, q Querier, in domain.SellerReportQuery, out *domain.SellerReportResponseTime,
) error {
	var average, median int64
	err := q.QueryRow(ctx,
		`WITH `+scopedRfqsCTE+`, sent AS (
		   SELECT scoped.id, scoped.received_at, min(change.changed_at) AS sent_at
		   FROM scoped_rfqs scoped
		   JOIN quote q ON q.account_id = $1 AND q.rfq_id = scoped.id
		   JOIN quote_status_change change ON change.account_id = $1
		     AND change.quote_id = q.id AND change.new_status = 'SENT'
		   WHERE scoped.received_at >= $4 AND scoped.received_at < $5
		     AND scoped.channel_type <> 'MANUAL_ENTRY'
		   GROUP BY scoped.id, scoped.received_at
		 )
		 SELECT count(*),
		        COALESCE(avg(extract(epoch FROM (sent.sent_at - sent.received_at))), 0)::bigint,
		        COALESCE(percentile_cont(0.5) WITHIN GROUP (
		          ORDER BY extract(epoch FROM (sent.sent_at - sent.received_at))
		        ), 0)::bigint
		 FROM sent
		 WHERE sent.sent_at >= sent.received_at`,
		reportArgs(in)...,
	).Scan(&out.Sample, &average, &median)
	if err != nil {
		return err
	}
	if out.Sample > 0 {
		out.AverageSeconds = &average
		out.MedianSeconds = &median
	}
	return nil
}

func (r *SellerReportRepository) readQuotesByStatus(
	ctx context.Context, q Querier, in domain.SellerReportQuery,
	out *[]domain.SellerReportStatusCount,
) error {
	rows, err := q.Query(ctx,
		`WITH `+scopedRfqsCTE+`
		 SELECT quote.current_status::text, count(*)
		 FROM quote
		 JOIN scoped_rfqs scoped ON scoped.id = quote.rfq_id
		 WHERE quote.account_id = $1
		   AND quote.created_at >= $4 AND quote.created_at < $5
		 GROUP BY quote.current_status`,
		reportArgs(in)...,
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item domain.SellerReportStatusCount
		if err := rows.Scan(&item.Status, &item.Count); err != nil {
			return err
		}
		*out = append(*out, item)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	sort.Slice(*out, func(i, j int) bool {
		left, leftOK := reportStatusOrder[(*out)[i].Status]
		right, rightOK := reportStatusOrder[(*out)[j].Status]
		if leftOK != rightOK {
			return leftOK
		}
		return left < right
	})
	return nil
}

func (r *SellerReportRepository) readTimeline(
	ctx context.Context, q Querier, in domain.SellerReportQuery,
	out *[]domain.SellerReportTimelinePoint,
) error {
	// $14 is the bucket unit date_trunc and the series step share; weeks start on Monday.
	rows, err := q.Query(ctx,
		`WITH `+scopedRfqsCTE+`, buckets AS (
		   SELECT generate_series(date_trunc($14, $11::date), $12::date,
		                          ('1 ' || $14)::interval)::date AS bucket
		 ), received AS (
		   SELECT date_trunc($14, received_at AT TIME ZONE $13)::date AS bucket, count(*) AS total
		   FROM scoped_rfqs
		   WHERE received_at >= $4 AND received_at < $5
		   GROUP BY 1
		 ), first_sent AS (
		   SELECT change.quote_id, min(change.changed_at) AS sent_at
		   FROM quote_status_change change
		   JOIN quote q ON q.account_id = change.account_id AND q.id = change.quote_id
		   JOIN scoped_rfqs scoped ON scoped.id = q.rfq_id
		   WHERE change.account_id = $1 AND change.new_status = 'SENT'
		   GROUP BY change.quote_id
		 ), sent AS (
		   SELECT date_trunc($14, sent_at AT TIME ZONE $13)::date AS bucket, count(*) AS total,
		          count(*) FILTER (WHERE EXISTS (
		            SELECT 1 FROM quote_status_change accepted
		            WHERE accepted.account_id = $1 AND accepted.quote_id = first_sent.quote_id
		              AND accepted.new_status = 'ACCEPTED'
		              AND accepted.changed_at >= first_sent.sent_at)) AS converted
		   FROM first_sent
		   WHERE sent_at >= $4 AND sent_at < $5
		   GROUP BY 1
		 ), accepted AS (
		   SELECT DISTINCT ON (change.quote_id) change.quote_id, change.changed_at
		   FROM quote_status_change change
		   JOIN quote q ON q.account_id = change.account_id AND q.id = change.quote_id
		   JOIN scoped_rfqs scoped ON scoped.id = q.rfq_id
		   WHERE change.account_id = $1 AND change.new_status = 'ACCEPTED'
		     AND change.changed_at >= $4 AND change.changed_at < $5
		   ORDER BY change.quote_id, change.changed_at DESC
		 ), accepted_by_bucket AS (
		   SELECT date_trunc($14, changed_at AT TIME ZONE $13)::date AS bucket, count(*) AS total
		   FROM accepted
		   GROUP BY 1
		 )
		 SELECT to_char(GREATEST(buckets.bucket, $11::date), 'YYYY-MM-DD'),
		        COALESCE(received.total, 0), COALESCE(sent.total, 0),
		        COALESCE(accepted_by_bucket.total, 0), COALESCE(sent.converted, 0)
		 FROM buckets
		 LEFT JOIN received ON received.bucket = buckets.bucket
		 LEFT JOIN sent ON sent.bucket = buckets.bucket
		 LEFT JOIN accepted_by_bucket ON accepted_by_bucket.bucket = buckets.bucket
		 ORDER BY buckets.bucket`,
		append(reportArgs(in), in.FromDate, in.ToDate, in.TimeZone, string(in.Granularity))...,
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item domain.SellerReportTimelinePoint
		if err := rows.Scan(&item.Date, &item.RFQs, &item.Sent, &item.Accepted,
			&item.SentAccepted); err != nil {
			return err
		}
		if item.Sent > 0 {
			rate := decimal.NewFromInt(int64(item.SentAccepted)).Div(decimal.NewFromInt(int64(item.Sent)))
			item.ConversionRate = &rate
		}
		*out = append(*out, item)
	}
	return rows.Err()
}

func (r *SellerReportRepository) readTopProducts(
	ctx context.Context, q Querier, in domain.SellerReportQuery,
	out *[]domain.SellerReportProduct,
) error {
	rows, err := q.Query(ctx,
		`WITH `+scopedRfqsCTE+`, latest_quote AS (
		   SELECT DISTINCT ON (quote.rfq_id) quote.id, quote.current_version_id
		   FROM quote
		   JOIN scoped_rfqs scoped ON scoped.id = quote.rfq_id
		   WHERE quote.account_id = $1
		     AND scoped.received_at >= $4 AND scoped.received_at < $5
		   ORDER BY quote.rfq_id, quote.created_at DESC
		 ), items AS (
		   SELECT item.product_id, product.canonical_name, item.requested_description
		   FROM latest_quote
		   JOIN quote_item item ON item.account_id = $1
		     AND item.version_id = latest_quote.current_version_id
		   LEFT JOIN product ON product.account_id = $1 AND product.id = item.product_id
		   WHERE latest_quote.current_version_id IS NOT NULL
		 )
		 SELECT product_id,
		        COALESCE(max(canonical_name), min(btrim(requested_description))),
		        count(*)
		 FROM items
		 GROUP BY product_id,
		          CASE WHEN product_id IS NULL THEN lower(btrim(requested_description)) ELSE '' END
		 ORDER BY count(*) DESC, 2, 1
		 LIMIT 5`,
		reportArgs(in)...,
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item domain.SellerReportProduct
		if err := rows.Scan(&item.ProductID, &item.Name, &item.Requests); err != nil {
			return err
		}
		*out = append(*out, item)
	}
	return rows.Err()
}

func (r *SellerReportRepository) readTopRevenueProducts(
	ctx context.Context, q Querier, in domain.SellerReportQuery,
	out *[]domain.SellerReportRevenueProduct,
) error {
	rows, err := q.Query(ctx, acceptedVersionsCTE+`
		 SELECT item.product_id,
		        COALESCE(max(product.canonical_name), min(item.requested_description)),
		        sum(round(item.quantity * item.unit_price_snapshot, 2))
		 FROM resolved
		 JOIN quote_item item ON item.account_id = $1
		   AND item.version_id = resolved.version_id
		 LEFT JOIN product ON product.account_id = $1 AND product.id = item.product_id
		 WHERE item.product_id IS NOT NULL AND item.unit_price_snapshot IS NOT NULL
		 GROUP BY item.product_id
		 HAVING sum(round(item.quantity * item.unit_price_snapshot, 2)) > 0
		 ORDER BY 3 DESC, 2, 1
		 LIMIT 5`,
		reportArgs(in)...,
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item domain.SellerReportRevenueProduct
		if err := rows.Scan(&item.ProductID, &item.Name, &item.Revenue); err != nil {
			return err
		}
		*out = append(*out, item)
	}
	return rows.Err()
}

func (r *SellerReportRepository) readTopClients(
	ctx context.Context, q Querier, in domain.SellerReportQuery,
	out *[]domain.SellerReportClient,
) error {
	rows, err := q.Query(ctx, acceptedVersionsCTE+`
		 SELECT quote.client_id,
		        COALESCE(max(client.name), max(client.email), max(client.phone),
		                 max(rfq.client_label), ''),
		        sum(COALESCE(version.total, 0))
		 FROM resolved
		 JOIN quote ON quote.account_id = $1 AND quote.id = resolved.quote_id
		 JOIN rfq ON rfq.account_id = $1 AND rfq.id = quote.rfq_id
		 LEFT JOIN client ON client.account_id = $1 AND client.id = quote.client_id
		 LEFT JOIN quote_version version ON version.account_id = $1
		   AND version.id = resolved.version_id
		 WHERE quote.client_id IS NOT NULL OR COALESCE(rfq.client_label, '') <> ''
		 GROUP BY quote.client_id, CASE WHEN quote.client_id IS NULL THEN rfq.client_label END
		 HAVING sum(COALESCE(version.total, 0)) > 0
		 ORDER BY sum(COALESCE(version.total, 0)) DESC, 2, 1
		 LIMIT 5`,
		reportArgs(in)...,
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item domain.SellerReportClient
		if err := rows.Scan(&item.ClientID, &item.Name, &item.AcceptedAmount); err != nil {
			return err
		}
		*out = append(*out, item)
	}
	return rows.Err()
}

func (r *SellerReportRepository) readAIMatch(
	ctx context.Context, q Querier, in domain.SellerReportQuery, out *domain.SellerReportAIMatch,
) error {
	var matched, low, medium, high int
	err := q.QueryRow(ctx,
		`WITH `+scopedRfqsCTE+`, latest_generation AS (
		   SELECT DISTINCT ON (generation.quote_id) generation.id
		   FROM quote_ai_generation generation
		   JOIN quote ON quote.account_id = generation.account_id
		     AND quote.id = generation.quote_id
		   JOIN scoped_rfqs scoped ON scoped.id = quote.rfq_id
		   WHERE generation.account_id = $1
		     AND scoped.received_at >= $4 AND scoped.received_at < $5
		   ORDER BY generation.quote_id, generation.created_at DESC
		 )
		 SELECT count(*),
		        count(*) FILTER (WHERE item.match_status = 'MATCHED'),
		        count(*) FILTER (WHERE item.confidence_score IS NOT NULL
		          AND item.match_status = 'NO_MATCH'),
		        count(*) FILTER (WHERE item.confidence_score IS NOT NULL AND (
		          item.match_status = 'AMBIGUOUS'
		          OR (item.match_status = 'MATCHED' AND item.confidence_score < $11))),
		        count(*) FILTER (WHERE item.confidence_score IS NOT NULL
		          AND item.match_status = 'MATCHED' AND item.confidence_score >= $11)
		 FROM latest_generation
		 JOIN quote_ai_generation_item item ON item.account_id = $1
		   AND item.generation_id = latest_generation.id`,
		append(reportArgs(in), in.HighConfidence)...,
	).Scan(&out.Total, &matched, &low, &medium, &high)
	if err != nil {
		return err
	}
	out.Matched = matched
	out.Confidence = domain.SellerReportConfidence{Low: low, Medium: medium, High: high}
	if out.Total > 0 {
		rate := decimal.NewFromInt(int64(matched)).
			Div(decimal.NewFromInt(int64(out.Total)))
		out.Rate = &rate
	}
	return nil
}

func (r *SellerReportRepository) readBranches(
	ctx context.Context, q Querier, in domain.SellerReportQuery, out *[]domain.SellerReportBranch,
) error {
	rows, err := q.Query(ctx,
		`SELECT branch.id, branch.name, count(scoped.id)
		 FROM branch
		 LEFT JOIN LATERAL (
		   SELECT rfq.id
		   FROM rfq
		   JOIN channel c ON c.account_id = rfq.account_id AND c.id = rfq.channel_id
		   LEFT JOIN quote q ON q.account_id = rfq.account_id AND q.rfq_id = rfq.id
		   WHERE rfq.account_id = branch.account_id
		     AND rfq.branch_id = branch.id
		     AND ($2::uuid[] IS NULL OR rfq.branch_id = ANY($2::uuid[]))
		     AND rfq.received_at >= $4 AND rfq.received_at < $5
		     AND ($3::uuid IS NULL OR q.seller_id = $3 OR q.seller_id IS NULL)
		     AND ($6::text[] IS NULL OR q.current_status::text = ANY($6::text[]))
		     AND ($7::text IS NULL OR c.type::text = $7)
		     AND ($8::uuid IS NULL OR rfq.client_id = $8)
		     AND ($9::uuid IS NULL OR q.seller_id = $9)
		     AND ($10::uuid[] IS NULL OR EXISTS (
		       SELECT 1 FROM client_tag ct
		       WHERE ct.account_id = rfq.account_id AND ct.client_id = rfq.client_id
		         AND ct.tag_id = ANY($10::uuid[])))
		 ) scoped ON TRUE
		 WHERE branch.account_id = $1
		   AND ($2::uuid[] IS NULL OR branch.id = ANY($2::uuid[]))
		 GROUP BY branch.id, branch.name
		 HAVING branch.is_active OR count(scoped.id) > 0
		 ORDER BY branch.name, branch.id`,
		reportArgs(in)...,
	)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var item domain.SellerReportBranch
		if err := rows.Scan(&item.BranchID, &item.BranchName, &item.RFQs); err != nil {
			return err
		}
		*out = append(*out, item)
	}
	return rows.Err()
}
