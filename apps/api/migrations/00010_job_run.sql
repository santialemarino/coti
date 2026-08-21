-- The record every scheduled run leaves behind: what it swept and what it changed.
--
-- It carries no account_id on purpose. A scheduled job runs as the owner across every account, so
-- the run is one row for all of them; a per-account column would have to be a lie or a list.

-- +goose Up

CREATE TYPE job_run_status AS ENUM ('RUNNING', 'SUCCEEDED', 'FAILED');

CREATE TABLE job_run (
  id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  job_name    VARCHAR(64) NOT NULL,
  status      job_run_status NOT NULL DEFAULT 'RUNNING',
  scanned     INTEGER NOT NULL DEFAULT 0,
  changed     INTEGER NOT NULL DEFAULT 0,
  error       TEXT,
  started_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  finished_at TIMESTAMPTZ,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- "Which run flagged this quote?" is answered by the row's own timestamp falling inside a run's
-- window, so the history is read newest-first per job.
CREATE INDEX idx_job_run_name_started ON job_run(job_name, started_at DESC);

-- ALTER DEFAULT PRIVILEGES grants every new table to the request role, and this one is an audit
-- trail no request has any reason to read, let alone rewrite.
REVOKE ALL ON job_run FROM coti_app;

-- +goose Down

DROP TABLE job_run;
DROP TYPE job_run_status;
