-- +goose Up
CREATE TABLE client_merge_audit (
 id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
 account_id UUID NOT NULL REFERENCES account(id),
 source_client_id UUID NOT NULL,
 target_client_id UUID NOT NULL,
 -- NULL when the customer's own contact proof merged the profiles.
 merged_by UUID,
 merged_at TIMESTAMPTZ NOT NULL DEFAULT now(),
 FOREIGN KEY (account_id,source_client_id) REFERENCES client(account_id,id),
 FOREIGN KEY (account_id,target_client_id) REFERENCES client(account_id,id),
 FOREIGN KEY (account_id,merged_by) REFERENCES app_user(account_id,id)
);
CREATE INDEX idx_client_merge_audit_target ON client_merge_audit(account_id,target_client_id);
ALTER TABLE client_merge_audit ENABLE ROW LEVEL SECURITY;
CREATE POLICY client_merge_audit_account_isolation ON client_merge_audit
 USING(account_id=app_current_account_id()) WITH CHECK(account_id=app_current_account_id());

-- +goose Down
DROP TABLE client_merge_audit;
