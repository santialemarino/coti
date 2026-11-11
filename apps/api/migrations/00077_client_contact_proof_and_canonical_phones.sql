-- +goose Up
SELECT assert_migration_sees_all_rows('client','client_contact','client_contact_observation','customer_contact_ownership',
 'client_contact_outbox','client_contact_challenge','client_contact_challenge_failure','customer_contact_history','rfq','quote','branch');

-- A contact is verified only by proof: the channel a message arrived from, or a customer's own proof,
-- which always leaves an ownership row. A contact a seller typed proves nothing.
ALTER TABLE client_contact ALTER COLUMN verified_at DROP DEFAULT;
UPDATE client_contact c SET verified_at=NULL,is_main=FALSE
 WHERE c.verified_at IS NOT NULL AND c.source<>'CHANNEL'
   AND NOT EXISTS(SELECT 1 FROM customer_contact_ownership o WHERE o.account_id=c.account_id AND o.contact_id=c.id);

-- An Argentine number written without the 9 (+54 11…, +54 2xx…, +54 3xx…) takes the +549 form WhatsApp
-- reports, as domain.CanonicalPhone writes it.
CREATE TEMP TABLE canonical_contact ON COMMIT DROP AS
 SELECT id,account_id,client_id,value AS old_value,'+549'||substr(value,4) AS new_value
 FROM client_contact WHERE kind='WHATSAPP' AND value ~ '^\+54(11[0-9]{8}|[23][0-9]{9})$';

-- Two live rows that become one value may stay verified on one row only: the customer-proved one, else
-- the first verified.
WITH grouped AS (
 SELECT c.id,c.account_id,COALESCE(x.new_value,c.value) AS canonical,
  EXISTS(SELECT 1 FROM customer_contact_ownership o WHERE o.account_id=c.account_id AND o.contact_id=c.id) AS proved,c.verified_at
 FROM client_contact c LEFT JOIN canonical_contact x ON x.id=c.id
 WHERE c.kind='WHATSAPP' AND c.verified_at IS NOT NULL AND c.retired_at IS NULL
), ranked AS (
 SELECT id,row_number() OVER (PARTITION BY account_id,canonical ORDER BY proved DESC,verified_at,id) AS rank FROM grouped
 WHERE canonical IN (SELECT new_value FROM canonical_contact)
)
UPDATE client_contact c SET verified_at=NULL,is_main=FALSE FROM ranked r WHERE c.id=r.id AND r.rank>1;

-- A profile holding both forms keeps the +549 row, which takes over the other's proof, flags and ownership.
CREATE TEMP TABLE twin_contact ON COMMIT DROP AS
 SELECT x.id AS old_id,k.id AS kept_id FROM canonical_contact x
 JOIN client_contact k ON k.account_id=x.account_id AND k.client_id=x.client_id AND k.kind='WHATSAPP' AND k.value=x.new_value;
INSERT INTO customer_contact_ownership(account_id,customer_id,contact_id,includes_profile_history,verified_at)
 SELECT o.account_id,o.customer_id,t.kept_id,o.includes_profile_history,o.verified_at
 FROM customer_contact_ownership o JOIN twin_contact t ON t.old_id=o.contact_id
 ON CONFLICT(account_id,customer_id,contact_id) DO NOTHING;
CREATE TEMP TABLE twin_row ON COMMIT DROP AS
 SELECT t.kept_id,c.is_primary,c.is_main,c.verified_at,c.declared_by,c.original_value,c.created_at
 FROM twin_contact t JOIN client_contact c ON c.id=t.old_id;
DELETE FROM client_contact WHERE id IN (SELECT old_id FROM twin_contact);
UPDATE client_contact k SET is_primary=k.is_primary OR r.is_primary,is_main=k.is_main OR r.is_main,
  verified_at=COALESCE(k.verified_at,r.verified_at),declared_by=COALESCE(k.declared_by,r.declared_by),
  original_value=COALESCE(k.original_value,r.original_value),created_at=LEAST(k.created_at,r.created_at)
 FROM twin_row r WHERE k.id=r.kept_id;

UPDATE client_contact c SET value=x.new_value,original_value=COALESCE(c.original_value,x.old_value)
 FROM canonical_contact x WHERE c.id=x.id;

-- Profiles that now share a rewritten value go to seller review, as 00062 raised shared legacy
-- contacts: the verified holder, else the one with a CUIT, else the newest, is the candidate.
WITH holder AS (
 SELECT c.account_id,c.client_id,c.value,COALESCE(c.original_value,c.value) AS original_text,cl.name,
  first_value(c.client_id) OVER owners AS owner_id
 FROM client_contact c JOIN client cl ON cl.account_id=c.account_id AND cl.id=c.client_id
 WHERE c.kind='WHATSAPP' AND c.retired_at IS NULL AND cl.merged_into IS NULL
   AND EXISTS(SELECT 1 FROM canonical_contact x WHERE x.account_id=c.account_id AND x.new_value=c.value)
 WINDOW owners AS (PARTITION BY c.account_id,c.value ORDER BY c.verified_at IS NOT NULL DESC,cl.tax_id IS NOT NULL DESC,cl.created_at DESC,cl.id DESC
  ROWS BETWEEN UNBOUNDED PRECEDING AND UNBOUNDED FOLLOWING)
)
INSERT INTO client_contact_observation(account_id,branch_id,source_client_id,candidate_client_id,kind,value,declared_name,original_text,status)
SELECT account_id,branch_id,client_id,owner_id,'WHATSAPP',value,name,original_text,'PENDING' FROM (
 SELECT h.*,COALESCE(
  (SELECT branch_id FROM rfq WHERE account_id=h.account_id AND client_id=h.client_id ORDER BY created_at DESC,id DESC LIMIT 1),
  (SELECT branch_id FROM quote WHERE account_id=h.account_id AND client_id=h.client_id ORDER BY created_at DESC,id DESC LIMIT 1),
  (SELECT id FROM branch WHERE account_id=h.account_id ORDER BY created_at,id LIMIT 1)) AS branch_id
 FROM holder h WHERE h.client_id<>h.owner_id
   AND NOT EXISTS(SELECT 1 FROM client_contact_observation o WHERE o.account_id=h.account_id AND o.status='PENDING' AND o.kind='WHATSAPP' AND o.value=h.value
     AND ((o.source_client_id=h.client_id AND o.candidate_client_id=h.owner_id) OR (o.source_client_id=h.owner_id AND o.candidate_client_id=h.client_id)))
) shared WHERE branch_id IS NOT NULL;

-- Every other record of the destination follows; a row whose +549 twin already exists is left as is.
UPDATE client_contact_observation o SET value='+549'||substr(o.value,4)
 WHERE o.kind='WHATSAPP' AND o.value ~ '^\+54(11[0-9]{8}|[23][0-9]{9})$'
   AND NOT EXISTS(SELECT 1 FROM client_contact_observation t WHERE t.account_id=o.account_id AND t.message_id=o.message_id AND t.kind=o.kind AND t.value='+549'||substr(o.value,4));
UPDATE client_contact_outbox o SET value='+549'||substr(o.value,4)
 WHERE o.kind='WHATSAPP' AND o.value ~ '^\+54(11[0-9]{8}|[23][0-9]{9})$'
   AND NOT EXISTS(SELECT 1 FROM client_contact_outbox t WHERE t.account_id=o.account_id AND t.kind=o.kind AND t.purpose=o.purpose AND t.value='+549'||substr(o.value,4));
UPDATE customer_contact_history h SET value='+549'||substr(h.value,4)
 WHERE h.kind='WHATSAPP' AND h.value ~ '^\+54(11[0-9]{8}|[23][0-9]{9})$'
   AND NOT EXISTS(SELECT 1 FROM customer_contact_history t WHERE t.account_id=h.account_id AND t.customer_id=h.customer_id AND t.rfq_id=h.rfq_id AND t.kind=h.kind AND t.value='+549'||substr(h.value,4));
UPDATE client_contact_challenge SET value='+549'||substr(value,4) WHERE kind='WHATSAPP' AND value ~ '^\+54(11[0-9]{8}|[23][0-9]{9})$';
UPDATE client_contact_challenge_failure SET value='+549'||substr(value,4) WHERE kind='WHATSAPP' AND value ~ '^\+54(11[0-9]{8}|[23][0-9]{9})$';
UPDATE client SET phone='+549'||substr(regexp_replace(phone,'[^0-9+]','','g'),4)
 WHERE regexp_replace(phone,'[^0-9+]','','g') ~ '^\+54(11[0-9]{8}|[23][0-9]{9})$';

DROP INDEX idx_client_account_tax_id;

-- +goose Down
-- The data rewrite is one-way: contacts stay unverified without proof, phones keep the +549 form and
-- the raised reviews stay. Both are valid under the previous schema.
CREATE INDEX idx_client_account_tax_id ON client(account_id,tax_id) WHERE tax_id IS NOT NULL;
ALTER TABLE client_contact ALTER COLUMN verified_at SET DEFAULT now();
