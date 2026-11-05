package repository

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// CustomerPortalRepository persists only the authenticated account's public hostname.
type CustomerPortalRepository struct{}

// NewCustomerPortalRepository builds a CustomerPortalRepository.
func NewCustomerPortalRepository() *CustomerPortalRepository { return &CustomerPortalRepository{} }

// GetHostname returns the account's hostname, including an unconfigured empty value.
func (r *CustomerPortalRepository) GetHostname(ctx context.Context, q Querier, accountID uuid.UUID) (string, error) {
	var hostname string
	err := q.QueryRow(ctx, `SELECT COALESCE(public_hostname, '') FROM account WHERE id = $1`, accountID).Scan(&hostname)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	return hostname, err
}

// IsAvailableCrossAccount discovers only whether a public routing name is free.
func (r *CustomerPortalRepository) IsAvailableCrossAccount(ctx context.Context, q Querier, accountID uuid.UUID, hostname string) (bool, error) {
	var available bool
	err := q.QueryRow(ctx, `SELECT NOT EXISTS (SELECT 1 FROM account WHERE public_hostname = $1 AND id <> $2)`, hostname, accountID).Scan(&available)
	return available, err
}

// UpdateHostname atomically checks the previous hostname and enforces global uniqueness.
func (r *CustomerPortalRepository) UpdateHostname(ctx context.Context, q Querier, accountID uuid.UUID, expected, hostname string) error {
	tag, err := q.Exec(ctx, `UPDATE account SET public_hostname = $3 WHERE id = $1 AND COALESCE(public_hostname, '') = $2`, accountID, expected, hostname)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return domain.ErrConflict
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrConflict
	}
	return nil
}
