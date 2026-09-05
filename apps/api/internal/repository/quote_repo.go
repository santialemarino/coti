package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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

// ListItemsWithProduct joins product, whose id and unit columns would collide with the
// unqualified ones, so this projection carries its table prefix.
const quoteItemColumnsPrefixed = `qi.id, qi.account_id, qi.version_id, qi.product_id,
	qi.requested_description, qi.quantity, qi.unit, qi.unit_price_snapshot,
	qi.min_price_snapshot, qi.subtotal, qi.confidence_score, qi.match_status,
	qi.quantity_rationale, qi.created_at`

const quoteStatusChangeColumns = `id, account_id, quote_id, previous_status, new_status,
	user_id, changed_at, created_at`

// The catalog identity trails the row's own columns: it is joined from product, which a combo
// alternative points at with nothing, so all three come back empty there.
const quoteItemAlternativeColumns = `alternative.id, alternative.account_id,
	alternative.quote_item_id, alternative.product_id, alternative.combo_id, alternative.type,
	alternative.origin, alternative.rank, alternative.confidence_score,
	alternative.price_snapshot, alternative.approved_by_seller, alternative.chosen_by_client,
	alternative.created_at, product.code, product.canonical_name, product.unit`

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

// GetByRFQID loads the quote associated with an RFQ, scoped to the account. Branch filtering
// is not needed here because RFQ→quote is 1-to-1 and the RFQ already validated the branch.
func (r *QuoteRepository) GetByRFQID(
	ctx context.Context, q Querier, accountID, rfqID uuid.UUID,
) (*domain.Quote, error) {
	return scanQuote(q.QueryRow(ctx,
		`SELECT `+quoteColumns+`
		 FROM quote
		 WHERE account_id = $1 AND rfq_id = $2`,
		accountID, rfqID))
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

// UpdateStatus moves the quote's derived status, and only from the status the caller read. Matching
// no row is a conflict rather than an absence: the transition is atomic because of that predicate.
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

// Archive flags a quote as archived. Archived is an orthogonal flag, not a transition: it sets
// the timestamp without touching current_status. It refuses (no row matched) when the quote is
// absent, already archived, or closed as accepted or rejected — an archived or closed quote has
// no reason to be boxed away again.
func (r *QuoteRepository) Archive(
	ctx context.Context, q Querier, accountID, branchID, quoteID uuid.UUID,
) (*domain.Quote, error) {
	quote, err := scanQuote(q.QueryRow(ctx,
		`UPDATE quote
		 SET archived_at = now()
		 WHERE account_id = $1 AND branch_id = $2 AND id = $3
		   AND archived_at IS NULL
		   AND current_status NOT IN ('ACCEPTED', 'REJECTED')
		 RETURNING `+quoteColumns,
		accountID, branchID, quoteID))
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrConflict
	}
	return quote, err
}

// Unarchive clears the archived timestamp, returning the quote to the list.
func (r *QuoteRepository) Unarchive(
	ctx context.Context, q Querier, accountID, branchID, quoteID uuid.UUID,
) (*domain.Quote, error) {
	quote, err := scanQuote(q.QueryRow(ctx,
		`UPDATE quote
		 SET archived_at = NULL
		 WHERE account_id = $1 AND branch_id = $2 AND id = $3 AND archived_at IS NOT NULL
		 RETURNING `+quoteColumns,
		accountID, branchID, quoteID))
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

// ListItems loads a version's lines. quote_item carries no ordinal column and one batch shares a
// single created_at, so this separates batches and leaves each one in the order it was written.
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
	// The caller names each line so its candidates can reference it. Left unset, every line would
	// insert the all-zeros uuid: the first order writes it as a real key and the next one collides.
	for i, item := range items {
		if item.ID == uuid.Nil {
			return nil, fmt.Errorf("quote item %d carries no id", i)
		}
	}
	payload, err := json.Marshal(quoteItemPayloads(items))
	if err != nil {
		return nil, err
	}

	rows, err := q.Query(ctx,
		`WITH incoming AS (
		   SELECT *
		   FROM jsonb_to_recordset($3::jsonb) AS x(
		     id uuid,
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
		   id, account_id, version_id, product_id, requested_description, quantity, unit,
		   unit_price_snapshot, min_price_snapshot, subtotal, confidence_score,
		   match_status, quantity_rationale
		 )
		 SELECT incoming.id, $1, version.id, incoming.product_id, incoming.requested_description,
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

// ApplyPricing freezes every line's valuation in one statement, keyed by line id, the empty ones
// included. The row count is what turns a predicate that matched nothing into an error.
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

// ListAlternativesByItemIDs loads the candidates offered for the given lines, keyed by line and
// ranked best first. The catalog identity is joined rather than frozen, so a product renamed since
// the match reads under its current name — the same as the product the line itself matched.
func (r *QuoteRepository) ListAlternativesByItemIDs(
	ctx context.Context, q Querier, accountID uuid.UUID, itemIDs []uuid.UUID,
) (map[uuid.UUID][]domain.QuoteItemAlternative, error) {
	byItem := make(map[uuid.UUID][]domain.QuoteItemAlternative, len(itemIDs))
	if len(itemIDs) == 0 {
		return byItem, nil
	}

	rows, err := q.Query(ctx,
		`SELECT `+quoteItemAlternativeColumns+`
		 FROM quote_item_alternative alternative
		 LEFT JOIN product ON product.account_id = alternative.account_id
		                  AND product.id = alternative.product_id
		 WHERE alternative.account_id = $1 AND alternative.quote_item_id = ANY($2)
		 ORDER BY alternative.rank`,
		accountID, itemIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var alternative domain.QuoteItemAlternative
		if scanErr := rows.Scan(&alternative.ID, &alternative.AccountID, &alternative.QuoteItemID,
			&alternative.ProductID, &alternative.ComboID, &alternative.Type, &alternative.Origin,
			&alternative.Rank, &alternative.ConfidenceScore, &alternative.PriceSnapshot,
			&alternative.ApprovedBySeller, &alternative.ChosenByClient, &alternative.CreatedAt,
			&alternative.Code, &alternative.CanonicalName, &alternative.Unit,
		); scanErr != nil {
			return nil, scanErr
		}
		byItem[alternative.QuoteItemID] = append(byItem[alternative.QuoteItemID], alternative)
	}
	return byItem, rows.Err()
}

// CreateAlternatives inserts the candidates offered for a version's lines in one statement. The
// row count is what turns a line of another account into an error: its foreign key would accept
// one, and the row would land in this tenant pointing at somebody else's quote.
func (r *QuoteRepository) CreateAlternatives(
	ctx context.Context, q Querier, accountID uuid.UUID,
	alternatives []domain.NewQuoteItemAlternative,
) error {
	if len(alternatives) == 0 {
		return nil
	}
	payload, err := json.Marshal(quoteItemAlternativePayloads(alternatives))
	if err != nil {
		return err
	}

	tag, err := q.Exec(ctx,
		`WITH incoming AS (
		   SELECT *
		   FROM jsonb_to_recordset($2::jsonb) AS x(
		     quote_item_id uuid,
		     product_id uuid,
		     combo_id uuid,
		     type text,
		     origin text,
		     rank int,
		     confidence_score numeric,
		     price_snapshot numeric
		   )
		 )
		 INSERT INTO quote_item_alternative (
		   account_id, quote_item_id, product_id, combo_id, type, origin, rank,
		   confidence_score, price_snapshot
		 )
		 SELECT $1, item.id, incoming.product_id, incoming.combo_id,
		        incoming.type::quote_item_alternative_type,
		        incoming.origin::quote_item_alternative_origin,
		        incoming.rank, incoming.confidence_score, incoming.price_snapshot
		 FROM incoming
		 JOIN quote_item item ON item.account_id = $1 AND item.id = incoming.quote_item_id`,
		accountID, payload)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != int64(len(alternatives)) {
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
	ID                   uuid.UUID  `json:"id"`
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

type quoteItemAlternativePayload struct {
	QuoteItemID     uuid.UUID  `json:"quote_item_id"`
	ProductID       *uuid.UUID `json:"product_id"`
	ComboID         *uuid.UUID `json:"combo_id"`
	Type            string     `json:"type"`
	Origin          string     `json:"origin"`
	Rank            int        `json:"rank"`
	ConfidenceScore *string    `json:"confidence_score"`
	PriceSnapshot   *string    `json:"price_snapshot"`
}

func quoteItemPayloads(items []domain.NewQuoteItem) []quoteItemPayload {
	payloads := make([]quoteItemPayload, 0, len(items))
	for i, item := range items {
		payloads = append(payloads, quoteItemPayload{
			Position:             i,
			ID:                   item.ID,
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

func quoteItemAlternativePayloads(
	alternatives []domain.NewQuoteItemAlternative,
) []quoteItemAlternativePayload {
	payloads := make([]quoteItemAlternativePayload, 0, len(alternatives))
	for _, alternative := range alternatives {
		payloads = append(payloads, quoteItemAlternativePayload{
			QuoteItemID:     alternative.QuoteItemID,
			ProductID:       alternative.ProductID,
			ComboID:         alternative.ComboID,
			Type:            string(alternative.Type),
			Origin:          string(alternative.Origin),
			Rank:            alternative.Rank,
			ConfidenceScore: nullDecimalString(alternative.ConfidenceScore),
			PriceSnapshot:   nullDecimalString(alternative.PriceSnapshot),
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

// ListItemsWithProduct loads a version's lines joined with product catalog identity. The product
// columns populate QuoteItem.ProductCode/Product Name/ProductUnit; bare ListItems leaves them nil.
func (r *QuoteRepository) ListItemsWithProduct(
	ctx context.Context, q Querier, accountID, versionID uuid.UUID,
) ([]domain.QuoteItem, error) {
	rows, err := q.Query(ctx,
		`SELECT `+quoteItemColumnsPrefixed+`, p.code, p.canonical_name, p.unit
		 FROM quote_item qi
		 LEFT JOIN product p ON p.account_id = qi.account_id AND p.id = qi.product_id
		 WHERE qi.account_id = $1 AND qi.version_id = $2
		 ORDER BY qi.created_at`,
		accountID, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []domain.QuoteItem
	for rows.Next() {
		item, scanErr := scanQuoteItemWithProductRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

// GetItem loads one quote item by ID, scoped to the account and version.
func (r *QuoteRepository) GetItem(
	ctx context.Context, q Querier, accountID, versionID, itemID uuid.UUID,
) (*domain.QuoteItem, error) {
	return scanQuoteItemRow(q.QueryRow(ctx,
		`SELECT `+quoteItemColumns+`
		 FROM quote_item
		 WHERE account_id = $1 AND version_id = $2 AND id = $3`,
		accountID, versionID, itemID))
}

// UpdateItem patches a draft quote item's mutable fields. Only non-nil fields in the update are
// written; the version must not be frozen.
func (r *QuoteRepository) UpdateItem(
	ctx context.Context, q Querier, accountID, versionID, itemID uuid.UUID,
	in domain.QuoteItemUpdate,
) (*domain.QuoteItem, error) {
	setClauses := []string{}
	args := []any{accountID, versionID, itemID}
	argIdx := 4

	if in.ProductID != nil {
		setClauses = append(setClauses, fmt.Sprintf("product_id = $%d", argIdx))
		args = append(args, *in.ProductID)
		argIdx++
	}
	if in.RequestedDescription != nil {
		setClauses = append(setClauses, fmt.Sprintf("requested_description = $%d", argIdx))
		args = append(args, *in.RequestedDescription)
		argIdx++
	}
	if in.Quantity != nil {
		setClauses = append(setClauses, fmt.Sprintf("quantity = $%d", argIdx))
		args = append(args, *in.Quantity)
		argIdx++
	}
	if in.Unit != nil {
		setClauses = append(setClauses, fmt.Sprintf("unit = $%d", argIdx))
		args = append(args, *in.Unit)
		argIdx++
	}
	if in.UnitPriceSnapshot != nil {
		setClauses = append(setClauses, fmt.Sprintf("unit_price_snapshot = $%d", argIdx))
		args = append(args, decimal.NewNullDecimal(*in.UnitPriceSnapshot))
	}

	if len(setClauses) == 0 {
		return r.GetItem(ctx, q, accountID, versionID, itemID)
	}

	// The UPDATE joins quote_version to refuse a frozen version, so RETURNING must read the item
	// table alone: the plain projection collides with the version's id and account_id.
	query := fmt.Sprintf(
		`UPDATE quote_item qi
		 SET %s
		 FROM quote_version qv
		 WHERE qi.account_id = $1 AND qi.version_id = $2 AND qi.id = $3
		   AND qv.account_id = $1 AND qv.id = $2 AND qv.is_immutable = FALSE
		 RETURNING `+quoteItemColumnsPrefixed,
		joinSetClauses(setClauses))

	if _, err := q.Exec(ctx, query, args...); err != nil {
		return nil, err
	}

	// Recompute the subtotal after the column update so it multiplies the new quantity and price;
	// folding it into the SET above would read the pre-update row and leave the total stale. The
	// update is deliberately a second statement so it sees the just-written line.
	if in.Quantity != nil || in.UnitPriceSnapshot != nil {
		if _, err := q.Exec(ctx,
			`UPDATE quote_item
			 SET subtotal = quantity * unit_price_snapshot
			 FROM quote_version qv
			 WHERE quote_item.account_id = $1 AND quote_item.version_id = $2
			   AND quote_item.id = $3 AND quote_item.unit_price_snapshot IS NOT NULL
			   AND qv.account_id = $1 AND qv.id = $2 AND qv.is_immutable = FALSE`,
			accountID, versionID, itemID); err != nil {
			return nil, err
		}
	}

	return r.GetItem(ctx, q, accountID, versionID, itemID)
}

// DeleteItem removes a draft quote item. The version must not be frozen. Returns ErrNotFound if
// the item or a mutable version does not exist.
func (r *QuoteRepository) DeleteItem(
	ctx context.Context, q Querier, accountID, versionID, itemID uuid.UUID,
) error {
	tag, err := q.Exec(ctx,
		`DELETE FROM quote_item qi
		 USING quote_version qv
		 WHERE qi.account_id = $1 AND qi.version_id = $2 AND qi.id = $3
		   AND qv.account_id = $1 AND qv.id = $2 AND qv.is_immutable = FALSE`,
		accountID, versionID, itemID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// CreateSingleItem adds one line to a draft quote version. The version must not be frozen.
func (r *QuoteRepository) CreateSingleItem(
	ctx context.Context, q Querier, accountID, versionID uuid.UUID, in domain.QuoteItemCreate,
) (*domain.QuoteItem, error) {
	return scanQuoteItemRow(q.QueryRow(ctx,
		`INSERT INTO quote_item (account_id, version_id, product_id, requested_description,
		        quantity, unit, match_status)
		 SELECT $1, qv.id, $3::uuid, $4::text, $5::numeric, $6::text,
		        CASE WHEN $3::uuid IS NOT NULL THEN 'MATCHED'::item_match_status
		             ELSE 'NO_MATCH'::item_match_status
		        END
		 FROM quote_version qv
		 WHERE qv.account_id = $1 AND qv.id = $2 AND qv.is_immutable = FALSE
		 RETURNING `+quoteItemColumns,
		accountID, versionID,
		in.ProductID, in.RequestedDescription, in.Quantity, in.Unit))
}

func joinSetClauses(clauses []string) string {
	result := clauses[0]
	for i := 1; i < len(clauses); i++ {
		result += ", " + clauses[i]
	}
	return result
}

func scanQuoteItemWithProductRow(row pgx.Row) (*domain.QuoteItem, error) {
	var item domain.QuoteItem
	err := row.Scan(&item.ID, &item.AccountID, &item.VersionID, &item.ProductID,
		&item.RequestedDescription, &item.Quantity, &item.Unit, &item.UnitPriceSnapshot,
		&item.MinPriceSnapshot, &item.Subtotal, &item.ConfidenceScore, &item.MatchStatus,
		&item.QuantityRationale, &item.CreatedAt,
		&item.ProductCode, &item.ProductName, &item.ProductUnit)
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
