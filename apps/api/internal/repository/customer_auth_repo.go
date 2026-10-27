package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

const customerIdentityColumns = `id, account_id, tax_id, COALESCE(password_hash,''), name, google_subject, recovery_email, recovery_email_verified_at,
 failed_attempts, locked_until, disabled_at, created_at`

// CustomerAuthRepository stores customer credentials without accessing commercial client profiles.
type CustomerAuthRepository struct{}

// NewCustomerAuthRepository builds the customer credential repository.
func NewCustomerAuthRepository() *CustomerAuthRepository { return &CustomerAuthRepository{} }

// GetHostnameByDeliveryTokenCrossAccount discovers only public supplier routing for legacy links.
func (r *CustomerAuthRepository) GetHostnameByDeliveryTokenCrossAccount(ctx context.Context, q Querier, token string) (string, error) {
	var hostname string
	err := q.QueryRow(ctx, `SELECT a.public_hostname FROM quote_send s JOIN account a ON a.id=s.account_id AND a.is_active WHERE s.public_token=$1 AND s.tracking_status IN ('SENT','DELIVERED','VIEWED') AND a.public_hostname IS NOT NULL`, token).Scan(&hostname)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	return hostname, err
}

// GetCustomerHostname reads a supplier's provisioned customer portal within tenant scope.
func (r *CustomerAuthRepository) GetCustomerHostname(ctx context.Context, q Querier, accountID uuid.UUID) (string, error) {
	var hostname string
	err := q.QueryRow(ctx, `SELECT public_hostname FROM account WHERE id=$1 AND is_active AND public_hostname IS NOT NULL`, accountID).Scan(&hostname)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrNotFound
	}
	return hostname, err
}

// GetSiteCrossAccount resolves an exact registered hostname before entering tenant scope.
func (r *CustomerAuthRepository) GetSiteCrossAccount(ctx context.Context, q Querier, hostname string) (*domain.CustomerSite, error) {
	var site domain.CustomerSite
	err := q.QueryRow(ctx, `SELECT id, public_hostname, name, brand_logo_url FROM account
  WHERE public_hostname = $1 AND is_active`, hostname).Scan(&site.AccountID, &site.Hostname, &site.SupplierName, &site.LogoURL)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &site, nil
}

// GetIdentityByTaxID reads only an active credential in the resolved supplier.
func (r *CustomerAuthRepository) GetIdentityByTaxID(ctx context.Context, q Querier, accountID uuid.UUID, taxID string) (*domain.CustomerIdentity, error) {
	return scanCustomerIdentity(q.QueryRow(ctx, `SELECT `+customerIdentityColumns+`
  FROM customer_identity WHERE account_id = $1 AND tax_id = $2 AND disabled_at IS NULL`, accountID, taxID))
}

// GetIdentityForUpdate serializes credential changes and session creation for one identity.
func (r *CustomerAuthRepository) GetIdentityForUpdate(ctx context.Context, q Querier, accountID, id uuid.UUID) (*domain.CustomerIdentity, error) {
	return scanCustomerIdentity(q.QueryRow(ctx, `SELECT `+customerIdentityColumns+`
  FROM customer_identity WHERE account_id = $1 AND id = $2 FOR UPDATE`, accountID, id))
}

// GetSession authenticates the opaque token within its hostname's account on every request.
func (r *CustomerAuthRepository) GetSession(ctx context.Context, q Querier, accountID uuid.UUID, hash string, now time.Time) (*domain.CustomerPrincipal, error) {
	var principal domain.CustomerPrincipal
	err := q.QueryRow(ctx, `SELECT c.id, c.account_id, c.tax_id, c.name, s.id, s.expires_at
  FROM customer_session s JOIN customer_identity c ON c.account_id = s.account_id AND c.id = s.customer_id
  WHERE s.account_id = $1 AND s.token_hash = $2 AND s.expires_at > $3 AND s.revoked_at IS NULL AND c.disabled_at IS NULL`,
		accountID, hash, now).Scan(&principal.CustomerID, &principal.AccountID, &principal.TaxID,
		&principal.Name, &principal.SessionID, &principal.SessionExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrUnauthenticated
	}
	if err != nil {
		return nil, err
	}
	return &principal, nil
}

// CreateIdentity registers credentials without associating preexisting RFQs or client profiles.
func (r *CustomerAuthRepository) CreateIdentity(ctx context.Context, q Querier, in domain.CustomerIdentity) (*domain.CustomerIdentity, error) {
	identity, err := scanCustomerIdentity(q.QueryRow(ctx, `INSERT INTO customer_identity
  (id, account_id, tax_id, password_hash, recovery_email, name) VALUES ($1,$2,$3,$4,$5,$6)
  ON CONFLICT (account_id, tax_id) WHERE disabled_at IS NULL DO NOTHING RETURNING `+customerIdentityColumns,
		in.ID, in.AccountID, in.TaxID, in.PasswordHash, in.RecoveryEmail, in.Name))
	if errors.Is(err, domain.ErrNotFound) {
		return nil, domain.ErrConflict
	}
	return identity, err
}

// CreateSession persists only the hash of the random session secret.
func (r *CustomerAuthRepository) CreateSession(ctx context.Context, q Querier, in domain.CustomerSession) error {
	_, err := q.Exec(ctx, `INSERT INTO customer_session (id,account_id,customer_id,token_hash,expires_at)
  VALUES ($1,$2,$3,$4,$5)`, in.ID, in.AccountID, in.CustomerID, in.TokenHash, in.ExpiresAt)
	return err
}

// RecordFailedAttempt atomically counts failures and applies the configured lockout.
func (r *CustomerAuthRepository) RecordFailedAttempt(ctx context.Context, q Querier, accountID, id uuid.UUID, maxAttempts int, now, lockUntil time.Time) error {
	_, err := q.Exec(ctx, `UPDATE customer_identity SET
  failed_attempts = CASE WHEN locked_until <= $4 THEN 1 ELSE failed_attempts+1 END,
  locked_until = CASE WHEN (CASE WHEN locked_until <= $4 THEN 1 ELSE failed_attempts+1 END) >= $3 THEN $5
    WHEN locked_until <= $4 THEN NULL ELSE locked_until END
  WHERE account_id=$1 AND id=$2 AND disabled_at IS NULL`, accountID, id, maxAttempts, now, lockUntil)
	return err
}

// RecordSuccessfulLogin clears the credential-attempt counters after a verified login.
func (r *CustomerAuthRepository) RecordSuccessfulLogin(ctx context.Context, q Querier, accountID, id uuid.UUID) error {
	_, err := q.Exec(ctx, `UPDATE customer_identity SET failed_attempts=0,locked_until=NULL WHERE account_id=$1 AND id=$2`, accountID, id)
	return err
}

// RevokeSession ends the one authenticated session named by the caller.
func (r *CustomerAuthRepository) RevokeSession(ctx context.Context, q Querier, accountID, customerID, sessionID uuid.UUID, now time.Time) error {
	_, err := q.Exec(ctx, `UPDATE customer_session SET revoked_at=$4
  WHERE account_id=$1 AND customer_id=$2 AND id=$3 AND revoked_at IS NULL`, accountID, customerID, sessionID, now)
	return err
}

func scanCustomerIdentity(row pgx.Row) (*domain.CustomerIdentity, error) {
	var identity domain.CustomerIdentity
	err := row.Scan(&identity.ID, &identity.AccountID, &identity.TaxID, &identity.PasswordHash,
		&identity.Name, &identity.GoogleSubject, &identity.RecoveryEmail, &identity.RecoveryEmailVerifiedAt, &identity.FailedAttempts, &identity.LockedUntil, &identity.DisabledAt, &identity.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &identity, nil
}
