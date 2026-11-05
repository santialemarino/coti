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

// ListMergeCandidates returns the live profiles without a CUIT, other than clientID, that the caller
// could merge.
func (r *ClientContactRepository) ListMergeCandidates(ctx context.Context, q Querier, tenant domain.Tenant, clientID uuid.UUID) ([]domain.Client, error) {
	rows, err := q.Query(ctx, `SELECT `+clientColumns+` FROM client c
 WHERE c.account_id=$1 AND c.id<>$4 AND c.merged_into IS NULL AND c.tax_id IS NULL AND `+mergeReach+`
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
	rows, err := q.Query(ctx, `SELECT c.id,c.tax_id,c.merged_into,`+mergeReach+` FROM client c
 WHERE c.account_id=$1 AND c.id=ANY($4) ORDER BY c.id FOR UPDATE`, tenant.AccountID, tenant.BranchFilter(), sellerParam(tenant), ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[uuid.UUID]domain.MergeProfile, len(ids))
	for rows.Next() {
		var p domain.MergeProfile
		if err := rows.Scan(&p.ID, &p.TaxID, &p.MergedInto, &p.InReach); err != nil {
			return nil, err
		}
		result[p.ID] = p
	}
	return result, rows.Err()
}

// MoveContacts retires the source's live contacts and gives the target a copy of each, primary only
// where the target has none of that kind. A copy keeps its channel verification: between two
// provisional profiles it still means the contact reached the supplier, and retiring the original
// first frees the one live verified row per contact.
func (r *ClientContactRepository) MoveContacts(ctx context.Context, q Querier, accountID, source, target uuid.UUID) error {
	_, err := q.Exec(ctx, `WITH retired AS (
   UPDATE client_contact SET retired_at=now(),is_main=FALSE WHERE account_id=$1 AND client_id=$2 AND retired_at IS NULL
   RETURNING account_id,kind,value,is_primary,source,original_value,verified_at)
 INSERT INTO client_contact(account_id,client_id,kind,value,is_primary,source,original_value,verified_at)
 SELECT s.account_id,$3,s.kind,s.value,s.is_primary AND NOT EXISTS(SELECT 1 FROM client_contact t WHERE t.account_id=$1 AND t.client_id=$3 AND t.kind=s.kind AND t.is_primary),s.source,s.original_value,s.verified_at
 FROM retired s
 ON CONFLICT(account_id,client_id,kind,value) DO UPDATE SET verified_at=COALESCE(client_contact.verified_at,excluded.verified_at)`, accountID, source, target)
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
