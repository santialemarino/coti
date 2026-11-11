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
// Unless all is set, it returns only the contacts this customer entered, ordered by value so neither
// order nor flags tell which of them the profile already had.
func (r *ClientContactRepository) ListForCustomer(ctx context.Context, q Querier, p domain.CustomerPrincipal, clientID uuid.UUID, all bool) ([]domain.ClientContact, error) {
	rows, err := q.Query(ctx, `SELECT c.id,c.kind,c.value,c.is_primary,c.id IS NOT DISTINCT FROM `+mainContact+`,EXISTS(SELECT 1 FROM customer_contact_ownership o WHERE o.account_id=c.account_id AND o.customer_id=$3 AND o.contact_id=c.id)
 FROM client_contact c WHERE c.account_id=$1 AND c.client_id=$2 AND c.retired_at IS NULL AND ($4 OR c.declared_by=$3)
 ORDER BY c.kind,($4 AND c.is_primary) DESC,CASE WHEN $4 THEN c.created_at END,c.value,c.id`, p.AccountID, clientID, p.CustomerID, all)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	contacts := []domain.ClientContact{}
	for rows.Next() {
		var c domain.ClientContact
		if err := rows.Scan(&c.ID, &c.Kind, &c.Value, &c.Primary, &c.Main, &c.Verified); err != nil {
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
	rows, err := q.Query(ctx, `SELECT o.id,o.branch_id,o.source_client_id,o.candidate_client_id,c.name,s.name,`+registered("s")+`,`+registered("c")+`,
 (`+registered("s")+`)<>(`+registered("c")+`) AND EXISTS(SELECT 1 FROM client r JOIN client_contact v ON v.account_id=r.account_id AND v.client_id=r.id
   JOIN customer_contact_ownership w ON w.account_id=v.account_id AND w.contact_id=v.id
   WHERE r.account_id=o.account_id AND r.id IN (o.source_client_id,o.candidate_client_id) AND `+registered("r")+`
     AND v.kind=o.kind AND v.value=o.value AND v.verified_at IS NOT NULL AND v.retired_at IS NULL),
 o.tax_id,o.kind,o.value,o.declared_name,o.original_text,o.status,o.created_at
 FROM client_contact_observation o
 LEFT JOIN client c ON c.account_id=o.account_id AND c.id=o.candidate_client_id
 LEFT JOIN client s ON s.account_id=o.account_id AND s.id=o.source_client_id
 WHERE o.account_id=$1 AND ($2::uuid[] IS NULL OR o.branch_id=ANY($2)) AND o.status='PENDING' ORDER BY o.created_at,o.id`, tenant.AccountID, tenant.BranchFilter())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.ContactObservation, 0)
	for rows.Next() {
		var o domain.ContactObservation
		if err := rows.Scan(&o.ID, &o.BranchID, &o.SourceClientID, &o.CandidateClientID, &o.CandidateName, &o.SourceName, &o.SourceRegistered, &o.CandidateRegistered, &o.AccountJoinReady, &o.TaxID, &o.Kind, &o.Value, &o.Name, &o.Original, &o.Status, &o.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, o)
	}
	return result, rows.Err()
}

// GetObservation locks one suggestion inside the staff caller's branch reach.
func (r *ClientContactRepository) GetObservation(ctx context.Context, q Querier, tenant domain.Tenant, id uuid.UUID) (*domain.ContactObservation, error) {
	var o domain.ContactObservation
	err := q.QueryRow(ctx, `SELECT id,branch_id,source_client_id,candidate_client_id,tax_id,kind,value,declared_name,original_text,status,created_at
 FROM client_contact_observation WHERE account_id=$1 AND id=$2 AND ($3::uuid[] IS NULL OR branch_id=ANY($3)) FOR UPDATE`, tenant.AccountID, id, tenant.BranchFilter()).Scan(&o.ID, &o.BranchID, &o.SourceClientID, &o.CandidateClientID, &o.TaxID, &o.Kind, &o.Value, &o.Name, &o.Original, &o.Status, &o.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	return &o, err
}

// FindProfiles returns exact fiscal or contact candidates without associating records.
func (r *ClientContactRepository) FindProfiles(ctx context.Context, q Querier, accountID uuid.UUID, taxID string, kind domain.ChannelType, value string) ([]domain.Client, error) {
	rows, err := q.Query(ctx, `SELECT `+clientColumns+` FROM client
 WHERE account_id=$1 AND merged_into IS NULL AND (tax_id=NULLIF($2,'') OR id IN (
   SELECT client_id FROM client_contact WHERE account_id=$1 AND kind=$3 AND value=$4 AND verified_at IS NOT NULL))
 ORDER BY id`, accountID, taxID, kind, value)
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

// FindMergedRoute returns the profile a merged provisional profile's verified contact still routes to,
// while that profile keeps the unverified copy the merge gave it. The latest merge wins.
func (r *ClientContactRepository) FindMergedRoute(ctx context.Context, q Querier, accountID uuid.UUID, kind domain.ChannelType, value string) ([]domain.Client, error) {
	rows, err := q.Query(ctx, `SELECT `+clientColumns+` FROM client WHERE account_id=$1 AND merged_into IS NULL AND id=(
 SELECT source.merged_into FROM client_contact retired JOIN client source ON source.account_id=retired.account_id AND source.id=retired.client_id
 WHERE retired.account_id=$1 AND retired.kind=$2 AND retired.value=$3 AND retired.verified_at IS NOT NULL AND retired.retired_at IS NOT NULL AND source.merged_into IS NOT NULL
   AND EXISTS(SELECT 1 FROM client_contact kept WHERE kept.account_id=$1 AND kept.client_id=source.merged_into AND kept.kind=$2 AND kept.value=$3 AND kept.retired_at IS NULL)
 ORDER BY retired.retired_at DESC,retired.id LIMIT 1)`, accountID, kind, value)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.Client, 0, 1)
	for rows.Next() {
		c, err := scanClient(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *c)
	}
	return result, rows.Err()
}

// GetConversationClient returns the live profile whose quote the inbound message joined, when that
// profile holds the sender's contact: the quote went there, so the reply proves the contact.
func (r *ClientContactRepository) GetConversationClient(ctx context.Context, q Querier, accountID, messageID uuid.UUID, kind domain.ChannelType, value string) (*domain.Client, error) {
	client, err := scanClient(q.QueryRow(ctx, `SELECT `+clientColumns+` FROM client WHERE account_id=$1 AND merged_into IS NULL AND id=(
   SELECT qt.client_id FROM inbound_channel_message m
   JOIN quote_message qm ON qm.account_id=m.account_id AND qm.id=m.quote_message_id
   JOIN quote qt ON qt.account_id=qm.account_id AND qt.id=qm.quote_id
   WHERE m.account_id=$1 AND m.id=$2)
 AND EXISTS(SELECT 1 FROM client_contact c WHERE c.account_id=$1 AND c.client_id=client.id AND c.kind=$3 AND c.value=$4 AND c.retired_at IS NULL)`, accountID, messageID, kind, value))
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	return client, err
}

// GetContactOwner returns the active customer who proved the profile's live contact, or nil.
func (r *ClientContactRepository) GetContactOwner(ctx context.Context, q Querier, accountID, clientID uuid.UUID, kind domain.ChannelType, value string) (*uuid.UUID, error) {
	var id uuid.UUID
	err := q.QueryRow(ctx, `SELECT w.customer_id FROM client_contact c
 JOIN customer_contact_ownership w ON w.account_id=c.account_id AND w.contact_id=c.id
 JOIN customer_identity i ON i.account_id=w.account_id AND i.id=w.customer_id AND i.disabled_at IS NULL
 WHERE c.account_id=$1 AND c.client_id=$2 AND c.kind=$3 AND c.value=$4 AND c.verified_at IS NOT NULL AND c.retired_at IS NULL
 ORDER BY w.verified_at,w.id LIMIT 1`, accountID, clientID, kind, value).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// HasObservation prevents a replay from restoring a contact the customer later removed.
func (r *ClientContactRepository) HasObservation(ctx context.Context, q Querier, accountID, messageID uuid.UUID) (bool, error) {
	var found bool
	err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM client_contact_observation WHERE account_id=$1 AND message_id=$2)`, accountID, messageID).Scan(&found)
	return found, err
}

// CreateObservation records the original declaration once per incoming contact and message.
func (r *ClientContactRepository) CreateObservation(ctx context.Context, q Querier, accountID, messageID uuid.UUID, o domain.ContactObservation) error {
	_, err := q.Exec(ctx, `INSERT INTO client_contact_observation(account_id,branch_id,message_id,source_client_id,candidate_client_id,tax_id,kind,value,declared_name,original_text,status)
 VALUES($1,$2,$3,$4,$5::uuid,$6::varchar(11),$7::channel_type,$8::varchar(255),$9,$10,CASE
   WHEN $11::client_contact_review_status='PENDING' AND EXISTS(
     SELECT 1 FROM client_contact_observation WHERE account_id=$1 AND branch_id=$2 AND candidate_client_id IS NOT DISTINCT FROM $5::uuid
       AND tax_id IS NOT DISTINCT FROM $6::varchar(11) AND kind=$7::channel_type AND value=$8::varchar(255) AND status='PENDING')
   THEN 'RECORDED'::client_contact_review_status ELSE $11::client_contact_review_status END)
 ON CONFLICT(account_id,message_id,kind,value) DO NOTHING`, accountID, o.BranchID, messageID, o.SourceClientID, o.CandidateClientID, o.TaxID, o.Kind, o.Value, o.Name, o.Original, o.Status)
	return err
}

// CreateMatchObservations raises a PENDING case between profile source and every other live profile
// holding the contact, verified or not, unless one is already pending for that pair on any contact.
func (r *ClientContactRepository) CreateMatchObservations(ctx context.Context, q Querier, accountID, branchID, source uuid.UUID, kind domain.ChannelType, value, original string) error {
	return createMatchObservations(ctx, q, accountID, branchID, source, kind, value, original)
}

// CreateMatchObservations raises the seller's review of a contact a new profile shares with others.
func (r *ClientRepository) CreateMatchObservations(ctx context.Context, q Querier, accountID, branchID, source uuid.UUID, kind domain.ChannelType, value, original string) error {
	return createMatchObservations(ctx, q, accountID, branchID, source, kind, value, original)
}

func createMatchObservations(ctx context.Context, q Querier, accountID, branchID, source uuid.UUID, kind domain.ChannelType, value, original string) error {
	_, err := q.Exec(ctx, `INSERT INTO client_contact_observation(account_id,branch_id,source_client_id,candidate_client_id,kind,value,original_text,status)
 SELECT DISTINCT $1::uuid,$2::uuid,$3::uuid,c.client_id,$4::channel_type,$5::varchar(255),$6::text,'PENDING'::client_contact_review_status
 FROM client_contact c JOIN client p ON p.account_id=c.account_id AND p.id=c.client_id
 WHERE c.account_id=$1 AND c.kind=$4 AND c.value=$5 AND c.retired_at IS NULL AND c.client_id<>$3 AND p.merged_into IS NULL
   AND NOT EXISTS(SELECT 1 FROM client_contact_observation o WHERE o.account_id=$1 AND o.status='PENDING'
     AND ((o.source_client_id=$3 AND o.candidate_client_id=c.client_id) OR (o.source_client_id=c.client_id AND o.candidate_client_id=$3)))`,
		accountID, branchID, source, kind, value, original)
	return err
}

// CreateProofMatchObservations raises a PENDING case from every live profile without a customer
// account that holds the contact unverified to the registered profile whose customer just proved it,
// filed under the branch that last served the provisional profile.
func (r *ClientContactRepository) CreateProofMatchObservations(ctx context.Context, q Querier, accountID, registeredID uuid.UUID, kind domain.ChannelType, value string) error {
	_, err := q.Exec(ctx, `INSERT INTO client_contact_observation(account_id,branch_id,source_client_id,candidate_client_id,kind,value,original_text,status)
 SELECT $1::uuid,branch_id,id,$2::uuid,$3::channel_type,$4::varchar(255),$4::text,'PENDING'::client_contact_review_status FROM (
  SELECT p.id,COALESCE(
   (SELECT branch_id FROM rfq WHERE account_id=$1 AND client_id=p.id ORDER BY created_at DESC,id DESC LIMIT 1),
   (SELECT branch_id FROM quote WHERE account_id=$1 AND client_id=p.id ORDER BY created_at DESC,id DESC LIMIT 1),
   (SELECT id FROM branch WHERE account_id=$1 ORDER BY created_at,id LIMIT 1)) AS branch_id
  FROM client p WHERE p.account_id=$1 AND p.id<>$2 AND p.merged_into IS NULL AND NOT `+registered("p")+`
   AND EXISTS(SELECT 1 FROM client_contact c WHERE c.account_id=$1 AND c.client_id=p.id AND c.kind=$3 AND c.value=$4 AND c.verified_at IS NULL AND c.retired_at IS NULL)
   AND NOT EXISTS(SELECT 1 FROM client_contact_observation o WHERE o.account_id=$1 AND o.status='PENDING'
     AND ((o.source_client_id=p.id AND o.candidate_client_id=$2) OR (o.source_client_id=$2 AND o.candidate_client_id=p.id)))
 ) provisional WHERE branch_id IS NOT NULL`, accountID, registeredID, kind, value)
	return err
}

// MarkChannelVerified verifies a profile's live contact that a message from it just proved.
func (r *ClientContactRepository) MarkChannelVerified(ctx context.Context, q Querier, accountID, clientID uuid.UUID, kind domain.ChannelType, value string) error {
	_, err := q.Exec(ctx, `UPDATE client_contact SET verified_at=now() WHERE account_id=$1 AND client_id=$2 AND kind=$3 AND value=$4 AND retired_at IS NULL AND verified_at IS NULL`, accountID, clientID, kind, value)
	return err
}

// Save replaces a contact collection; ProjectPrimaries then copies its primaries to client.phone/email.
// Only a CHANNEL contact, the sender a message arrived from, is saved verified.
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
	_, err = q.Exec(ctx, `INSERT INTO client_contact(account_id,client_id,kind,value,is_primary,source,verified_at)
 SELECT $1,$2,x.kind::channel_type,x.value,x."primary",$4,CASE WHEN $4::client_contact_source='CHANNEL' THEN now() END
 FROM jsonb_to_recordset($3::jsonb) AS x(kind text,value text,"primary" boolean)
 ON CONFLICT(account_id,client_id,kind,value) DO UPDATE SET is_primary=excluded.is_primary`, accountID, clientID, payload, source)
	return err
}

// ProjectPrimaries copies a profile's primary contacts to client.phone/email, which sellers quote and
// route through. It is never called for a profile its customer has not proved.
func (r *ClientContactRepository) ProjectPrimaries(ctx context.Context, q Querier, accountID, clientID uuid.UUID) error {
	_, err := q.Exec(ctx, `UPDATE client SET phone=(SELECT value FROM client_contact WHERE account_id=$1 AND client_id=$2 AND kind='WHATSAPP' AND is_primary),email=(SELECT value FROM client_contact WHERE account_id=$1 AND client_id=$2 AND kind='EMAIL' AND is_primary) WHERE account_id=$1 AND id=$2`, accountID, clientID)
	return err
}

// SaveDeclared replaces only the contacts this customer added to a profile it has not proved. The
// profile's earlier contacts are never removed, and its primaries and projected phone/email never move.
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
	if _, err = q.Exec(ctx, `DELETE FROM client_contact WHERE account_id=$1 AND client_id=$2 AND `+own+` AND NOT `+typed, args...); err != nil {
		return err
	}
	_, err = q.Exec(ctx, `INSERT INTO client_contact(account_id,client_id,kind,value,is_primary,source,verified_at,declared_by)
 SELECT $1,$2,x.kind::channel_type,x.value,FALSE,'CUSTOMER',NULL,$3 FROM jsonb_to_recordset($4::jsonb) AS x(kind text,value text,"primary" boolean)
 ON CONFLICT(account_id,client_id,kind,value) DO UPDATE SET declared_by=excluded.declared_by`, p.AccountID, clientID, p.CustomerID, payload)
	return err
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

// Decide retains the seller and outcome without deleting the original observation.
func (r *ClientContactRepository) Decide(ctx context.Context, q Querier, tenant domain.Tenant, id uuid.UUID, status string) error {
	_, err := q.Exec(ctx, `UPDATE client_contact_observation SET status=$3,decided_by=$4,decided_at=now() WHERE account_id=$1 AND id=$2 AND ($5::uuid[] IS NULL OR branch_id=ANY($5))`, tenant.AccountID, id, status, tenant.UserID, tenant.BranchFilter())
	return err
}

// mainContact selects the profile's main contact among those customer $3 proved on profile $2 of
// account $1: the one the customer chose, else the first WhatsApp, else the first contact.
const mainContact = `(SELECT m.id FROM client_contact m JOIN customer_contact_ownership mo ON mo.account_id=m.account_id AND mo.customer_id=$3 AND mo.contact_id=m.id
 WHERE m.account_id=$1 AND m.client_id=$2 AND m.verified_at IS NOT NULL AND m.retired_at IS NULL
 ORDER BY m.is_main DESC,m.kind='WHATSAPP' DESC,m.created_at,m.id LIMIT 1)`

// SetMain records the customer's choice of main contact, provided the customer proved it.
func (r *ClientContactRepository) SetMain(ctx context.Context, q Querier, p domain.CustomerPrincipal, clientID uuid.UUID, kind domain.ChannelType, value string) error {
	var id uuid.UUID
	err := q.QueryRow(ctx, `SELECT c.id FROM client_contact c JOIN customer_contact_ownership o ON o.account_id=c.account_id AND o.customer_id=$3 AND o.contact_id=c.id
 WHERE c.account_id=$1 AND c.client_id=$2 AND c.kind=$4 AND c.value=$5 AND c.verified_at IS NOT NULL AND c.retired_at IS NULL`, p.AccountID, clientID, p.CustomerID, kind, value).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err = q.Exec(ctx, `UPDATE client_contact SET is_main=FALSE WHERE account_id=$1 AND client_id=$2 AND is_main AND id<>$3`, p.AccountID, clientID, id); err != nil {
		return err
	}
	_, err = q.Exec(ctx, `UPDATE client_contact SET is_main=TRUE WHERE account_id=$1 AND id=$2`, p.AccountID, id)
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
	err := q.QueryRow(ctx, `SELECT EXISTS(
   SELECT 1 FROM client s JOIN client t ON t.account_id=s.account_id AND t.id=$3
   WHERE s.account_id=$1 AND s.id=$2 AND s.id<>t.id AND s.merged_into IS NULL AND t.merged_into IS NULL AND s.tax_id IS NULL)
 AND NOT EXISTS(SELECT 1 FROM rfq WHERE account_id=$1 AND client_id=$2 AND ($4::uuid[] IS NOT NULL AND NOT(branch_id=ANY($4))))
 AND NOT EXISTS(SELECT 1 FROM quote WHERE account_id=$1 AND client_id=$2
   AND (($4::uuid[] IS NOT NULL AND NOT(branch_id=ANY($4))) OR ($5::uuid IS NOT NULL AND seller_id IS NOT NULL AND seller_id<>$5)))`, tenant.AccountID, source, target, tenant.BranchFilter(), sellerParam(tenant)).Scan(&allowed)
	if err != nil {
		return err
	}
	if !allowed {
		return domain.ErrForbidden
	}
	if err = r.CompleteNameRequests(ctx, q, tenant.AccountID, source); err != nil {
		return err
	}
	if _, err = q.Exec(ctx, `UPDATE client_contact SET retired_at=now(),is_main=FALSE WHERE account_id=$1 AND client_id=$2`, tenant.AccountID, source); err != nil {
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
	// The DNI and fiscal data fill only what the target lacks; nothing it already holds is overwritten.
	// Both profiles' notes are kept, the source's after the target's, within the column's 512.
	if _, err = q.Exec(ctx, `UPDATE client t SET notes=left(NULLIF(concat_ws(E'\n\n',NULLIF(btrim(t.notes),''),NULLIF(btrim(s.notes),'')),''),512),dni=COALESCE(t.dni,s.dni),legal_name=COALESCE(t.legal_name,s.legal_name),iva_condition=COALESCE(t.iva_condition,s.iva_condition),fiscal_address=COALESCE(t.fiscal_address,s.fiscal_address)
 FROM client s WHERE t.account_id=$1 AND t.id=$3 AND s.account_id=$1 AND s.id=$2`, tenant.AccountID, source, target); err != nil {
		return err
	}
	// Profiles merged into the source follow it, so a merged profile always names a live survivor.
	if _, err = q.Exec(ctx, `UPDATE client SET merged_into=$3 WHERE account_id=$1 AND (id=$2 OR merged_into=$2)`, tenant.AccountID, source, target); err != nil {
		return err
	}
	_, err = q.Exec(ctx, `INSERT INTO client_merge_audit(account_id,source_client_id,target_client_id,merged_by) VALUES($1,$2,$3,$4)`, tenant.AccountID, source, target, actorParam(tenant))
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
