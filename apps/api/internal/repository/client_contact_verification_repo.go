package repository

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// GetChallenge reads a proof only within its authenticated customer and supplier.
func (r *ClientContactRepository) GetChallenge(ctx context.Context, q Querier, p domain.CustomerPrincipal, id uuid.UUID) (*domain.ContactChallenge, error) {
	return r.getChallenge(ctx, q, p, id, "")
}

// LockChallenge locks a proof after the destination advisory lock has been acquired.
func (r *ClientContactRepository) LockChallenge(ctx context.Context, q Querier, p domain.CustomerPrincipal, id uuid.UUID) (*domain.ContactChallenge, error) {
	return r.getChallenge(ctx, q, p, id, " FOR UPDATE")
}

func (r *ClientContactRepository) getChallenge(ctx context.Context, q Querier, p domain.CustomerPrincipal, id uuid.UUID, suffix string) (*domain.ContactChallenge, error) {
	c := &domain.ContactChallenge{}
	err := q.QueryRow(ctx, `SELECT id,client_id,kind,value,token_hash,attempts,expires_at,consumed_at FROM client_contact_challenge WHERE account_id=$1 AND customer_id=$2 AND id=$3 AND purpose='VERIFY'`+suffix, p.AccountID, p.CustomerID, id).Scan(&c.ID, &c.ClientID, &c.Kind, &c.Value, &c.TokenHash, &c.Attempts, &c.ExpiresAt, &c.ConsumedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	c.AccountID, c.CustomerID, c.Purpose = p.AccountID, p.CustomerID, domain.ContactChallengeVerify
	return c, err
}

// GetRecoveryChallenge reads one account recovery code, whichever customer it was sent for.
func (r *ClientContactRepository) GetRecoveryChallenge(ctx context.Context, q Querier, accountID, id uuid.UUID) (*domain.ContactChallenge, error) {
	return r.getRecoveryChallenge(ctx, q, accountID, id, "")
}

// LockRecoveryChallenge locks a recovery code after the destination advisory lock has been acquired.
func (r *ClientContactRepository) LockRecoveryChallenge(ctx context.Context, q Querier, accountID, id uuid.UUID) (*domain.ContactChallenge, error) {
	return r.getRecoveryChallenge(ctx, q, accountID, id, " FOR UPDATE")
}

func (r *ClientContactRepository) getRecoveryChallenge(ctx context.Context, q Querier, accountID, id uuid.UUID, suffix string) (*domain.ContactChallenge, error) {
	c := &domain.ContactChallenge{AccountID: accountID, Purpose: domain.ContactChallengeRecover}
	err := q.QueryRow(ctx, `SELECT id,customer_id,client_id,kind,value,token_hash,attempts,expires_at,consumed_at FROM client_contact_challenge WHERE account_id=$1 AND id=$2 AND purpose='RECOVER'`+suffix, accountID, id).Scan(&c.ID, &c.CustomerID, &c.ClientID, &c.Kind, &c.Value, &c.TokenHash, &c.Attempts, &c.ExpiresAt, &c.ConsumedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return c, err
}

// ListRecoveryContacts returns the live contacts the customer proved on its profile, the main one first.
func (r *ClientContactRepository) ListRecoveryContacts(ctx context.Context, q Querier, accountID, clientID, customerID uuid.UUID) ([]domain.ClientContact, error) {
	rows, err := q.Query(ctx, `SELECT c.id,c.kind,c.value,c.id IS NOT DISTINCT FROM `+mainContact+` FROM client_contact c
 JOIN customer_contact_ownership o ON o.account_id=c.account_id AND o.customer_id=$3 AND o.contact_id=c.id
 WHERE c.account_id=$1 AND c.client_id=$2 AND c.verified_at IS NOT NULL AND c.retired_at IS NULL
 ORDER BY c.is_main DESC,c.kind='WHATSAPP' DESC,c.created_at,c.id`, accountID, clientID, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	contacts := []domain.ClientContact{}
	for rows.Next() {
		var c domain.ClientContact
		if err := rows.Scan(&c.ID, &c.Kind, &c.Value, &c.Main); err != nil {
			return nil, err
		}
		c.Verified = true
		contacts = append(contacts, c)
	}
	return contacts, rows.Err()
}

// HasRecentChallenge limits repeat delivery to the same customer and destination.
func (r *ClientContactRepository) HasRecentChallenge(ctx context.Context, q Querier, p domain.CustomerPrincipal, kind domain.ChannelType, value string, since time.Time) (bool, error) {
	var found bool
	err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM client_contact_challenge WHERE account_id=$1 AND customer_id=$2 AND kind=$3 AND value=$4 AND created_at>$5 AND consumed_at IS NULL)`, p.AccountID, p.CustomerID, kind, value, since).Scan(&found)
	return found, err
}

// GetDestinationActivity counts one supplier's proofs and failed guesses for a destination, plus every
// supplier's proofs for it.
func (r *ClientContactRepository) GetDestinationActivity(ctx context.Context, q Querier, accountID uuid.UUID, kind domain.ChannelType, value string, since domain.DestinationWindows) (*domain.DestinationActivity, error) {
	a := &domain.DestinationActivity{}
	err := q.QueryRow(ctx, `SELECT supplier_challenges,supplier_failures,all_challenges FROM contact_destination_activity($1,$2,$3,$4,$5,$6)`, accountID, kind, value, since.Challenges, since.Failures, since.AllSuppliers).Scan(&a.SupplierChallenges, &a.SupplierFailures, &a.AllChallenges)
	return a, err
}

// CreateChallenge persists only a hash and invalidates earlier proofs of the same purpose for the destination.
func (r *ClientContactRepository) CreateChallenge(ctx context.Context, q Querier, c domain.ContactChallenge) error {
	if _, err := q.Exec(ctx, `UPDATE client_contact_challenge SET consumed_at=now() WHERE account_id=$1 AND customer_id=$2 AND kind=$3 AND value=$4 AND purpose=$5 AND consumed_at IS NULL`, c.AccountID, c.CustomerID, c.Kind, c.Value, c.Purpose); err != nil {
		return err
	}
	_, err := q.Exec(ctx, `INSERT INTO client_contact_challenge(id,account_id,customer_id,client_id,purpose,kind,value,token_hash,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, c.ID, c.AccountID, c.CustomerID, c.ClientID, c.Purpose, c.Kind, c.Value, c.TokenHash, c.ExpiresAt)
	return err
}

// CreateFailure records one wrong proof against the challenge's destination.
func (r *ClientContactRepository) CreateFailure(ctx context.Context, q Querier, c domain.ContactChallenge) error {
	_, err := q.Exec(ctx, `INSERT INTO client_contact_challenge_failure(account_id,challenge_id,kind,value) VALUES($1,$2,$3,$4)`, c.AccountID, c.ID, c.Kind, c.Value)
	return err
}

// FinishChallenge retains attempt counts and consumes successful or cancelled proofs.
func (r *ClientContactRepository) FinishChallenge(ctx context.Context, q Querier, p domain.CustomerPrincipal, id uuid.UUID, consume bool) error {
	_, err := q.Exec(ctx, `UPDATE client_contact_challenge SET attempts=attempts+1,consumed_at=CASE WHEN $4 THEN now() ELSE consumed_at END WHERE account_id=$1 AND customer_id=$2 AND id=$3`, p.AccountID, p.CustomerID, id, consume)
	return err
}

// MarkVerified activates exactly the destination whose ownership was proved.
func (r *ClientContactRepository) MarkVerified(ctx context.Context, q Querier, p domain.CustomerPrincipal, clientID uuid.UUID, kind domain.ChannelType, value string) error {
	result, err := q.Exec(ctx, `UPDATE client_contact SET verified_at=now() WHERE account_id=$1 AND client_id=$2 AND kind=$3 AND value=$4 AND retired_at IS NULL`, p.AccountID, clientID, kind, value)
	if isUniqueViolation(err, "uq_verified_client_contact") {
		return domain.ErrConflict
	}
	if err == nil && result.RowsAffected() != 1 {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	result, err = q.Exec(ctx, `INSERT INTO customer_contact_ownership(account_id,customer_id,contact_id,includes_profile_history)
 SELECT c.account_id,i.id,c.id,(c.created_at<i.created_at OR profile.created_at>=i.created_at) FROM client_contact c
 JOIN client profile ON profile.account_id=c.account_id AND profile.id=c.client_id
 JOIN customer_identity i ON i.account_id=c.account_id AND i.id=$5 AND i.disabled_at IS NULL
 WHERE c.account_id=$1 AND c.client_id=$2 AND c.kind=$3 AND c.value=$4 AND c.retired_at IS NULL
 ON CONFLICT(account_id,customer_id,contact_id) DO UPDATE SET verified_at=now()`, p.AccountID, clientID, kind, value, p.CustomerID)
	if err == nil && result.RowsAffected() != 1 {
		return domain.ErrUnauthenticated
	}
	return err
}

// GetContactHostname reads only this supplier's configured portal hostname.
func (r *ClientContactRepository) GetContactHostname(ctx context.Context, q Querier, accountID uuid.UUID) (string, error) {
	var hostname string
	err := q.QueryRow(ctx, `SELECT public_hostname FROM account WHERE id=$1 AND is_active AND public_hostname IS NOT NULL`, accountID).Scan(&hostname)
	return hostname, err
}

// GetMessagingConfiguration selects an active supplier-owned channel without exposing its secrets.
func (r *ClientContactRepository) GetMessagingConfiguration(ctx context.Context, q Querier, accountID, branchID, channelID uuid.UUID, kind domain.ChannelType) (*domain.ChannelConfiguration, error) {
	c := &domain.ChannelConfiguration{}
	err := q.QueryRow(ctx, `SELECT c.id,c.branch_id,c.type,c.config FROM channel c JOIN branch b ON b.account_id=c.account_id AND b.id=c.branch_id AND b.is_active JOIN account a ON a.id=c.account_id AND a.is_active WHERE c.account_id=$1 AND c.type=$2 AND c.is_active AND c.config IS NOT NULL AND ($3::uuid='00000000-0000-0000-0000-000000000000' OR c.id=$3) AND ($4::uuid='00000000-0000-0000-0000-000000000000' OR c.branch_id=$4) ORDER BY c.id LIMIT 1`, accountID, kind, channelID, branchID).Scan(&c.ChannelID, &c.BranchID, &c.Type, &c.Config)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotConfigured
	}
	c.AccountID = accountID
	return c, err
}

// GrantProvisionalHistory retains the proved source's orders before its profile is merged.
func (r *ClientContactRepository) GrantProvisionalHistory(ctx context.Context, q Querier, p domain.CustomerPrincipal, sourceID uuid.UUID, kind domain.ChannelType, value string) error {
	_, err := q.Exec(ctx, `INSERT INTO customer_contact_history(account_id,customer_id,rfq_id,kind,value)
 SELECT r.account_id,$2,r.id,$4,$5 FROM rfq r LEFT JOIN quote qt ON qt.account_id=r.account_id AND qt.branch_id=r.branch_id AND qt.rfq_id=r.id
 WHERE r.account_id=$1 AND COALESCE(qt.client_id,r.client_id)=$3
 ON CONFLICT(account_id,customer_id,rfq_id,kind,value) DO NOTHING`, p.AccountID, p.CustomerID, sourceID, kind, value)
	return err
}

// LockDestination serializes proofs for one destination across every supplier until the transaction ends.
func (r *ClientContactRepository) LockDestination(ctx context.Context, q Querier, kind domain.ChannelType, value string) error {
	_, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('contact-destination:' || $1 || ':' || $2,0))`, string(kind), value)
	return err
}
