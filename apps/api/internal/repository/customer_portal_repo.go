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

// ListTakenCrossAccount reports which of hostnames another account already holds.
func (r *CustomerPortalRepository) ListTakenCrossAccount(ctx context.Context, q Querier, hostnames []string) (map[string]bool, error) {
	rows, err := q.Query(ctx, `SELECT public_hostname FROM account WHERE public_hostname = ANY($1)`, hostnames)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	taken := make(map[string]bool)
	for rows.Next() {
		var hostname string
		if err := rows.Scan(&hostname); err != nil {
			return nil, err
		}
		taken[hostname] = true
	}
	return taken, rows.Err()
}

// ListWithoutPortalCrossAccount returns every account with no portal of its own: none at all, or
// the shared base hostname every customer reaches without a label.
func (r *CustomerPortalRepository) ListWithoutPortalCrossAccount(ctx context.Context, q Querier, baseHostname string) ([]domain.PortalAssignment, error) {
	rows, err := q.Query(ctx, `SELECT id, name, COALESCE(public_hostname, '') FROM account
		WHERE public_hostname IS NULL OR public_hostname = $1 ORDER BY created_at, id`, baseHostname)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	accounts := make([]domain.PortalAssignment, 0)
	for rows.Next() {
		var a domain.PortalAssignment
		if err := rows.Scan(&a.AccountID, &a.AccountName, &a.Previous); err != nil {
			return nil, err
		}
		accounts = append(accounts, a)
	}
	return accounts, rows.Err()
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

// AssignHostnamesCrossAccount writes every assignment in one statement, each only over the
// hostname it was planned from; any row that moved since makes the whole write a conflict.
func (r *CustomerPortalRepository) AssignHostnamesCrossAccount(ctx context.Context, q Querier, assignments []domain.PortalAssignment) error {
	ids := make([]uuid.UUID, len(assignments))
	expected := make([]string, len(assignments))
	hostnames := make([]string, len(assignments))
	for i, a := range assignments {
		ids[i], expected[i], hostnames[i] = a.AccountID, a.Previous, a.Hostname
	}
	tag, err := q.Exec(ctx, `UPDATE account a SET public_hostname = u.hostname
		FROM unnest($1::uuid[], $2::text[], $3::text[]) AS u(id, expected, hostname)
		WHERE a.id = u.id AND COALESCE(a.public_hostname, '') = u.expected`, ids, expected, hostnames)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return domain.ErrConflict
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() != int64(len(assignments)) {
		return domain.ErrConflict
	}
	return nil
}

// LockHostnames serializes hostname allocation until the transaction ends, so two writers never
// pick the same free name; the unique index stays the last word.
func (r *CustomerPortalRepository) LockHostnames(ctx context.Context, q Querier) error {
	_, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('account.public_hostname'))`)
	return err
}
