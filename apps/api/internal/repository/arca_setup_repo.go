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

// ARCASetupRepository persists encrypted account credentials and branch verification.
type ARCASetupRepository struct{}

// NewARCASetupRepository builds an ARCASetupRepository.
func NewARCASetupRepository() *ARCASetupRepository { return &ARCASetupRepository{} }

// Get returns the account setup and the selected branch's verification.
func (r *ARCASetupRepository) Get(ctx context.Context, q Querier, accountID, branchID uuid.UUID) (*domain.ARCASetup, error) {
	var s domain.ARCASetup
	err := q.QueryRow(ctx, `SELECT s.id, s.tax_id, s.csr, s.sealed_key, s.certificate,
	 s.sealed_ticket, s.certificate_expires_at, COALESCE(b.point_of_sale, 0), b.verified_at
	 FROM arca_setup s LEFT JOIN branch_arca_setup b
	 ON b.account_id = s.account_id AND b.setup_id = s.id AND b.branch_id = $2
	 WHERE s.account_id = $1`, accountID, branchID).Scan(&s.ID, &s.TaxID, &s.CSR, &s.SealedKey,
		&s.Certificate, &s.SealedTicket, &s.CertificateExpiresAt, &s.PointOfSale, &s.VerifiedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return &s, err
}

// Create inserts the account's first certificate request.
func (r *ARCASetupRepository) Create(ctx context.Context, q Querier, accountID uuid.UUID, s domain.ARCASetup) error {
	_, err := q.Exec(ctx, `INSERT INTO arca_setup (id, account_id, tax_id, csr, sealed_key)
	 VALUES ($1, $2, $3, $4, $5)`, s.ID, accountID, s.TaxID, s.CSR, s.SealedKey)
	return arcaConstraintError(err)
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

// SaveTicket retains a WSAA ticket without marking a branch as verified.
func (r *ARCASetupRepository) SaveTicket(ctx context.Context, q Querier, accountID, setupID uuid.UUID, ticket string) error {
	tag, err := q.Exec(ctx, `UPDATE arca_setup SET sealed_ticket = $3 WHERE account_id = $1 AND id = $2`, accountID, setupID, ticket)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrConflict
	}
	return nil
}

// SaveBranch records a successful check under the unchanged account identity.
func (r *ARCASetupRepository) SaveBranch(ctx context.Context, q Querier, accountID, branchID, setupID uuid.UUID, pointOfSale int) error {
	tag, err := q.Exec(ctx, `INSERT INTO branch_arca_setup (account_id, branch_id, setup_id, point_of_sale)
	 SELECT $1, $2, id, $4 FROM arca_setup WHERE account_id = $1 AND id = $3
	 ON CONFLICT (account_id, branch_id) DO UPDATE SET setup_id = EXCLUDED.setup_id,
	 point_of_sale = EXCLUDED.point_of_sale, verified_at = now()`, accountID, branchID, setupID, pointOfSale)
	if err != nil {
		return arcaConstraintError(err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrConflict
	}
	return nil
}

// Delete removes the setup and every branch verification under it.
func (r *ARCASetupRepository) Delete(ctx context.Context, q Querier, accountID uuid.UUID) error {
	_, err := q.Exec(ctx, `DELETE FROM arca_setup WHERE account_id = $1`, accountID)
	return err
}

func arcaConstraintError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "23505" || pgErr.Code == "23503") {
		return domain.ErrConflict
	}
	return err
}
