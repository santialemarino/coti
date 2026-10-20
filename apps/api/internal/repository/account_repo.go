package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const accountColumns = `id, name, legal_name, tax_id, brand_logo_url, brand_color,
	preferred_lower_price_strategy, invoicing_enabled,
	EXISTS (SELECT 1 FROM invoice WHERE invoice.account_id = account.id),
	is_active, created_at, updated_at`

// AccountRepository owns persistence for account.
type AccountRepository struct{}

// NewAccountRepository builds an AccountRepository.
func NewAccountRepository() *AccountRepository {
	return &AccountRepository{}
}

// GetByID returns one account.
func (r *AccountRepository) GetByID(
	ctx context.Context, q Querier, accountID uuid.UUID,
) (*domain.Account, error) {
	account, err := scanAccount(q.QueryRow(ctx,
		`SELECT `+accountColumns+` FROM account WHERE id = $1`, accountID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return account, nil
}

// GetByIDForUpdate returns one account and locks its row until the transaction ends, so a write
// that depends on its CUIT cannot interleave with another one.
func (r *AccountRepository) GetByIDForUpdate(
	ctx context.Context, q Querier, accountID uuid.UUID,
) (*domain.Account, error) {
	account, err := scanAccount(q.QueryRow(ctx,
		`SELECT `+accountColumns+` FROM account WHERE id = $1 FOR UPDATE`, accountID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return account, nil
}

// Create inserts the account a registration opens.
func (r *AccountRepository) Create(
	ctx context.Context, q Querier, name string, legalName, taxID *string,
) (*domain.Account, error) {
	return scanAccount(q.QueryRow(ctx,
		`INSERT INTO account (name, legal_name, tax_id)
		 VALUES ($1, $2, $3)
		 RETURNING `+accountColumns,
		name, legalName, taxID))
}

// Update replaces the account's editable fields.
func (r *AccountRepository) Update(
	ctx context.Context, q Querier, accountID uuid.UUID, in domain.AccountUpdate,
) (*domain.Account, error) {
	account, err := scanAccount(q.QueryRow(ctx,
		`UPDATE account
		 SET name = $2, legal_name = $3, tax_id = $4, brand_logo_url = $5, brand_color = $6,
		     preferred_lower_price_strategy = COALESCE($7::lower_price_strategy,
		       preferred_lower_price_strategy)
		 WHERE id = $1
		 RETURNING `+accountColumns,
		accountID, in.Name, in.LegalName, in.TaxID, in.BrandLogoURL, in.BrandColor,
		in.PreferredLowerPriceStrategy))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return account, nil
}

// SetInvoicingEnabled turns invoicing with Coti on or off for the account.
func (r *AccountRepository) SetInvoicingEnabled(
	ctx context.Context, q Querier, accountID uuid.UUID, enabled bool,
) (*domain.Account, error) {
	account, err := scanAccount(q.QueryRow(ctx,
		`UPDATE account SET invoicing_enabled = $2 WHERE id = $1 RETURNING `+accountColumns,
		accountID, enabled))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return account, nil
}

func scanAccount(row pgx.Row) (*domain.Account, error) {
	var a domain.Account
	if err := row.Scan(&a.ID, &a.Name, &a.LegalName, &a.TaxID, &a.BrandLogoURL, &a.BrandColor,
		&a.PreferredLowerPriceStrategy, &a.InvoicingEnabled, &a.HasInvoices, &a.IsActive, &a.CreatedAt,
		&a.UpdatedAt); err != nil {
		return nil, err
	}
	return &a, nil
}
