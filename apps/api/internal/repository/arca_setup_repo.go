package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// ARCASetupRepository persists an account's encrypted ARCA credentials.
type ARCASetupRepository struct{}

// NewARCASetupRepository builds an ARCASetupRepository.
func NewARCASetupRepository() *ARCASetupRepository { return &ARCASetupRepository{} }

// Get returns the account's setup. Returns domain.ErrNotFound when it has none.
func (r *ARCASetupRepository) Get(ctx context.Context, q Querier, accountID uuid.UUID) (*domain.ARCASetup, error) {
	var s domain.ARCASetup
	err := q.QueryRow(ctx, `SELECT id, environment, tax_id, csr, sealed_key, certificate, certificate_expires_at
	 FROM arca_setup WHERE account_id = $1`, accountID).Scan(&s.ID, &s.Environment, &s.TaxID, &s.CSR, &s.SealedKey,
		&s.Certificate, &s.CertificateExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return &s, err
}

// Exists reports whether the account has an ARCA setup, finished or not.
func (r *ARCASetupRepository) Exists(ctx context.Context, q Querier, accountID uuid.UUID) (bool, error) {
	var exists bool
	err := q.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM arca_setup WHERE account_id = $1)`, accountID).Scan(&exists)
	return exists, err
}

// Create inserts the account's first certificate request.
func (r *ARCASetupRepository) Create(ctx context.Context, q Querier, accountID uuid.UUID, s domain.ARCASetup) error {
	_, err := q.Exec(ctx, `INSERT INTO arca_setup (id, account_id, environment, tax_id, csr, sealed_key)
	 VALUES ($1, $2, $3, $4, $5, $6)`, s.ID, accountID, s.Environment, s.TaxID, s.CSR, s.SealedKey)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return domain.ErrConflict
	}
	return err
}

// SaveCertificate installs the certificate if the request has not changed.
func (r *ARCASetupRepository) SaveCertificate(ctx context.Context, q Querier, accountID, setupID uuid.UUID, certificate string, expires time.Time) error {
	tag, err := q.Exec(ctx, `UPDATE arca_setup SET certificate = $3, certificate_expires_at = $4,
	 sealed_ticket = '' WHERE account_id = $1 AND id = $2 AND certificate = ''`, accountID, setupID, certificate, expires)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrConflict
	}
	return nil
}

// Delete removes the account's setup.
func (r *ARCASetupRepository) Delete(ctx context.Context, q Querier, accountID uuid.UUID) error {
	_, err := q.Exec(ctx, `DELETE FROM arca_setup WHERE account_id = $1`, accountID)
	return err
}
