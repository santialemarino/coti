# Database

PostgreSQL 16 + pgvector. The model is 46 tables with UUID v4 primary keys, native enums, and
money in `NUMERIC(14,2)`.

## What is the source and what is the reference

| File                                     | Role                                                                 |
| ---------------------------------------- | -------------------------------------------------------------------- |
| `apps/api/migrations/*.sql`              | **The executable source.** The only thing that writes to a database. |
| `apps/api/database/01_create_tables.sql` | Consolidated reference. Read, never applied.                         |
| `apps/api/database/02_seed_dev.sql`      | Development data. Idempotent.                                        |

A schema change ships a goose migration **and** updates the reference in the same PR. The
reference is what you read to write a `SELECT` column list, a scan order, or a domain
struct's fields.

Past the first migration the comparison stops being textual — the reference describes the
result, not the sequence that reaches it — so compare the schema each one produces. Two empty
databases, one migrated and one with the reference applied, and a `pg_dump` of each: the
difference has to be empty apart from the session tokens `pg_dump` generates at random.

```bash
docker exec migrated  pg_dump -U coti -d coti --schema-only --no-owner --no-privileges \
  --exclude-table=goose_db_version > /tmp/from-migrations.sql
docker exec reference pg_dump -U coti -d coti --schema-only --no-owner --no-privileges \
  > /tmp/from-reference.sql
diff /tmp/from-reference.sql /tmp/from-migrations.sql
```

This also catches column order, which is easy to lose sight of: `ALTER TABLE ADD COLUMN`
appends, so a new column is last in the real table even though it would read better next to
its siblings. The reference follows the physical schema, not taste.

## Bringing a database up

```bash
pnpm db:init      # postgres + goose up + the app role's password + seed
pnpm db:migrate   # apply pending migrations
pnpm db:seed      # the seed only (idempotent)
pnpm db:reset     # drop the volume and rebuild
pnpm db:create-migration <name>
```

**One index is not in the chain, on purpose.** The catalog's approximate vector index is
degenerate when built on an empty table, so it is a start-up step run once the catalog is loaded
and embedded — `pnpm db:vector-index`, documented in [catalog.md](catalog.md#embedding-the-catalog).
A fresh database is correct without it; the semantic search is just slower.

`POSTGRES_PORT` (default 5432) changes the port the container publishes when another local
Postgres already holds it. It has to stay in sync with the URLs.

**`ON CONFLICT DO NOTHING` is not idempotent on its own:** it needs a unique constraint to
collide with. On a table whose only unique index is the primary key — a random UUID — it
filters nothing and every run re-inserts everything. That is what happened to
`product_synonym` until `uq_product_synonym_term`. Seeding a table means naming the
`ON CONFLICT` target explicitly, and if no natural key backs it, an index is missing.

**The seed is idempotent, not convergent.** It inserts with `ON CONFLICT ... DO NOTHING`, so
it creates what is missing but **never rewrites an existing row**. If a seed value changes — a
status, a total, a name — a database that already had the row keeps the old one, and
`pnpm db:seed` does not correct it. Picking up changed values means `pnpm db:reset`, which
rebuilds through the whole chain. That is deliberate: a seed that overwrites destroys whatever
you were testing against.

Two cases where `db:reset` is the only way out:

- **A seed value changed** (above).
- **An already-applied migration was edited.** The `down` describes the new `up`, so against a
  database that ran the old one it tries to revert things it never created, and fails. This
  happens while a migration PR is in review: whoever already ran it has to reset when pulling.
  The way out is **not** to fill the `down` with `IF EXISTS` to tolerate half-states.

## Deactivating and reactivating an account

`account.is_active` decides whether a corralón's users can get in at all — login, refresh and
tenant resolution read it on every request. It is written by a script pair rather than an
endpoint, because `user_role` holds only `ADMIN` and `SELLER` and neither of them is an actor
who should be able to close a corralón.

```bash
pnpm db:account:deactivate --account <uuid>
pnpm db:account:activate   --account <uuid>
```

Both run on `DATABASE_ADMIN_URL` — they are not request-scoped, so they have no tenant to
narrow to. Both take the id as a query parameter, report the account they touched and the state
it was in, and refuse an unknown id without writing anything.

`deactivate` is two writes in one transaction: the flag, and `session_epoch + 1` for every user
of the account. **The flag is what cuts access** — it is read on every request — so the bump is
not what makes outstanding tokens stop working. What the bump buys is that a token minted before
the closure stays dead once the account is reopened, instead of working again for the rest of its
lifetime. `activate` is therefore the flag alone. The behavioural side is in
[authentication.md](authentication.md#a-deactivated-account-cuts-every-way-in).

## Two connection roles

| Variable             | Role                                   | What for                                                                                                   |
| -------------------- | -------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| `DATABASE_URL`       | `coti_app` — restricted, `NOBYPASSRLS` | Every request-scoped query.                                                                                |
| `DATABASE_ADMIN_URL` | owner                                  | Migrations, the operational scripts, the follow-up cron, and the lookups that legitimately cross accounts. |

**Never use the owner role for a request-scoped query.** It bypasses RLS.

**A process with no cross-account job does not open it at all.** `repository.NewTenantDB` opens the
restricted pool alone and returns a type carrying neither `CrossAccount` nor `AdminTx`, so the
boundary is the compiler rather than a rule someone has to remember — `cmd/catalog-embed` runs that
way. `repository.NewDB` adds the owner pool for the processes below.

The four legitimate owner cases:

1. **Migrations** — they create tables and grant permissions.
2. **Operational scripts** — `pnpm db:seed`, the account activation pair above, and
   `pnpm db:vector-index`, which spans accounts and creates an index the request role does not
   own the table for. They are run by hand, from outside any request, so there is no tenant to
   scope them to.
3. **Scheduled jobs** — `cmd/scheduled-job` sweeps across every account, because "every quote with
   no movement" belongs to no single corralón. It holds one owner connection for the run rather
   than working through the pool (`DB.AdminConn`), since the advisory lock keeping two runs apart
   lives on the connection that took it. See [scheduled-jobs.md](scheduled-jobs.md).
4. **Pre-auth lookups** — login by email (the account is not known yet) and resolving
   `quote_send.public_token` for the sessionless webapp. The correct pattern for the token:
   the owner resolves token → `account_id`, and the rest of the request continues on the
   restricted role with the GUC set.

### The app role has no password until something sets one

`00001` creates `coti_app` with **no `PASSWORD` clause at all**. A `LOGIN` role whose password is
null cannot authenticate under `scram-sha-256`, so a database built by running the chain and
nothing else refuses every connection as `coti_app` instead of accepting a password anyone can read
in a public repository. The refusal is loud: `repository.NewDB` pings each pool before returning, so
the API **fails to start** and says why, rather than booting and failing a request later.

Giving it one is a separate step, and it is deliberately not the chain's:

- **Locally**, `pnpm db:init` does it after migrating, taking the role and password straight out of
  `DATABASE_URL` so the two cannot drift. Running `pnpm db:migrate` alone against a brand-new
  database leaves the API unable to start; `pnpm db:init` is the command that finishes the job.
- **In CI**, `ci.api.yml` sets the same published default before the integration suite, which is
  the only job that connects as `coti_app`.
- **In a deployment**, a person sets it once, and best **before the first migration runs**: the
  `IF NOT EXISTS` guard leaves an already-provisioned role exactly as it is while it still collects
  every grant. It is not done from a deploy job: `ALTER ROLE` and `CREATE ROLE` are DDL, so a
  managed Postgres running with `log_statement = 'ddl'` would write the password into a log the
  platform exposes.

That same guard is why an existing database is untouched by this: the role is already there with
whatever password it was given, and re-running the chain never revisits it.

`docker-compose.yml` overrides the dockerised API's `DATABASE_URL` to reach the `postgres` service
by name, and that override carries the password literally. It matches what `.env.example` ships, so
changing the local one means changing both.

## Account isolation (RLS)

Every tenant-scoped table carries `account_id` — child tables included — and a policy
comparing it against `app_current_account_id()`, which reads the `app.current_account_id` GUC.

```sql
BEGIN;
  SET LOCAL app.current_account_id = '<account uuid>';
  -- the request's queries
COMMIT;
```

That is for psql. **From Go the GUC is set by `repository.DB.InTenantTx`** with
`SELECT set_config('app.current_account_id', $1, true)`, and that is the only path for a
request-scoped query. `SET LOCAL` is not used in code: that form accepts no bind parameters,
so it would force interpolating a request-derived value into the SQL.

**It goes in every transaction**: the pool reuses connections, so it is not inherited. And
**every request-scoped query has to run inside a transaction**, reads included: the GUC is
transaction-scoped, so a query on the bare pool runs outside the scope, matches no policy, and
silently reads zero rows.

The three cases that legitimately cross accounts use `db.CrossAccount()` (or `db.AdminTx()`
for multi-step writes), which go through the owner pool.

The **account** is enforced, not the branch: an admin legitimately reads every branch of their
own account, so `branch_id` scoping stays in the application. RLS is the second net, not a
replacement for the `WHERE`: the explicit predicates still go in every query so the plan uses
the indexes.

Three known traps:

- **Works in psql, empty in the app.** psql as owner bypasses RLS; the app does not. To
  reproduce what the app sees, connect as `coti_app` and set the GUC.
- **A migration that adds a table and forgets the RLS policy does not fail: it returns every
  account's rows.** A new table is born with RLS disabled, so the app role reads all of it
  with no error, no permission denied and nothing to give it away — silent cross-account
  exposure. Every new table with `account_id` ships its `ENABLE ROW LEVEL SECURITY` and its
  policy in the same migration.
- **The GRANT, by contrast, is already covered:** `ALTER DEFAULT PRIVILEGES` reaches the
  tables the owner creates, and migrations run as that role. It is written explicitly anyway,
  so nothing depends on the migrating role always being the same.

## Invariants in the database

Whatever can be expressed in the schema is expressed in the schema:

| Index                                    | Invariant                                          |
| ---------------------------------------- | -------------------------------------------------- |
| `uq_quote_rfq` + `quote.rfq_id NOT NULL` | 1-to-1 rfq→quote                                   |
| `uq_quote_version_draft`                 | one in-progress draft per quote                    |
| `uq_message_batch_open`                  | one open message window per quote                  |
| `uq_message_batch_processing`            | one processing batch (FIFO queue)                  |
| `uq_quote_send_public_token`             | the magic-link token is unique                     |
| `uq_product_account_code`                | a product code is unique within the account        |
| `uq_product_synonym_term`                | one term per product, case-insensitively           |
| `uq_channel_branch_type_no_identifier`   | one identifier-less channel per branch and type    |
| `uq_product_price_open_period`           | one open price period per branch and product       |
| `uq_app_user_email_global`               | an address identifies one user, case-insensitively |
| `uq_auth_token_hash`                     | a recovery or verification link is unique          |
| `uq_tag_account_name`                    | one tag name per account, case-insensitively       |
| `uq_promotion_tier_from_quantity`        | one tier per promotion and starting quantity       |
| `uq_promotion_condition_item_target`     | one condition row per promotion and target         |

**A unique constraint does not compare NULLs**, so on a nullable column it lets every empty
row escape. That is why the 1-to-1 needs the NOT NULL as well as the index:
`uq_channel_branch_type_identifier` alone does not bound the identifier-less channels, which
is where the partial index comes in. Pinning an invariant on a nullable column has three ways
out — a NOT NULL, a partial index over the NULL case, or `UNIQUE NULLS NOT DISTINCT`, which
compares them as equal. The third is what `uq_promotion_condition_item_target` uses: a condition
row names one target and leaves the other three key columns null, so an index that skipped them
would bound nothing.

**An index and a lock do different jobs, and one open price period needs both.**
`uq_product_price_open_period` turns a second open row into an error, but an error is not what
a legitimate concurrent reprice deserves: without a lock the loser's
`UPDATE ... WHERE valid_to IS NULL` matches nothing and its insert then hits the index. What
makes both writes _succeed correctly_ is `SetPrice` taking a `SELECT ... FOR UPDATE` on the
parent product row first. The index is the backstop for a path that forgets the lock.

Single use on `auth_token` is the same shape from the other direction: it is not an index at
all but a predicate, `UPDATE ... WHERE consumed_at IS NULL`, which is what serializes two
simultaneous redemptions of one link.

## Catalog

The catalog belongs to the **account**: `product`, `product_synonym`, `product_alternative`
and `combo` all hang off `account_id`. One product is one row per account, with one embedding
and one set of synonyms.

What varies per branch:

- `branch_product` — whether the branch carries it, and with what stock.
- `product_price` — price and `min_price` per branch, versioned by validity.
- `branch_combo` — whether the branch offers the combo. No price and no stock: both derive
  from the items, which are already priced per branch.

Semantic search filters by `account_id` and joins `branch_product` to exclude what the branch
does not sell. Because the ANN index filters **after** ordering, you have to over-fetch
(`LIMIT k * factor`) and trim in the service, or the branch filter can leave fewer than K
results.

## Vector index

`idx_product_embedding` (ivfflat, `vector_cosine_ops`) is **commented out** in the migration:
built on an empty table it is suboptimal. It is created after the catalog loads and the
embeddings are generated.

## Enums

Native PostgreSQL types, values in **UPPERCASE English**. The Spanish labels live in the
frontend i18n and are never stored in the database. Adding a value takes
`ALTER TYPE ... ADD VALUE`, which is acceptable because the domain's enums are closed by
design.

Three things that bite when migrating an enum:

- **A newly added value cannot be used in the transaction that added it**
  (`unsafe use of new value`). goose wraps each file in a transaction, so a migration that
  adds the value **and** writes rows with it has to be split in two.
- **Renaming does work** (`ALTER TYPE ... RENAME VALUE`): it is metadata only, existing rows
  keep pointing at the same entry, and the value is usable immediately. When the old value
  already meant the new thing, renaming beats adding.
- **Removing a value does not exist.** You have to recreate the type and cast every column
  that uses it, so it pays to know which ones those are before starting.

The lifecycle is split across two entities: `rfq.status` (`RECEIVED`, `GENERATED`) and
`quote.current_status` (`DRAFT` onward). The quote is born at the RECEIVED → GENERATED
transition with `current_status = DRAFT` (materials matched, prices not accepted), because
extracted items can only live under a quote version.

## `created_at` / `updated_at`

`created_at` on every table. `updated_at` plus the `set_updated_at()` trigger only on the ones
that mutate in place: `account`, `branch`, `app_user`, `product`, `branch_product`, `combo`,
`branch_combo`, `client`, `channel`, `rfq`, `quote`, `promotion`. Append-only tables carry
none, and that communicates in the schema itself which table is a log and which is live state.

Process tables instead carry **one timestamp per transition that matters** rather than a
generic `updated_at`: `notification.sent_at`, `rfq_attachment.processed_at`,
`quote.followup_flagged_at`, `quote.archived_at`. An `updated_at` says something changed;
these say what changed and when, which is what gets queried later.
