-- +goose Up
-- The destination counts read every supplier's rows, so the account they report on must be the caller's.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION contact_destination_activity(p_account_id UUID, p_kind channel_type, p_value TEXT, p_challenges_since TIMESTAMPTZ, p_failures_since TIMESTAMPTZ, p_all_since TIMESTAMPTZ)
 RETURNS TABLE(supplier_challenges INTEGER, supplier_failures INTEGER, all_challenges INTEGER)
 LANGUAGE plpgsql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
BEGIN
 IF p_account_id IS DISTINCT FROM app_current_account_id() THEN
  RAISE EXCEPTION 'Destination activity is readable only for the current account' USING ERRCODE='42501';
 END IF;
 RETURN QUERY SELECT
  (SELECT count(*)::int FROM client_contact_challenge c WHERE c.account_id=p_account_id AND c.kind=p_kind AND c.value=p_value AND c.created_at>p_challenges_since),
  (SELECT count(*)::int FROM client_contact_challenge_failure f WHERE f.account_id=p_account_id AND f.kind=p_kind AND f.value=p_value AND f.created_at>p_failures_since),
  (SELECT count(*)::int FROM client_contact_challenge c WHERE c.kind=p_kind AND c.value=p_value AND c.created_at>p_all_since);
END;
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION contact_destination_activity(UUID,channel_type,TEXT,TIMESTAMPTZ,TIMESTAMPTZ,TIMESTAMPTZ) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION contact_destination_activity(UUID,channel_type,TEXT,TIMESTAMPTZ,TIMESTAMPTZ,TIMESTAMPTZ) TO coti_app;

-- A portal request's observation stored the request id, which is its message's external id: it is
-- pointed at that message. One whose message is gone, or another account's, keeps the review but not
-- the link.
SELECT assert_migration_sees_all_rows('client_contact_observation','inbound_channel_message');
UPDATE client_contact_observation o SET message_id=m.id FROM inbound_channel_message m
 WHERE o.message_id IS NOT NULL AND m.account_id=o.account_id AND m.branch_id=o.branch_id AND m.external_message_id=o.message_id::text
   AND NOT EXISTS(SELECT 1 FROM inbound_channel_message x WHERE x.account_id=o.account_id AND x.id=o.message_id);
UPDATE client_contact_observation o SET message_id=NULL
 WHERE o.message_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM inbound_channel_message m WHERE m.account_id=o.account_id AND m.id=o.message_id);
ALTER TABLE inbound_channel_message ADD CONSTRAINT uq_inbound_channel_message_account UNIQUE(account_id,id);
ALTER TABLE client_contact_observation ADD CONSTRAINT fk_client_contact_observation_message
 FOREIGN KEY(account_id,message_id) REFERENCES inbound_channel_message(account_id,id);

-- A registered profile is its customer's; only the schema owner, which runs migrations and teardown,
-- may delete one.
-- +goose StatementBegin
CREATE FUNCTION protect_registered_client() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.tax_id IS NOT NULL AND NOT pg_has_role(current_user,(SELECT relowner FROM pg_class WHERE oid=TG_RELID),'USAGE') THEN
  RAISE EXCEPTION 'A client with a CUIT cannot be deleted' USING ERRCODE='23514';
 END IF;
 RETURN OLD;
END;
$$;
-- +goose StatementEnd
CREATE TRIGGER protect_registered_client BEFORE DELETE ON client FOR EACH ROW EXECUTE FUNCTION protect_registered_client();

-- +goose Down
DROP TRIGGER protect_registered_client ON client;
DROP FUNCTION protect_registered_client();
ALTER TABLE client_contact_observation DROP CONSTRAINT fk_client_contact_observation_message;
ALTER TABLE inbound_channel_message DROP CONSTRAINT uq_inbound_channel_message_account;
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION contact_destination_activity(p_account_id UUID, p_kind channel_type, p_value TEXT, p_challenges_since TIMESTAMPTZ, p_failures_since TIMESTAMPTZ, p_all_since TIMESTAMPTZ)
 RETURNS TABLE(supplier_challenges INTEGER, supplier_failures INTEGER, all_challenges INTEGER)
 LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
 SELECT (SELECT count(*)::int FROM client_contact_challenge WHERE account_id=p_account_id AND kind=p_kind AND value=p_value AND created_at>p_challenges_since),
        (SELECT count(*)::int FROM client_contact_challenge_failure WHERE account_id=p_account_id AND kind=p_kind AND value=p_value AND created_at>p_failures_since),
        (SELECT count(*)::int FROM client_contact_challenge WHERE kind=p_kind AND value=p_value AND created_at>p_all_since)
$$;
-- +goose StatementEnd
