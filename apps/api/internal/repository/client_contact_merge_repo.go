package repository

import (
	"context"

	"github.com/google/uuid"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// mergeReach is true when no order of profile c sits outside branches $2 and, for seller $3, no quote
// is another seller's. An empty $2 reaches nothing, so a seller without branches merges nothing.
const mergeReach = `NOT EXISTS(SELECT 1 FROM rfq WHERE rfq.account_id=$1 AND rfq.client_id=c.id AND $2::uuid[] IS NOT NULL AND NOT(rfq.branch_id=ANY($2)))
 AND NOT EXISTS(SELECT 1 FROM quote WHERE quote.account_id=$1 AND quote.client_id=c.id AND (($2::uuid[] IS NOT NULL AND NOT(quote.branch_id=ANY($2))) OR ($3::uuid IS NOT NULL AND quote.seller_id IS NOT NULL AND quote.seller_id<>$3)))`

// registered is true when profile alias has a customer account: an active identity with its CUIT.
func registered(alias string) string {
	return `EXISTS(SELECT 1 FROM customer_identity i WHERE i.account_id=` + alias + `.account_id AND i.tax_id=` + alias + `.tax_id AND i.disabled_at IS NULL)`
}

// ListMergeCandidates returns the live profiles, other than clientID, that a merge with it would accept:
// two profiles without an account, never two CUITs, and a customer's account only beside a profile
// without a CUIT. Reach is checked on the side whose orders would move, never on the account.
func (r *ClientContactRepository) ListMergeCandidates(ctx context.Context, q Querier, tenant domain.Tenant, clientID uuid.UUID) ([]domain.Client, error) {
	rows, err := q.Query(ctx, `WITH cur AS (SELECT p.tax_id AS cur_tax_id,`+registered("p")+` AS cur_registered FROM client p WHERE p.account_id=$1 AND p.id=$4)
 SELECT `+clientColumns+` FROM client c,cur
 WHERE c.account_id=$1 AND c.id<>$4 AND c.merged_into IS NULL AND (
   NOT cur.cur_registered AND NOT `+registered("c")+` AND (c.tax_id IS NULL OR cur.cur_tax_id IS NULL) AND `+mergeReach+`
   OR NOT cur.cur_registered AND cur.cur_tax_id IS NULL AND `+registered("c")+`
   OR cur.cur_registered AND c.tax_id IS NULL AND `+mergeReach+`)
 ORDER BY lower(COALESCE(c.name,c.email,c.phone,'')),c.id`, tenant.AccountID, tenant.BranchFilter(), sellerParam(tenant), clientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.Client, 0)
	for rows.Next() {
		c, err := scanClient(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *c)
	}
	return result, rows.Err()
}

// LockMergeProfiles locks the given profiles of the caller's account and reports what a merge checks.
func (r *ClientContactRepository) LockMergeProfiles(ctx context.Context, q Querier, tenant domain.Tenant, ids []uuid.UUID) (map[uuid.UUID]domain.MergeProfile, error) {
	rows, err := q.Query(ctx, `SELECT c.id,c.tax_id,`+registered("c")+`,c.merged_into,`+mergeReach+` FROM client c
 WHERE c.account_id=$1 AND c.id=ANY($4) ORDER BY c.id FOR UPDATE OF c`, tenant.AccountID, tenant.BranchFilter(), sellerParam(tenant), ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[uuid.UUID]domain.MergeProfile, len(ids))
	for rows.Next() {
		var p domain.MergeProfile
		if err := rows.Scan(&p.ID, &p.TaxID, &p.Registered, &p.MergedInto, &p.InReach); err != nil {
			return nil, err
		}
		result[p.ID] = p
	}
	return result, rows.Err()
}

// MoveContacts retires the source's live contacts and gives the target a copy of each. Between two
// provisional profiles (keep) a copy keeps its channel verification, which still means the contact
// reached the supplier, and becomes primary where the target has none of that kind. Into a customer's
// account a copy arrives unverified and never primary: only the customer's own proof verifies there.
func (r *ClientContactRepository) MoveContacts(ctx context.Context, q Querier, accountID, source, target uuid.UUID, keep bool) error {
	_, err := q.Exec(ctx, `WITH retired AS (
   UPDATE client_contact SET retired_at=now(),is_main=FALSE WHERE account_id=$1 AND client_id=$2 AND retired_at IS NULL
   RETURNING account_id,kind,value,is_primary,source,original_value,verified_at)
 INSERT INTO client_contact(account_id,client_id,kind,value,is_primary,source,original_value,verified_at)
 SELECT s.account_id,$3,s.kind,s.value,$4 AND s.is_primary AND NOT EXISTS(SELECT 1 FROM client_contact t WHERE t.account_id=$1 AND t.client_id=$3 AND t.kind=s.kind AND t.is_primary),s.source,s.original_value,CASE WHEN $4 THEN s.verified_at END
 FROM retired s
 ON CONFLICT(account_id,client_id,kind,value) DO UPDATE SET verified_at=COALESCE(client_contact.verified_at,excluded.verified_at)`, accountID, source, target, keep)
	return err
}

// ConfirmCustomerProof lifts the portal hold of every seller merge into clientID whose merged profile
// held the contact the customer just proved, so those orders appear whole.
func (r *ClientContactRepository) ConfirmCustomerProof(ctx context.Context, q Querier, accountID, clientID uuid.UUID, kind domain.ChannelType, value string) error {
	_, err := q.Exec(ctx, `UPDATE client_merge_audit a SET customer_confirmed_at=now()
 WHERE a.account_id=$1 AND a.target_client_id=$2 AND a.requires_customer_proof AND a.customer_confirmed_at IS NULL
   AND EXISTS(SELECT 1 FROM client_contact c WHERE c.account_id=$1 AND c.client_id=a.source_client_id AND c.kind=$3 AND c.value=$4)`,
		accountID, clientID, kind, value)
	return err
}

// CompleteNameRequests settles every open name question sent to one of the profile's contacts, so a
// late reply cannot rename a profile whose name was already decided.
func (r *ClientContactRepository) CompleteNameRequests(ctx context.Context, q Querier, accountID, clientID uuid.UUID) error {
	_, err := q.Exec(ctx, `UPDATE client_contact_outbox o SET status='COMPLETED',status_changed_at=now()
 WHERE o.account_id=$1 AND o.purpose='REQUEST_NAME' AND o.status<>'COMPLETED'
   AND EXISTS(SELECT 1 FROM client_contact c WHERE c.account_id=$1 AND c.client_id=$2 AND c.kind=o.kind AND c.value=o.value)`, accountID, clientID)
	return err
}

// actorParam is the staff user behind a write, or nil when a customer's own proof made it.
func actorParam(tenant domain.Tenant) *uuid.UUID {
	if tenant.UserID == uuid.Nil {
		return nil
	}
	return &tenant.UserID
}
