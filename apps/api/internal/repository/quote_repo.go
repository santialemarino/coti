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

// GetByID loads one quote, scoped to the account and to the branch it belongs to. Filtering the
// branch is load-bearing: row level security guards the account boundary only, so a branch taken
// from a request and left out of the predicate would read another branch of the caller's account.
func (r *QuoteRepository) GetByID(
	ctx context.Context, q Querier, accountID, branchID, id uuid.UUID,
) (*domain.Quote, error) {
	return scanQuote(q.QueryRow(ctx,
		`SELECT `+quoteColumns+`
		 FROM quote
		 WHERE account_id = $1 AND branch_id = $2 AND id = $3`,
		accountID, branchID, id))
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

// UpdateStatus moves the quote's derived status, and only from the status the caller read. The
// guard is what makes a transition atomic: two callers who both read DRAFT would otherwise both
// write QUOTED and append a status change each, the second recording a previous status the quote
// had already left. A statement that matches no row is that conflict — inside a transaction that
// has already read the quote, it cannot be an absent one.
func (r *QuoteRepository) UpdateStatus(
	ctx context.Context, q Querier, accountID, branchID, quoteID uuid.UUID,
	from, to domain.QuoteStatus,
) (*domain.Quote, error) {
	quote, err := scanQuote(q.QueryRow(ctx,
		`UPDATE quote
		 SET current_status = $5
		 WHERE account_id = $1 AND branch_id = $2 AND id = $3 AND current_status = $4
		 RETURNING `+quoteColumns,
		accountID, branchID, quoteID, from, to))
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrConflict
	}
	return quote, err
}

// GetCurrentVersion loads the version the quote points at. Returns domain.ErrNotFound when the
// quote is absent, sits in another branch, or points at nothing. The branch is in the predicate
// because quoteID reaches this from a request, and row level security guards only the account.
func (r *QuoteRepository) GetCurrentVersion(
	ctx context.Context, q Querier, accountID, branchID, quoteID uuid.UUID,
) (*domain.QuoteVersion, error) {
	return scanQuoteVersion(q.QueryRow(ctx,
		`SELECT `+quoteVersionColumns+`
		 FROM quote_version
		 WHERE account_id = $1
		   AND id = (SELECT current_version_id FROM quote
		             WHERE account_id = $1 AND branch_id = $2 AND id = $3)`,
		accountID, branchID, quoteID))
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

// UpdateVersionTotal writes the version's total.
func (r *QuoteRepository) UpdateVersionTotal(
	ctx context.Context, q Querier, accountID, versionID uuid.UUID, total decimal.Decimal,
) (*domain.QuoteVersion, error) {
	return scanQuoteVersion(q.QueryRow(ctx,
		`UPDATE quote_version
		 SET total = $3
		 WHERE account_id = $1 AND id = $2
		 RETURNING `+quoteVersionColumns,
		accountID, versionID, total))
}

// ListItems loads a version's lines in the order they were inserted, which is the order the
// client listed the materials in. quote_item carries no ordinal column, and one batch shares a
// single created_at, so this orders lines added later after the original ones and leaves the
// batch itself to the order it was written in.
func (r *QuoteRepository) ListItems(
	ctx context.Context, q Querier, accountID, versionID uuid.UUID,
) ([]domain.QuoteItem, error) {
	rows, err := q.Query(ctx,
		`SELECT `+quoteItemColumns+`
		 FROM quote_item
		 WHERE account_id = $1 AND version_id = $2
		 ORDER BY created_at`,
		accountID, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.QuoteItem
	for rows.Next() {
		item, scanErr := scanQuoteItemRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

// CreateItems inserts a quote version's line items in one statement, in the order given — the
// order the client listed the materials in.
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
		     quantity_rationale text,
		     position int
		   )
		 )
		 INSERT INTO quote_item (
		   account_id, version_id, product_id, requested_description, quantity, unit,
		   unit_price_snapshot, min_price_snapshot, subtotal, confidence_score,
		   match_status, quantity_rationale
		 )
		 SELECT $1, version.id, incoming.product_id, incoming.requested_description,
		        incoming.quantity, incoming.unit, incoming.unit_price_snapshot,
		        incoming.min_price_snapshot, incoming.subtotal, incoming.confidence_score,
		        incoming.match_status::item_match_status, incoming.quantity_rationale
		 FROM incoming
		 JOIN quote_version version ON version.account_id = $1 AND version.id = $2
		 ORDER BY incoming.position
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// The join is what refuses a version of another account: the foreign key alone would accept
	// one, and the row would land in this tenant pointing at somebody else's quote.
	if len(created) != len(items) {
		return nil, domain.ErrNotFound
	}
	return created, nil
}

// ApplyPricing freezes every line's valuation in one statement, keyed by line id. It writes the
// empty ones too, so the count check covers the whole version: the account predicate on the join
// and the one on the line each refuse another tenant's version on their own, and the row count is
// what turns either refusal into an error rather than a statement that quietly matched nothing.
func (r *QuoteRepository) ApplyPricing(
	ctx context.Context, q Querier, accountID, versionID uuid.UUID,
	pricings []domain.QuoteItemPricing,
) error {
	payload, err := json.Marshal(quoteItemPricingPayloads(pricings))
	if err != nil {
		return err
	}

	tag, err := q.Exec(ctx,
		`WITH incoming AS (
		   SELECT *
		   FROM jsonb_to_recordset($3::jsonb) AS x(
		     item_id uuid,
		     unit_price_snapshot numeric,
		     min_price_snapshot numeric,
		     subtotal numeric
		   )
		 )
		 UPDATE quote_item item
		 SET unit_price_snapshot = incoming.unit_price_snapshot,
		     min_price_snapshot = incoming.min_price_snapshot,
		     subtotal = incoming.subtotal
		 FROM incoming
		 JOIN quote_version version ON version.account_id = $1 AND version.id = $2
		 WHERE item.account_id = $1
		   AND item.version_id = version.id
		   AND item.id = incoming.item_id`,
		accountID, versionID, payload)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != int64(len(pricings)) {
		return domain.ErrNotFound
	}
	return nil
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
	// Position rides the payload because the caller's order is the client's order, and
	// jsonb_to_recordset promises none of its own.
	Position             int        `json:"position"`
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

type quoteItemPricingPayload struct {
	ItemID            uuid.UUID `json:"item_id"`
	UnitPriceSnapshot *string   `json:"unit_price_snapshot"`
	MinPriceSnapshot  *string   `json:"min_price_snapshot"`
	Subtotal          *string   `json:"subtotal"`
}

func quoteItemPayloads(items []domain.NewQuoteItem) []quoteItemPayload {
	payloads := make([]quoteItemPayload, 0, len(items))
	for i, item := range items {
		payloads = append(payloads, quoteItemPayload{
			Position:             i,
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

func quoteItemPricingPayloads(pricings []domain.QuoteItemPricing) []quoteItemPricingPayload {
	payloads := make([]quoteItemPricingPayload, 0, len(pricings))
	for _, pricing := range pricings {
		payloads = append(payloads, quoteItemPricingPayload{
			ItemID:            pricing.ItemID,
			UnitPriceSnapshot: nullDecimalString(pricing.UnitPriceSnapshot),
			MinPriceSnapshot:  nullDecimalString(pricing.MinPriceSnapshot),
			Subtotal:          nullDecimalString(pricing.Subtotal),
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
