package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

// ClientContactRepository persists commercial contacts and retained ingress observations.
type ClientContactRepository struct{}

// NewClientContactRepository builds the commercial contacts repository.
func NewClientContactRepository() *ClientContactRepository { return &ClientContactRepository{} }

// ListForCustomer projects verification performed by this credential rather than legacy contact trust.
// Unless all is set, it returns only the contacts this customer entered.
func (r *ClientContactRepository) ListForCustomer(ctx context.Context, q Querier, p domain.CustomerPrincipal, clientID uuid.UUID, all bool) ([]domain.ClientContact, error) {
	rows, err := q.Query(ctx, `SELECT c.id,c.kind,c.value,c.is_primary,EXISTS(SELECT 1 FROM customer_contact_ownership o WHERE o.account_id=c.account_id AND o.customer_id=$3 AND o.contact_id=c.id)
 FROM client_contact c WHERE c.account_id=$1 AND c.client_id=$2 AND c.retired_at IS NULL AND ($4 OR c.declared_by=$3) ORDER BY c.kind,c.is_primary DESC,c.created_at,c.id`, p.AccountID, clientID, p.CustomerID, all)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	contacts := []domain.ClientContact{}
	for rows.Next() {
		var c domain.ClientContact
		if err := rows.Scan(&c.ID, &c.Kind, &c.Value, &c.Primary, &c.Verified); err != nil {
			return nil, err
		}
		contacts = append(contacts, c)
	}
	return contacts, rows.Err()
}

// List returns contacts for a batch of account-owned profiles.
func (r *ClientContactRepository) List(ctx context.Context, q Querier, accountID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID][]domain.ClientContact, error) {
	rows, err := q.Query(ctx, `SELECT client_id,id,kind,value,is_primary,verified_at IS NOT NULL FROM client_contact WHERE account_id=$1 AND client_id=ANY($2) ORDER BY kind,is_primary DESC,created_at,id`, accountID, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[uuid.UUID][]domain.ClientContact)
	for rows.Next() {
		var id uuid.UUID
		var c domain.ClientContact
		if err := rows.Scan(&id, &c.ID, &c.Kind, &c.Value, &c.Primary, &c.Verified); err != nil {
			return nil, err
		}
		result[id] = append(result[id], c)
	}
	return result, rows.Err()
}

// GetProfileAccess reports whether a customer proved the profile and the name it entered.
func (r *ClientContactRepository) GetProfileAccess(ctx context.Context, q Querier, p domain.CustomerPrincipal, clientID uuid.UUID) (*domain.CustomerProfileAccess, error) {
	var access domain.CustomerProfileAccess
	err := q.QueryRow(ctx, `SELECT profile.created_at>=i.created_at OR EXISTS(
   SELECT 1 FROM customer_contact_ownership o JOIN client_contact c ON c.account_id=o.account_id AND c.id=o.contact_id
   WHERE o.account_id=$1 AND o.customer_id=$3 AND o.includes_profile_history AND c.client_id=profile.id AND c.verified_at IS NOT NULL AND c.retired_at IS NULL),
 i.name
 FROM client profile JOIN customer_identity i ON i.account_id=profile.account_id AND i.id=$3
 WHERE profile.account_id=$1 AND profile.id=$2`, p.AccountID, clientID, p.CustomerID).Scan(&access.Proved, &access.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &access, nil
}

// ListObservations reads pending suggestions within the staff caller's branch reach.
func (r *ClientContactRepository) ListObservations(ctx context.Context, q Querier, tenant domain.Tenant) ([]domain.ContactObservation, error) {
	rows, err := q.Query(ctx, `SELECT o.id,o.branch_id,o.source_client_id,o.candidate_client_id,c.name,o.tax_id,o.kind,o.value,o.declared_name,o.original_text,o.status,o.created_at FROM client_contact_observation o LEFT JOIN client c ON c.account_id=o.account_id AND c.id=o.candidate_client_id WHERE o.account_id=$1 AND ($2::uuid[] IS NULL OR o.branch_id=ANY($2)) AND o.status='PENDING' ORDER BY o.created_at,o.id`, tenant.AccountID, tenant.BranchFilter())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.ContactObservation, 0)
	for rows.Next() {
		var o domain.ContactObservation
		if err := rows.Scan(&o.ID, &o.BranchID, &o.SourceClientID, &o.CandidateClientID, &o.CandidateName, &o.TaxID, &o.Kind, &o.Value, &o.Name, &o.Original, &o.Status, &o.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, o)
	}
	return result, rows.Err()
}

// GetObservation locks one suggestion inside the staff caller's branch reach.
func (r *ClientContactRepository) GetObservation(ctx context.Context, q Querier, tenant domain.Tenant, id uuid.UUID) (*domain.ContactObservation, error) {
	var o domain.ContactObservation
	err := q.QueryRow(ctx, `SELECT id,branch_id,source_client_id,candidate_client_id,tax_id,kind,value,declared_name,original_text,status,created_at FROM client_contact_observation WHERE account_id=$1 AND id=$2 AND ($3::uuid[] IS NULL OR branch_id=ANY($3)) FOR UPDATE`, tenant.AccountID, id, tenant.BranchFilter()).Scan(&o.ID, &o.BranchID, &o.SourceClientID, &o.CandidateClientID, &o.TaxID, &o.Kind, &o.Value, &o.Name, &o.Original, &o.Status, &o.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return &o, err
}

// FindProfiles returns exact fiscal or contact candidates without associating records.
func (r *ClientContactRepository) FindProfiles(ctx context.Context, q Querier, accountID uuid.UUID, taxID string, kind domain.ChannelType, value string) ([]domain.Client, error) {
	rows, err := q.Query(ctx, `SELECT `+clientColumns+` FROM client WHERE account_id=$1 AND merged_into IS NULL AND (tax_id=NULLIF($2,'') OR id IN (SELECT client_id FROM client_contact WHERE account_id=$1 AND kind=$3 AND verified_at IS NOT NULL AND CASE WHEN kind='EMAIL' THEN lower(btrim(value)) ELSE regexp_replace(value,'[^0-9+]','','g') END=$4)) ORDER BY id`, accountID, taxID, kind, value)
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

// HasAssociations checks whether enriching a fiscal identifier could expose earlier requests.
func (r *ClientContactRepository) HasAssociations(ctx context.Context, q Querier, accountID, clientID uuid.UUID) (bool, error) {
	var found bool
	err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM rfq WHERE account_id=$1 AND client_id=$2) OR EXISTS(SELECT 1 FROM quote WHERE account_id=$1 AND client_id=$2)`, accountID, clientID).Scan(&found)
	return found, err
}

// HasPendingForTaxID reports only whether the authenticated customer's profile needs review.
func (r *ClientContactRepository) HasPendingForTaxID(ctx context.Context, q Querier, accountID uuid.UUID, taxID string) (bool, error) {
	var found bool
	err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM client_contact_observation WHERE account_id=$1 AND tax_id=$2 AND status='PENDING')`, accountID, taxID).Scan(&found)
	return found, err
}

// HasObservation prevents a replay from restoring a contact the customer later removed.
func (r *ClientContactRepository) HasObservation(ctx context.Context, q Querier, accountID, messageID uuid.UUID) (bool, error) {
	var found bool
	err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM client_contact_observation WHERE account_id=$1 AND message_id=$2)`, accountID, messageID).Scan(&found)
	return found, err
}

// PriorSeparation recognizes a seller's earlier decision for the same declared contact.
func (r *ClientContactRepository) PriorSeparation(ctx context.Context, q Querier, accountID uuid.UUID, taxID string, kind domain.ChannelType, value string) (bool, error) {
	var found bool
	err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM client_contact_observation WHERE account_id=$1 AND tax_id=$2 AND kind=$3 AND value=$4 AND status='SEPARATE')`, accountID, taxID, kind, value).Scan(&found)
	return found, err
}

// CreateObservation records the original declaration once per incoming contact and message.
func (r *ClientContactRepository) CreateObservation(ctx context.Context, q Querier, accountID, messageID uuid.UUID, o domain.ContactObservation) error {
	_, err := q.Exec(ctx, `INSERT INTO client_contact_observation(account_id,branch_id,message_id,source_client_id,candidate_client_id,tax_id,kind,value,declared_name,original_text,status) VALUES($1,$2,$3,$4,$5::uuid,$6::varchar(11),$7::channel_type,$8::varchar(255),$9,$10,CASE WHEN $11::client_contact_review_status='PENDING' AND EXISTS(SELECT 1 FROM client_contact_observation WHERE account_id=$1 AND branch_id=$2 AND candidate_client_id IS NOT DISTINCT FROM $5::uuid AND tax_id IS NOT DISTINCT FROM $6::varchar(11) AND kind=$7::channel_type AND value=$8::varchar(255) AND status='PENDING') THEN 'RECORDED'::client_contact_review_status ELSE $11::client_contact_review_status END) ON CONFLICT(account_id,message_id,kind,value) DO NOTHING`, accountID, o.BranchID, messageID, o.SourceClientID, o.CandidateClientID, o.TaxID, o.Kind, o.Value, o.Name, o.Original, o.Status)
	return err
}

// Save replaces a contact collection and synchronizes legacy primary projections atomically.
func (r *ClientContactRepository) Save(ctx context.Context, q Querier, accountID, clientID uuid.UUID, contacts []domain.ClientContact, source string) error {
	payload, err := contactPayload(contacts)
	if err != nil {
		return err
	}
	if _, err = q.Exec(ctx, `UPDATE client_contact SET is_primary=FALSE WHERE account_id=$1 AND client_id=$2`, accountID, clientID); err != nil {
		return err
	}
	if _, err = q.Exec(ctx, `DELETE FROM client_contact WHERE account_id=$1 AND client_id=$2 AND NOT EXISTS(SELECT 1 FROM jsonb_to_recordset($3::jsonb) AS x(kind text,value text,"primary" boolean) WHERE x.kind=client_contact.kind::text AND x.value=client_contact.value)`, accountID, clientID, payload); err != nil {
		return err
	}
	if _, err = q.Exec(ctx, `INSERT INTO client_contact(account_id,client_id,kind,value,is_primary,source,verified_at) SELECT $1,$2,x.kind::channel_type,x.value,x."primary",$4,CASE WHEN $4::client_contact_source='CUSTOMER' THEN NULL ELSE now() END FROM jsonb_to_recordset($3::jsonb) AS x(kind text,value text,"primary" boolean) ON CONFLICT(account_id,client_id,kind,value) DO UPDATE SET is_primary=excluded.is_primary`, accountID, clientID, payload, source); err != nil {
		return err
	}
	return projectPrimaryContacts(ctx, q, accountID, clientID)
}

// SaveDeclared replaces only the contacts this customer added to a profile it has not proved. The
// profile's earlier contacts are never removed, and a customer contact becomes primary only for a
// kind the profile had no other contact of.
func (r *ClientContactRepository) SaveDeclared(ctx context.Context, q Querier, p domain.CustomerPrincipal, clientID uuid.UUID, contacts []domain.ClientContact) error {
	payload, err := contactPayload(contacts)
	if err != nil {
		return err
	}
	var since time.Time
	if err = q.QueryRow(ctx, `SELECT created_at FROM customer_identity WHERE account_id=$1 AND id=$2`, p.AccountID, p.CustomerID).Scan(&since); err != nil {
		return err
	}
	const own = `declared_by=$3 AND source='CUSTOMER' AND created_at>=$4`
	const typed = `EXISTS(SELECT 1 FROM jsonb_to_recordset($5::jsonb) AS x(kind text,value text,"primary" boolean) WHERE x.kind=client_contact.kind::text AND x.value=client_contact.value)`
	args := []any{p.AccountID, clientID, p.CustomerID, since, payload}
	if _, err = q.Exec(ctx, `UPDATE client_contact SET declared_by=NULL WHERE account_id=$1 AND client_id=$2 AND declared_by=$3 AND NOT (`+own+`) AND NOT `+typed, args...); err != nil {
		return err
	}
	if _, err = q.Exec(ctx, `UPDATE client_contact SET is_primary=FALSE WHERE account_id=$1 AND client_id=$2 AND `+own, args[:4]...); err != nil {
		return err
	}
	if _, err = q.Exec(ctx, `DELETE FROM client_contact WHERE account_id=$1 AND client_id=$2 AND `+own+` AND NOT `+typed, args...); err != nil {
		return err
	}
	if _, err = q.Exec(ctx, `INSERT INTO client_contact(account_id,client_id,kind,value,is_primary,source,verified_at,declared_by) SELECT $1,$2,x.kind::channel_type,x.value,FALSE,'CUSTOMER',NULL,$3 FROM jsonb_to_recordset($4::jsonb) AS x(kind text,value text,"primary" boolean) ON CONFLICT(account_id,client_id,kind,value) DO UPDATE SET declared_by=excluded.declared_by`, p.AccountID, clientID, p.CustomerID, payload); err != nil {
		return err
	}
	if _, err = q.Exec(ctx, `UPDATE client_contact SET is_primary=TRUE WHERE account_id=$1 AND client_id=$2 AND `+own+` AND EXISTS(SELECT 1 FROM jsonb_to_recordset($5::jsonb) AS x(kind text,value text,"primary" boolean) WHERE x."primary" AND x.kind=client_contact.kind::text AND x.value=client_contact.value)
 AND NOT EXISTS(SELECT 1 FROM client_contact other WHERE other.account_id=$1 AND other.client_id=$2 AND other.kind=client_contact.kind AND NOT (other.declared_by IS NOT DISTINCT FROM $3 AND other.source='CUSTOMER' AND other.created_at>=$4))`, args...); err != nil {
		return err
	}
	return projectPrimaryContacts(ctx, q, p.AccountID, clientID)
}

// Declare marks the given contacts of a profile as entered by this customer.
func (r *ClientContactRepository) Declare(ctx context.Context, q Querier, p domain.CustomerPrincipal, clientID uuid.UUID, contacts []domain.ClientContact) error {
	payload, err := contactPayload(contacts)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `UPDATE client_contact SET declared_by=$3 WHERE account_id=$1 AND client_id=$2 AND EXISTS(SELECT 1 FROM jsonb_to_recordset($4::jsonb) AS x(kind text,value text,"primary" boolean) WHERE x.kind=client_contact.kind::text AND x.value=client_contact.value)`, p.AccountID, clientID, p.CustomerID, payload)
	return err
}

// UpdateName changes only the commercial name of an account-owned profile.
func (r *ClientContactRepository) UpdateName(ctx context.Context, q Querier, accountID, clientID uuid.UUID, name string) error {
	_, err := q.Exec(ctx, `UPDATE client SET name=NULLIF($3,'') WHERE account_id=$1 AND id=$2`, accountID, clientID, name)
	return err
}

// UpdateCustomerName keeps the name a customer entered on its own identity, leaving the profile's.
func (r *ClientContactRepository) UpdateCustomerName(ctx context.Context, q Querier, p domain.CustomerPrincipal, name string) error {
	_, err := q.Exec(ctx, `UPDATE customer_identity SET name=NULLIF($3,'') WHERE account_id=$1 AND id=$2`, p.AccountID, p.CustomerID, name)
	return err
}

// AttachSource retains the independently created profile behind a discarded match.
func (r *ClientContactRepository) AttachSource(ctx context.Context, q Querier, accountID, id, clientID uuid.UUID) error {
	_, err := q.Exec(ctx, `UPDATE client_contact_observation SET source_client_id=$3 WHERE account_id=$1 AND id=$2`, accountID, id, clientID)
	return err
}

// Decide retains the seller and outcome without deleting the original observation.
func (r *ClientContactRepository) Decide(ctx context.Context, q Querier, tenant domain.Tenant, id uuid.UUID, status string) error {
	_, err := q.Exec(ctx, `UPDATE client_contact_observation SET status=$3,decided_by=$4,decided_at=now() WHERE account_id=$1 AND id=$2 AND ($5::uuid[] IS NULL OR branch_id=ANY($5))`, tenant.AccountID, id, status, tenant.UserID, tenant.BranchFilter())
	return err
}

// Lock acquires account-scoped identity keys in a stable order within the caller's transaction.
func (r *ClientContactRepository) Lock(ctx context.Context, q Querier, accountID uuid.UUID, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	_, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1 || ':' || key,0)) FROM (SELECT DISTINCT unnest($2::text[]) AS key ORDER BY key) AS locks`, accountID.String(), keys)
	return err
}

// Merge preserves the source profile while moving its commercial associations to the target.
func (r *ClientContactRepository) Merge(ctx context.Context, q Querier, tenant domain.Tenant, source, target uuid.UUID) error {
	var allowed bool
	err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM client s JOIN client t ON t.account_id=s.account_id AND t.id=$3 WHERE s.account_id=$1 AND s.id=$2 AND s.id<>t.id AND s.merged_into IS NULL AND t.merged_into IS NULL AND s.tax_id IS NULL) AND NOT EXISTS(SELECT 1 FROM rfq WHERE account_id=$1 AND client_id=$2 AND ($4::uuid[] IS NOT NULL AND NOT(branch_id=ANY($4)))) AND NOT EXISTS(SELECT 1 FROM quote WHERE account_id=$1 AND client_id=$2 AND (($4::uuid[] IS NOT NULL AND NOT(branch_id=ANY($4))) OR ($5::uuid IS NOT NULL AND seller_id IS NOT NULL AND seller_id<>$5)))`, tenant.AccountID, source, target, tenant.BranchFilter(), sellerParam(tenant)).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrForbidden
	}
	if _, err = q.Exec(ctx, `UPDATE client_contact SET retired_at=now() WHERE account_id=$1 AND client_id=$2`, tenant.AccountID, source); err != nil {
		return err
	}
	if _, err = q.Exec(ctx, `INSERT INTO client_tag(account_id,client_id,tag_id) SELECT account_id,$3,tag_id FROM client_tag WHERE account_id=$1 AND client_id=$2 ON CONFLICT(client_id,tag_id) DO NOTHING`, tenant.AccountID, source, target); err != nil {
		return err
	}
	if _, err = q.Exec(ctx, `UPDATE rfq SET client_id=$3 WHERE account_id=$1 AND client_id=$2`, tenant.AccountID, source, target); err != nil {
		return err
	}
	if _, err = q.Exec(ctx, `UPDATE quote SET client_id=$3 WHERE account_id=$1 AND client_id=$2`, tenant.AccountID, source, target); err != nil {
		return err
	}
	if _, err = q.Exec(ctx, `UPDATE notification SET client_id=$3 WHERE account_id=$1 AND client_id=$2`, tenant.AccountID, source, target); err != nil {
		return err
	}
	_, err = q.Exec(ctx, `UPDATE client SET merged_into=$3 WHERE account_id=$1 AND id=$2`, tenant.AccountID, source, target)
	return err
}

func contactPayload(contacts []domain.ClientContact) ([]byte, error) {
	type wire struct {
		Kind    domain.ChannelType `json:"kind"`
		Value   string             `json:"value"`
		Primary bool               `json:"primary"`
	}
	data := make([]wire, 0, len(contacts))
	for _, c := range contacts {
		data = append(data, wire{c.Kind, c.Value, c.Primary})
	}
	return json.Marshal(data)
}

func projectPrimaryContacts(ctx context.Context, q Querier, accountID, clientID uuid.UUID) error {
	_, err := q.Exec(ctx, `UPDATE client SET phone=(SELECT value FROM client_contact WHERE account_id=$1 AND client_id=$2 AND kind='WHATSAPP' AND is_primary),email=(SELECT value FROM client_contact WHERE account_id=$1 AND client_id=$2 AND kind='EMAIL' AND is_primary) WHERE account_id=$1 AND id=$2`, accountID, clientID)
	return err
}
