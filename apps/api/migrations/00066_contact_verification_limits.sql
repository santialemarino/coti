-- +goose Up
-- One row per wrong proof, so failed guesses against a destination are counted across every supplier.
CREATE TABLE client_contact_challenge_failure (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 account_id UUID NOT NULL REFERENCES account(id),
 challenge_id UUID NOT NULL REFERENCES client_contact_challenge(id) ON DELETE CASCADE,
 kind channel_type NOT NULL CHECK(kind IN ('WHATSAPP','EMAIL')),
 value VARCHAR(255) NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_contact_challenge_failure_destination ON client_contact_challenge_failure(account_id,kind,value,created_at);
ALTER TABLE client_contact_challenge_failure ENABLE ROW LEVEL SECURITY;
CREATE POLICY contact_challenge_failure_account_isolation ON client_contact_challenge_failure USING(account_id=app_current_account_id()) WITH CHECK(account_id=app_current_account_id());

CREATE INDEX idx_contact_challenge_destination ON client_contact_challenge(kind,value,created_at);

-- Runs as the table owner so the anti-spam cap sees every supplier's proofs; returns only counts.
-- +goose StatementBegin
CREATE FUNCTION contact_destination_activity(p_account_id UUID, p_kind channel_type, p_value TEXT, p_challenges_since TIMESTAMPTZ, p_failures_since TIMESTAMPTZ, p_all_since TIMESTAMPTZ)
 RETURNS TABLE(supplier_challenges INTEGER, supplier_failures INTEGER, all_challenges INTEGER)
 LANGUAGE sql STABLE SECURITY DEFINER SET search_path = public, pg_temp AS $$
 SELECT (SELECT count(*)::int FROM client_contact_challenge WHERE account_id=p_account_id AND kind=p_kind AND value=p_value AND created_at>p_challenges_since),
        (SELECT count(*)::int FROM client_contact_challenge_failure WHERE account_id=p_account_id AND kind=p_kind AND value=p_value AND created_at>p_failures_since),
        (SELECT count(*)::int FROM client_contact_challenge WHERE kind=p_kind AND value=p_value AND created_at>p_all_since)
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION contact_destination_activity(UUID,channel_type,TEXT,TIMESTAMPTZ,TIMESTAMPTZ,TIMESTAMPTZ) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION contact_destination_activity(UUID,channel_type,TEXT,TIMESTAMPTZ,TIMESTAMPTZ,TIMESTAMPTZ) TO coti_app;

-- +goose Down
DROP FUNCTION contact_destination_activity(UUID,channel_type,TEXT,TIMESTAMPTZ,TIMESTAMPTZ,TIMESTAMPTZ);
DROP INDEX idx_contact_challenge_destination;
DROP TABLE client_contact_challenge_failure;
