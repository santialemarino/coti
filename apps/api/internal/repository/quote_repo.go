package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const quoteColumns = `id, account_id, branch_id, client_id, rfq_id, seller_id,
	current_version_id, current_status, expires_at, archived_at, needs_followup,
	followup_flagged_at, created_at, updated_at`

const quoteVersionColumns = `id, account_id, quote_id, author_id, version_number, total,
	is_immutable, comment, created_at`

const quoteItemColumns = `id, account_id, version_id, product_id, requested_description,
	quantity, unit, unit_price_snapshot, min_price_snapshot, subtotal, confidence_score,
	match_status, quantity_rationale, created_at`

const quoteStatusChangeColumns = `id, account_id, quote_id, previous_status, new_status,
	user_id, changed_at, created_at`

const quoteRFQIndex = "uq_quote_rfq"
const quoteVersionIndex = "uq_quote_version"
const quoteVersionDraftIndex = "uq_quote_version_draft"

// QuoteRepository owns persistence for quotes, versions, items, and status history.
type QuoteRepository struct{}

// NewQuoteRepository builds a QuoteRepository.
func NewQuoteRepository() *QuoteRepository {
	return &QuoteRepository{}
}

// Create inserts a quote shell.
func (r *QuoteRepository) Create(
	ctx context.Context, q Querier, accountID uuid.UUID, in domain.NewQuote,
) (*domain.Quote, error) {
	quote, err := scanQuote(q.QueryRow(ctx,
		`INSERT INTO quote (account_id, branch_id, client_id, rfq_id, seller_id,
		                    current_status, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING `+quoteColumns,
		accountID, in.BranchID, in.ClientID, in.RFQID, in.SellerID, in.CurrentStatus, in.ExpiresAt))
	if isUniqueViolation(err, quoteRFQIndex) {
		return nil, domain.ErrConflict
	}
	return quote, err
}

// UpdateCurrentVersion writes the quote's current version pointer.
func (r *QuoteRepository) UpdateCurrentVersion(
	ctx context.Context, q Querier, accountID, quoteID, versionID uuid.UUID,
) (*domain.Quote, error) {
	return scanQuote(q.QueryRow(ctx,
		`UPDATE quote
		 SET current_version_id = $3
		 WHERE account_id = $1 AND id = $2
		 RETURNING `+quoteColumns,
		accountID, quoteID, versionID))
}

// CreateVersion inserts a quote version.
func (r *QuoteRepository) CreateVersion(
	ctx context.Context, q Querier, accountID uuid.UUID, in domain.NewQuoteVersion,
) (*domain.QuoteVersion, error) {
	version, err := scanQuoteVersion(q.QueryRow(ctx,
		`INSERT INTO quote_version (account_id, quote_id, author_id, version_number, total,
		                            is_immutable, comment)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 RETURNING `+quoteVersionColumns,
		accountID, in.QuoteID, in.AuthorID, in.VersionNumber, in.Total, in.IsImmutable,
		in.Comment))
	if isUniqueViolation(err, quoteVersionIndex) || isUniqueViolation(err, quoteVersionDraftIndex) {
		return nil, domain.ErrConflict
	}
	return version, err
}

// CreateItems inserts a quote version's line items in one statement.
func (r *QuoteRepository) CreateItems(
	ctx context.Context, q Querier, accountID, versionID uuid.UUID, items []domain.NewQuoteItem,
) ([]domain.QuoteItem, error) {
	payload, err := json.Marshal(quoteItemPayloads(items))
	if err != nil {
		return nil, err
	}

	rows, err := q.Query(ctx,
		`WITH incoming AS (
		   SELECT *
		   FROM jsonb_to_recordset($3::jsonb) AS x(
		     product_id uuid,
		     requested_description text,
		     quantity numeric,
		     unit text,
		     unit_price_snapshot numeric,
		     min_price_snapshot numeric,
		     subtotal numeric,
		     confidence_score numeric,
		     match_status text,
		     quantity_rationale text
		   )
		 )
		 INSERT INTO quote_item (
		   account_id, version_id, product_id, requested_description, quantity, unit,
		   unit_price_snapshot, min_price_snapshot, subtotal, confidence_score,
		   match_status, quantity_rationale
		 )
		 SELECT $1, $2, product_id, requested_description, quantity, unit, unit_price_snapshot,
		        min_price_snapshot, subtotal, confidence_score, match_status::item_match_status,
		        quantity_rationale
		 FROM incoming
		 RETURNING `+quoteItemColumns,
		accountID, versionID, payload)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	created := make([]domain.QuoteItem, 0, len(items))
	for rows.Next() {
		item, scanErr := scanQuoteItemRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		created = append(created, *item)
	}
	return created, rows.Err()
}

// AppendStatusChange records a quote lifecycle transition.
func (r *QuoteRepository) AppendStatusChange(
	ctx context.Context, q Querier, accountID, quoteID uuid.UUID, previousStatus *domain.QuoteStatus,
	newStatus domain.QuoteStatus, userID *uuid.UUID,
) (*domain.QuoteStatusChange, error) {
	return scanQuoteStatusChange(q.QueryRow(ctx,
		`INSERT INTO quote_status_change (account_id, quote_id, previous_status, new_status, user_id)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+quoteStatusChangeColumns,
		accountID, quoteID, previousStatus, newStatus, userID))
}

type quoteItemPayload struct {
	ProductID            *uuid.UUID `json:"product_id"`
	RequestedDescription string     `json:"requested_description"`
	Quantity             string     `json:"quantity"`
	Unit                 *string    `json:"unit"`
	UnitPriceSnapshot    *string    `json:"unit_price_snapshot"`
	MinPriceSnapshot     *string    `json:"min_price_snapshot"`
	Subtotal             *string    `json:"subtotal"`
	ConfidenceScore      *string    `json:"confidence_score"`
	MatchStatus          string     `json:"match_status"`
	QuantityRationale    *string    `json:"quantity_rationale"`
}

func quoteItemPayloads(items []domain.NewQuoteItem) []quoteItemPayload {
	payloads := make([]quoteItemPayload, 0, len(items))
	for _, item := range items {
		payloads = append(payloads, quoteItemPayload{
			ProductID:            item.ProductID,
			RequestedDescription: item.RequestedDescription,
			Quantity:             item.Quantity.String(),
			Unit:                 item.Unit,
			UnitPriceSnapshot:    nullDecimalString(item.UnitPriceSnapshot),
			MinPriceSnapshot:     nullDecimalString(item.MinPriceSnapshot),
			Subtotal:             nullDecimalString(item.Subtotal),
			ConfidenceScore:      nullDecimalString(item.ConfidenceScore),
			MatchStatus:          string(item.MatchStatus),
			QuantityRationale:    item.QuantityRationale,
		})
	}
	return payloads
}

func nullDecimalString(amount decimal.NullDecimal) *string {
	if !amount.Valid {
		return nil
	}
	value := amount.Decimal.String()
	return &value
}

func scanQuote(row pgx.Row) (*domain.Quote, error) {
	var quote domain.Quote
	err := row.Scan(&quote.ID, &quote.AccountID, &quote.BranchID, &quote.ClientID, &quote.RFQID,
		&quote.SellerID, &quote.CurrentVersionID, &quote.CurrentStatus, &quote.ExpiresAt,
		&quote.ArchivedAt, &quote.NeedsFollowup, &quote.FollowupFlaggedAt, &quote.CreatedAt,
		&quote.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &quote, nil
}

func scanQuoteVersion(row pgx.Row) (*domain.QuoteVersion, error) {
	var version domain.QuoteVersion
	err := row.Scan(&version.ID, &version.AccountID, &version.QuoteID, &version.AuthorID,
		&version.VersionNumber, &version.Total, &version.IsImmutable, &version.Comment,
		&version.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &version, nil
}

func scanQuoteItemRow(row pgx.Row) (*domain.QuoteItem, error) {
	var item domain.QuoteItem
	err := row.Scan(&item.ID, &item.AccountID, &item.VersionID, &item.ProductID,
		&item.RequestedDescription, &item.Quantity, &item.Unit, &item.UnitPriceSnapshot,
		&item.MinPriceSnapshot, &item.Subtotal, &item.ConfidenceScore, &item.MatchStatus,
		&item.QuantityRationale, &item.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func scanQuoteStatusChange(row pgx.Row) (*domain.QuoteStatusChange, error) {
	var change domain.QuoteStatusChange
	err := row.Scan(&change.ID, &change.AccountID, &change.QuoteID, &change.PreviousStatus,
		&change.NewStatus, &change.UserID, &change.ChangedAt, &change.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &change, nil
}
