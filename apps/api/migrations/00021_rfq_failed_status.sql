-- An RFQ whose reading never finished needs a state of its own. Without one it stays RECEIVED,
-- which the backoffice renders as "still working on it" — so a failed extraction is shown as an
-- order in progress forever, and the seller can neither retry it nor tell it apart from one that
-- really is being read.
--
-- FAILED is terminal on the RFQ side and never produces a quote, so the state×intention matrix,
-- which is evaluated on quote.current_status, is untouched.

-- +goose Up
ALTER TYPE rfq_status ADD VALUE IF NOT EXISTS 'FAILED';

-- +goose Down
-- Postgres cannot drop an enum value, so the type is rebuilt without it. Three columns carry it:
-- the RFQ's own status and both ends of its history.
DELETE FROM rfq_status_change WHERE new_status = 'FAILED' OR previous_status = 'FAILED';

ALTER TABLE rfq ALTER COLUMN status DROP DEFAULT;

ALTER TYPE rfq_status RENAME TO rfq_status_old;

CREATE TYPE rfq_status AS ENUM ('RECEIVED', 'GENERATED');

-- An RFQ that had given up goes back to the state it was created with, which is the only one
-- left that means "no quote yet".
ALTER TABLE rfq
  ALTER COLUMN status TYPE rfq_status
  USING (CASE status::text WHEN 'FAILED' THEN 'RECEIVED' ELSE status::text END)::rfq_status;

ALTER TABLE rfq_status_change
  ALTER COLUMN previous_status TYPE rfq_status USING previous_status::text::rfq_status,
  ALTER COLUMN new_status TYPE rfq_status USING new_status::text::rfq_status;

ALTER TABLE rfq ALTER COLUMN status SET DEFAULT 'RECEIVED';

DROP TYPE rfq_status_old;
