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

// GetIdentityByGoogleSubject locates only a previously linked stable Google subject.
func (r *CustomerAuthRepository) GetIdentityByGoogleSubject(ctx context.Context, q Querier, accountID uuid.UUID, subject string) (*domain.CustomerIdentity, error) {
	return scanCustomerIdentity(q.QueryRow(ctx, `SELECT `+customerIdentityColumns+` FROM customer_identity WHERE account_id=$1 AND google_subject=$2 AND disabled_at IS NULL`, accountID, subject))
}

// GetVerifiedRecoveryIdentity requires both declared recovery identifiers to match stored verified data.
func (r *CustomerAuthRepository) GetVerifiedRecoveryIdentity(ctx context.Context, q Querier, accountID uuid.UUID, taxID, email string) (*domain.CustomerIdentity, error) {
	return scanCustomerIdentity(q.QueryRow(ctx, `SELECT `+customerIdentityColumns+` FROM customer_identity WHERE account_id=$1 AND tax_id=$2 AND recovery_email=$3 AND recovery_email_verified_at IS NOT NULL AND disabled_at IS NULL`, accountID, taxID, email))
}

// ConsumeChallenge atomically redeems one unexpired token of the expected purpose.
func (r *CustomerAuthRepository) ConsumeChallenge(ctx context.Context, q Querier, accountID uuid.UUID, hash string, purpose domain.CustomerChallengePurpose, now time.Time) (*domain.CustomerChallenge, error) {
	var token domain.CustomerChallenge
	err := q.QueryRow(ctx, `UPDATE customer_auth_challenge SET consumed_at=$4 WHERE account_id=$1 AND token_hash=$2 AND purpose=$3 AND consumed_at IS NULL AND expires_at>$4 RETURNING id,account_id,customer_id,purpose,token_hash,payload,expires_at`, accountID, hash, purpose, now).Scan(&token.ID, &token.AccountID, &token.CustomerID, &token.Purpose, &token.TokenHash, &token.Payload, &token.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUnauthenticated
	}
	if err != nil {
		return nil, err
	}
	return &token, nil
}

// CreateGoogleIdentity registers a new CUIT/CUIL without merging an existing customer account.
func (r *CustomerAuthRepository) CreateGoogleIdentity(ctx context.Context, q Querier, in domain.CustomerIdentity) (*domain.CustomerIdentity, error) {
	identity, err := scanCustomerIdentity(q.QueryRow(ctx, `INSERT INTO customer_identity (id,account_id,tax_id,google_subject,name) VALUES ($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING RETURNING `+customerIdentityColumns, in.ID, in.AccountID, in.TaxID, in.GoogleSubject, in.Name))
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrConflict
	}
	return identity, err
}

// CreateChallenge persists only the hash of a short-lived one-use capability.
func (r *CustomerAuthRepository) CreateChallenge(ctx context.Context, q Querier, in domain.CustomerChallenge) error {
	_, err := q.Exec(ctx, `INSERT INTO customer_auth_challenge (id,account_id,customer_id,purpose,token_hash,payload,expires_at) VALUES ($1,$2,$3,$4,$5,$6,$7)`, in.ID, in.AccountID, in.CustomerID, in.Purpose, in.TokenHash, in.Payload, in.ExpiresAt)
	return err
}

// LinkGoogle requires an authenticated identity and refuses replacing a different Google subject.
func (r *CustomerAuthRepository) LinkGoogle(ctx context.Context, q Querier, accountID, customerID uuid.UUID, subject string) error {
	result, err := q.Exec(ctx, `UPDATE customer_identity SET google_subject=$3 WHERE account_id=$1 AND id=$2 AND disabled_at IS NULL AND (google_subject IS NULL OR google_subject=$3)`, accountID, customerID, subject)
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "23505" {
		return domain.ErrConflict
	}
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return domain.ErrConflict
	}
	return nil
}

// VerifyRecoveryEmail confirms only the unchanged address originally named by the verification link.
func (r *CustomerAuthRepository) VerifyRecoveryEmail(ctx context.Context, q Querier, accountID, customerID uuid.UUID, email string, now time.Time) error {
	result, err := q.Exec(ctx, `UPDATE customer_identity SET recovery_email_verified_at=$4 WHERE account_id=$1 AND id=$2 AND recovery_email=$3 AND disabled_at IS NULL`, accountID, customerID, email, now)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return domain.ErrUnauthenticated
	}
	return nil
}

// ResetCustomerPassword replaces the password and revokes sessions and remaining recovery capabilities atomically.
func (r *CustomerAuthRepository) ResetCustomerPassword(ctx context.Context, q Querier, accountID, customerID uuid.UUID, hash string, now time.Time) error {
	result, err := q.Exec(ctx, `UPDATE customer_identity SET password_hash=$3,failed_attempts=0,locked_until=NULL WHERE account_id=$1 AND id=$2 AND disabled_at IS NULL`, accountID, customerID, hash)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return domain.ErrUnauthenticated
	}
	if _, err = q.Exec(ctx, `UPDATE customer_session SET revoked_at=$3 WHERE account_id=$1 AND customer_id=$2 AND revoked_at IS NULL`, accountID, customerID, now); err != nil {
		return err
	}
	_, err = q.Exec(ctx, `UPDATE customer_auth_challenge SET consumed_at=$3 WHERE account_id=$1 AND customer_id=$2 AND purpose='RESET_PASSWORD' AND consumed_at IS NULL`, accountID, customerID, now)
	return err
}
