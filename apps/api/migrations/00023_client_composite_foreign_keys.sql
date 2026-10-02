-- A row referencing a client did so by id alone, so the database would accept another account's
-- client on any of these four tables. Nothing exploits that today — every write path checks the
-- client's account first, and row level security refuses the reads — but that leaves the boundary
-- resting on the application alone, and multi-tenancy in this schema is meant to be enforced twice.
--
-- The parents gain a (account_id, id) unique key so the children can reference the pair. client_tag
-- names both of its parents from a request, which is why its tag edge is here too.
--
-- The client columns on rfq, quote and notification are nullable, and a composite foreign key with
-- the default MATCH SIMPLE skips its check when any column is NULL — so "no client attached" keeps
-- working untouched.

-- +goose Up
ALTER TABLE client ADD CONSTRAINT uq_client_account UNIQUE (account_id, id);
ALTER TABLE tag ADD CONSTRAINT uq_tag_account UNIQUE (account_id, id);

ALTER TABLE rfq DROP CONSTRAINT fk_rfq_client;
ALTER TABLE rfq ADD CONSTRAINT fk_rfq_client
  FOREIGN KEY (account_id, client_id) REFERENCES client(account_id, id);

ALTER TABLE quote DROP CONSTRAINT fk_quote_client;
ALTER TABLE quote ADD CONSTRAINT fk_quote_client
  FOREIGN KEY (account_id, client_id) REFERENCES client(account_id, id);

ALTER TABLE notification DROP CONSTRAINT fk_notification_client;
ALTER TABLE notification ADD CONSTRAINT fk_notification_client
  FOREIGN KEY (account_id, client_id) REFERENCES client(account_id, id);

ALTER TABLE client_tag DROP CONSTRAINT fk_client_tag_client;
ALTER TABLE client_tag ADD CONSTRAINT fk_client_tag_client
  FOREIGN KEY (account_id, client_id) REFERENCES client(account_id, id);

ALTER TABLE client_tag DROP CONSTRAINT fk_client_tag_tag;
ALTER TABLE client_tag ADD CONSTRAINT fk_client_tag_tag
  FOREIGN KEY (account_id, tag_id) REFERENCES tag(account_id, id);

-- +goose Down
ALTER TABLE client_tag DROP CONSTRAINT fk_client_tag_tag;
ALTER TABLE client_tag ADD CONSTRAINT fk_client_tag_tag FOREIGN KEY (tag_id) REFERENCES tag(id);

ALTER TABLE client_tag DROP CONSTRAINT fk_client_tag_client;
ALTER TABLE client_tag ADD CONSTRAINT fk_client_tag_client
  FOREIGN KEY (client_id) REFERENCES client(id);

ALTER TABLE notification DROP CONSTRAINT fk_notification_client;
ALTER TABLE notification ADD CONSTRAINT fk_notification_client
  FOREIGN KEY (client_id) REFERENCES client(id);

ALTER TABLE quote DROP CONSTRAINT fk_quote_client;
ALTER TABLE quote ADD CONSTRAINT fk_quote_client FOREIGN KEY (client_id) REFERENCES client(id);

ALTER TABLE rfq DROP CONSTRAINT fk_rfq_client;
ALTER TABLE rfq ADD CONSTRAINT fk_rfq_client FOREIGN KEY (client_id) REFERENCES client(id);

ALTER TABLE tag DROP CONSTRAINT uq_tag_account;
ALTER TABLE client DROP CONSTRAINT uq_client_account;
