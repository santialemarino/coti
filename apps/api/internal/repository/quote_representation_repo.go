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

const quoteRepresentationColumns = `id, account_id, branch_id, quote_id, version_id,
	schema_version, payload, message, pdf_storage_key, pdf_content_type, pdf_size_bytes,
	pdf_sha256, logo_fallback_used, created_at`

// QuoteRepresentationRepository owns immutable representation persistence and its source read.
type QuoteRepresentationRepository struct{}

// NewQuoteRepresentationRepository builds a QuoteRepresentationRepository.
func NewQuoteRepresentationRepository() *QuoteRepresentationRepository {
	return &QuoteRepresentationRepository{}
}

// GetByVersion loads an immutable bundle through all tenant and branch boundaries.
func (r *QuoteRepresentationRepository) GetByVersion(
	ctx context.Context, q Querier, accountID, branchID, quoteID, versionID uuid.UUID,
) (*domain.QuoteRepresentation, error) {
	return scanQuoteRepresentation(q.QueryRow(ctx, `SELECT `+quoteRepresentationColumns+`
		FROM quote_representation
		WHERE account_id = $1 AND branch_id = $2 AND quote_id = $3 AND version_id = $4`,
		accountID, branchID, quoteID, versionID))
}

// GetByVersionID loads a bundle after a successful public token resolved its account.
func (r *QuoteRepresentationRepository) GetByVersionID(
	ctx context.Context, q Querier, accountID, versionID uuid.UUID,
) (*domain.QuoteRepresentation, error) {
	return scanQuoteRepresentation(q.QueryRow(ctx, `SELECT `+quoteRepresentationColumns+`
		FROM quote_representation
		WHERE account_id = $1 AND version_id = $2`, accountID, versionID))
}

// Create inserts a bundle only when its quote and version belong to the requested branch.
func (r *QuoteRepresentationRepository) Create(
	ctx context.Context, q Querier, accountID uuid.UUID, in domain.NewQuoteRepresentation,
) (*domain.QuoteRepresentation, error) {
	payload, err := json.Marshal(in.Payload)
	if err != nil {
		return nil, err
	}
	representation, err := scanQuoteRepresentation(q.QueryRow(ctx,
		`INSERT INTO quote_representation (
		   id, account_id, branch_id, quote_id, version_id, schema_version, payload, message,
		   pdf_storage_key, pdf_content_type, pdf_size_bytes, pdf_sha256, logo_fallback_used
		 )
		 SELECT $2, $1, quote.branch_id, quote.id, version.id, $6, $7, $8, $9, $10, $11, $12, $13
		 FROM quote
		 JOIN quote_version version ON version.account_id = $1 AND version.id = $5
		                           AND version.quote_id = quote.id AND version.is_immutable = TRUE
		 WHERE quote.account_id = $1 AND quote.branch_id = $3 AND quote.id = $4
		 RETURNING `+quoteRepresentationColumns,
		accountID, in.ID, in.BranchID, in.QuoteID, in.VersionID, in.SchemaVersion, payload,
		in.Message, in.PDFStorageKey, in.PDFContentType, in.PDFSizeBytes, in.PDFSHA256,
		in.LogoFallbackUsed))
	if isUniqueViolation(err, "uq_quote_representation_version") {
		return nil, domain.ErrConflict
	}
	return representation, err
}

// LoadSource reads the mutable commercial model once before it becomes a representation.
func (r *QuoteRepresentationRepository) LoadSource(
	ctx context.Context, q Querier, accountID, branchID, quoteID, versionID uuid.UUID,
) (*domain.QuoteRepresentationSource, error) {
	var source domain.QuoteRepresentationSource
	err := q.QueryRow(ctx, `SELECT
		account.id, account.name, account.legal_name, account.tax_id, account.brand_logo_url,
		account.brand_color, account.is_active, account.created_at, account.updated_at,
		branch.id, branch.account_id, branch.name, branch.address, branch.default_expiry_days,
		branch.is_active, branch.created_at, branch.updated_at,
		quote.id, quote.account_id, quote.number, quote.branch_id, quote.client_id, quote.rfq_id,
		quote.seller_id, quote.current_version_id, quote.current_status, quote.expires_at,
		quote.archived_at, quote.needs_followup, quote.followup_flagged_at, quote.created_at,
		quote.updated_at,
		version.id, version.account_id, version.quote_id, version.author_id,
		version.version_number, version.currency, version.total, version.is_immutable,
		version.frozen_at, version.comment, version.created_at,
		COALESCE(client.name, rfq.client_label)
	FROM quote
	JOIN account ON account.id = $1
	JOIN branch ON branch.account_id = $1 AND branch.id = $2 AND branch.id = quote.branch_id
	JOIN quote_version version ON version.account_id = $1 AND version.id = $4
	                          AND version.quote_id = quote.id
	JOIN rfq ON rfq.account_id = $1 AND rfq.id = quote.rfq_id
	LEFT JOIN client ON client.account_id = $1 AND client.id = quote.client_id
	WHERE quote.account_id = $1 AND quote.id = $3`,
		accountID, branchID, quoteID, versionID).Scan(
		&source.Account.ID, &source.Account.Name, &source.Account.LegalName, &source.Account.TaxID,
		&source.Account.BrandLogoURL, &source.Account.BrandColor, &source.Account.IsActive,
		&source.Account.CreatedAt, &source.Account.UpdatedAt,
		&source.Branch.ID, &source.Branch.AccountID, &source.Branch.Name, &source.Branch.Address,
		&source.Branch.DefaultExpiryDays, &source.Branch.IsActive, &source.Branch.CreatedAt,
		&source.Branch.UpdatedAt,
		&source.Quote.ID, &source.Quote.AccountID, &source.Quote.Number, &source.Quote.BranchID,
		&source.Quote.ClientID, &source.Quote.RFQID, &source.Quote.SellerID,
		&source.Quote.CurrentVersionID, &source.Quote.CurrentStatus, &source.Quote.ExpiresAt,
		&source.Quote.ArchivedAt, &source.Quote.NeedsFollowup, &source.Quote.FollowupFlaggedAt,
		&source.Quote.CreatedAt, &source.Quote.UpdatedAt,
		&source.Version.ID, &source.Version.AccountID, &source.Version.QuoteID,
		&source.Version.AuthorID, &source.Version.VersionNumber, &source.Version.Currency,
		&source.Version.Total, &source.Version.IsImmutable, &source.Version.FrozenAt,
		&source.Version.Comment,
		&source.Version.CreatedAt, &source.CustomerName)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	items, err := r.listItems(ctx, q, accountID, versionID)
	if err != nil {
		return nil, err
	}
	source.Items = items
	source.Alternatives, err = r.listApprovedAlternatives(ctx, q, accountID, versionID,
		quoteItemIDsForRepresentation(items))
	if err != nil {
		return nil, err
	}
	source.Discounts, err = r.listDiscounts(ctx, q, accountID, versionID)
	if err != nil {
		return nil, err
	}
	return &source, nil
}

func (r *QuoteRepresentationRepository) listItems(
	ctx context.Context, q Querier, accountID, versionID uuid.UUID,
) ([]domain.QuoteItem, error) {
	rows, err := q.Query(ctx, `SELECT `+quoteItemColumnsPrefixed+`, p.code, p.canonical_name, p.unit
		FROM quote_item qi
		LEFT JOIN product p ON p.account_id = qi.account_id AND p.id = qi.product_id
		WHERE qi.account_id = $1 AND qi.version_id = $2
		ORDER BY qi.created_at, qi.id`, accountID, versionID)
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

func (r *QuoteRepresentationRepository) listApprovedAlternatives(
	ctx context.Context, q Querier, accountID, versionID uuid.UUID, itemIDs []uuid.UUID,
) (map[uuid.UUID][]domain.QuoteItemAlternative, error) {
	result := make(map[uuid.UUID][]domain.QuoteItemAlternative, len(itemIDs))
	if len(itemIDs) == 0 {
		return result, nil
	}
	rows, err := q.Query(ctx, `SELECT alternative.id, alternative.account_id,
		alternative.quote_item_id, alternative.product_id, alternative.combo_id, alternative.type,
		alternative.origin, alternative.rank, alternative.confidence_score,
		alternative.price_snapshot, alternative.approved_by_seller, alternative.chosen_by_client,
		alternative.created_at, product.code, COALESCE(product.canonical_name, combo.name), product.unit
	FROM quote_item_alternative alternative
	JOIN quote_item item ON item.account_id = $1 AND item.version_id = $2
	                    AND item.id = alternative.quote_item_id
	LEFT JOIN product ON product.account_id = $1 AND product.id = alternative.product_id
	LEFT JOIN combo ON combo.account_id = $1 AND combo.id = alternative.combo_id
	WHERE alternative.account_id = $1 AND alternative.quote_item_id = ANY($3)
	  AND alternative.approved_by_seller = TRUE
	ORDER BY alternative.quote_item_id, alternative.rank`, accountID, versionID, itemIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		alternative, scanErr := scanQuoteItemAlternative(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result[alternative.QuoteItemID] = append(result[alternative.QuoteItemID], *alternative)
	}
	return result, rows.Err()
}

func (r *QuoteRepresentationRepository) listDiscounts(
	ctx context.Context, q Querier, accountID, versionID uuid.UUID,
) ([]domain.QuoteRepresentationDiscount, error) {
	rows, err := q.Query(ctx, `SELECT COALESCE(description, ''), amount
		FROM quote_discount
		WHERE account_id = $1 AND quote_version_id = $2 AND suppressed_by_seller = FALSE
		ORDER BY created_at, id`, accountID, versionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	discounts := make([]domain.QuoteRepresentationDiscount, 0)
	for rows.Next() {
		var discount domain.QuoteRepresentationDiscount
		var amount decimal.Decimal
		if err := rows.Scan(&discount.Description, &amount); err != nil {
			return nil, err
		}
		discount.Amount = amount.StringFixed(domain.MoneyScale)
		discounts = append(discounts, discount)
	}
	return discounts, rows.Err()
}

func quoteItemIDsForRepresentation(items []domain.QuoteItem) []uuid.UUID {
	ids := make([]uuid.UUID, len(items))
	for i := range items {
		ids[i] = items[i].ID
	}
	return ids
}

func scanQuoteRepresentation(row pgx.Row) (*domain.QuoteRepresentation, error) {
	var representation domain.QuoteRepresentation
	var payload []byte
	err := row.Scan(&representation.ID, &representation.AccountID, &representation.BranchID,
		&representation.QuoteID, &representation.VersionID, &representation.SchemaVersion,
		&payload, &representation.Message, &representation.PDFStorageKey,
		&representation.PDFContentType, &representation.PDFSizeBytes,
		&representation.PDFSHA256, &representation.LogoFallbackUsed, &representation.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(payload, &representation.Payload); err != nil {
		return nil, err
	}
	return &representation, nil
}
