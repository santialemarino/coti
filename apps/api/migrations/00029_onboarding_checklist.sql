-- The setup checklist: when its card was hidden, and every setup step an account already did outside
-- the wizard recorded as done, so an account whose checklist holds nothing pending leaves it.

-- +goose Up
ALTER TABLE account_onboarding ADD COLUMN checklist_hidden_at TIMESTAMPTZ;

INSERT INTO onboarding_step_progress (account_id, onboarding_id, step_key, status)
SELECT o.account_id, o.id, proof.step_key, 'COMPLETED'
FROM account_onboarding o
JOIN account a ON a.id = o.account_id
CROSS JOIN LATERAL (VALUES
  ('BRAND', a.brand_logo_url IS NOT NULL OR a.brand_color IS NOT NULL),
  ('CATALOG_UPLOAD', EXISTS (
    SELECT 1 FROM product p WHERE p.account_id = o.account_id AND p.is_active)),
  ('TEAM', (
    SELECT count(*) FROM app_user u WHERE u.account_id = o.account_id AND u.is_active) > 1)
) AS proof (step_key, done)
WHERE o.status <> 'IN_PROGRESS' AND proof.done
ON CONFLICT (onboarding_id, step_key) DO UPDATE SET status = 'COMPLETED';

UPDATE account_onboarding o
SET status = 'COMPLETED', current_step = 'COMPLETE', completed_at = now()
WHERE o.status = 'DISMISSED'
  AND (
    SELECT count(*) FROM onboarding_step_progress s
    WHERE s.onboarding_id = o.id AND s.status = 'COMPLETED'
      AND s.step_key IN ('BRAND', 'CATALOG_UPLOAD', 'TEAM')
  ) = 3;

-- +goose Down
-- The recorded steps stay: each one is a fact about the account, not a product of this migration.
ALTER TABLE account_onboarding DROP COLUMN checklist_hidden_at;
