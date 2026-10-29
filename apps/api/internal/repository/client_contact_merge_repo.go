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

// CopyContacts gives the target an unverified copy of each live source contact it lacks, primary only
// where the target has none of that kind. The retired verified original keeps routing to the target.
func (r *ClientContactRepository) CopyContacts(ctx context.Context, q Querier, accountID, source, target uuid.UUID) error {
	_, err := q.Exec(ctx, `INSERT INTO client_contact(account_id,client_id,kind,value,is_primary,source,original_value,verified_at)
 SELECT s.account_id,$3,s.kind,s.value,s.is_primary AND NOT EXISTS(SELECT 1 FROM client_contact t WHERE t.account_id=$1 AND t.client_id=$3 AND t.kind=s.kind AND t.is_primary),s.source,s.original_value,NULL
 FROM client_contact s WHERE s.account_id=$1 AND s.client_id=$2 AND s.retired_at IS NULL
 ON CONFLICT(account_id,client_id,kind,value) DO NOTHING`, accountID, source, target)
	return err
}

// actorParam is the staff user behind a write, or nil when a customer's own proof made it.
func actorParam(tenant domain.Tenant) *uuid.UUID {
	if tenant.UserID == uuid.Nil {
		return nil
	}
	return &tenant.UserID
}
