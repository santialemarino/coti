# Deployment

Nothing is deployed yet. This is the map of what a deploy needs, written so the first one is a
morning rather than a week. The target is **DigitalOcean**, chosen because the follow-up sweep
needs a scheduled job the platform owns (see [scheduled-jobs.md](scheduled-jobs.md)).

`.do/app.yaml` is the committed app spec. This repository is public, so it carries the shape —
components, ports, and the settings that are not credentials — while every credential is a
`type: SECRET` entry with no value, filled in the console. Four of them are needed before the first
deploy and the rest are not; see [Secrets the deploy has to supply](#secrets-the-deploy-has-to-supply).

## Three products, and they are not interchangeable

| Product              | What it is                                            | Needed?                                       |
| -------------------- | ----------------------------------------------------- | --------------------------------------------- |
| **App Platform**     | Compute. Runs the Docker images.                      | **Yes** — this is the hosting.                |
| **Managed Postgres** | The database.                                         | **Yes**                                       |
| **Spaces**           | S3-compatible object storage for RFQ attachment files | Only once `STORAGE_PROVIDER=spaces` is chosen |

Spaces is not on the critical path: `STORAGE_PROVIDER` defaults to `local`, the local adapter
works, and the Spaces adapter exists but has never run against a real bucket. See
[file-storage.md](file-storage.md).

## Six components, one app

| Component       | Type                    | Dockerfile                     | Port |
| --------------- | ----------------------- | ------------------------------ | ---- |
| `api`           | Web Service             | `docker/api.Dockerfile`        | 8000 |
| `backoffice`    | Web Service             | `docker/backoffice.Dockerfile` | 3000 |
| `webapp`        | Web Service             | `docker/webapp.Dockerfile`     | 3001 |
| `migrate`       | Job, `kind: PRE_DEPLOY` | `docker/api.Dockerfile`        | —    |
| `scheduled-job` | Job, `kind: SCHEDULED`  | `docker/api.Dockerfile`        | —    |
| the database    | Managed Postgres        | —                              | —    |

**Every component takes `source_dir: /`.** All three Dockerfiles build with the repository root as
their context — `docker-compose.yml` says `context: .`, and the web ones copy `pnpm-workspace.yaml`,
`turbo.json` and `packages/` — so a component pointed at `apps/api` fails at the first `COPY`. This
is the most likely first failure and it is the cheapest one to avoid.

The api image carries **four** binaries and the migration chain: `/api/bin/api`,
`/api/bin/scheduled-job`, `/api/bin/catalog-embed`, `/api/bin/goose`, and `/api/migrations`. That is
why one Dockerfile serves three components.

**Instance sizing.** The spec uses `apps-s-1vcpu-1gb` (1 shared vCPU, 1 GiB, $12/month). Nothing has
been load-tested; it is the smallest size worth starting a Next.js server on, and the number the
cost estimate is built from. Jobs are billed only for the time they run.

## Managed Postgres

**PostgreSQL 16 or 17, never 15.** The chain needs three extensions and `vector` is absent from
PG15's Standard extension list, while `unaccent` and `pgcrypto` are listed for 14 through 18.
`00001` creates `vector` and `pgcrypto`, `00009` creates `unaccent`.

**The extension registers as `vector`, not `pgvector`** — `CREATE EXTENSION vector;` is what the
migration runs, and asking for `pgvector` fails.

**Managed Postgres forces TLS.** Every connection string needs `?sslmode=require`; the local ones
use `sslmode=disable` and copying one up is a connection that never opens.

### Two roles, and only one of them exists for you

| Variable             | Role       | Where it comes from                       |
| -------------------- | ---------- | ----------------------------------------- |
| `DATABASE_ADMIN_URL` | `doadmin`  | The cluster's own owner, created with it. |
| `DATABASE_URL`       | `coti_app` | Created by `00001` **with no password**.  |

`doadmin` is the owner: migrations, the scheduled jobs, and the pre-auth lookups that legitimately
cross accounts run as it. `coti_app` is the restricted `NOBYPASSRLS` role every request-scoped query
uses, and row level security is only a second net because the API connects as it. See
[database.md](database.md).

**Provisioning `coti_app`'s password is a manual step and nothing does it for you.** The chain
creates a `LOGIN` role with a null password, which cannot authenticate under `scram-sha-256`, so a
database that skipped this step refuses the API loudly instead of accepting a password anyone can
read in this repository.

**Do it on the cluster before the first deploy, not after it.** `00001` guards the role with
`IF NOT EXISTS`, so a role that is already there is left exactly as provisioned and still collects
every grant the migration hands out:

```sql
CREATE ROLE coti_app LOGIN PASSWORD '<generated>'
  NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS;
```

Then set `DATABASE_URL` to that role's connection string as a `SECRET`. Provisioning afterwards
works too — `ALTER ROLE coti_app PASSWORD '<generated>'` — but the deploy in between fails, because
the API cannot start against a role it cannot authenticate as.

**Either way, do not put it in a deploy job.** `ALTER ROLE` and `CREATE ROLE` are DDL, so a cluster
running with `log_statement = 'ddl'` writes the plaintext password into a log the platform exposes —
a loud refusal beats a self-healing secret in a log.

The failure mode is legible, and it is louder than a failed probe: `repository.NewDB` pings each
pool before returning, so an unprovisioned role makes the API **refuse to start**, with the reason
in the first line of the log.

```
level=ERROR msg="startup failed" error="app pool: ping: failed to connect to `user=coti_app …`:
failed SASL auth: FATAL: password authentication failed for user \"coti_app\" (SQLSTATE 28P01)"
```

## Migrations run from a PRE_DEPLOY job

```
run_command: /api/bin/goose -dir /api/migrations postgres "$DATABASE_ADMIN_URL" up
```

**Not on API startup.** Startup migration races across instances, and a bad migration takes the app
down instead of failing the deploy. A `PRE_DEPLOY` job runs before the new containers take traffic,
once, and a failure stops the rollout with the old version still serving.

The job needs `DATABASE_ADMIN_URL` and nothing else. goose is pinned as a tool dependency in
`apps/api/go.mod`, so the image's copy and the one `pnpm db:migrate` uses locally are the same
version by construction.

## Two gotchas that cost an afternoon

**`NEXT_PUBLIC_API_URL` is baked into the JavaScript bundle.** It is a build `ARG` in both web
Dockerfiles, so on App Platform it must be scoped **`RUN_AND_BUILD_TIME`**. Set as run-time only,
the deployed frontends call `http://localhost:8000` from the visitor's browser and there is no
server-side error to find.

There is an ordering problem behind it: the frontends need the API's public URL at build time, and
that URL does not exist until the app does. **`${APP_URL}` does not get you out of it** — under a
Dockerfile build App Platform resolves bindable variables at runtime only, so a build-time
`${APP_URL}/api` is baked in literally. The way through is the round trip: deploy with a
placeholder, read the app's URL, set it, and redeploy so the frontends rebuild. Runtime-only
settings can still use the bindable, and `STORAGE_LOCAL_API_BASE_URL` and `WEB_BACKOFFICE_URL` do.

**`ENV=production` refuses to boot when `DATABASE_URL` equals `DATABASE_ADMIN_URL`.** `config.Load()`
rejects it deliberately: the request pool running as the owner would bypass every RLS policy, which
is a cross-account leak rather than a misconfiguration. Give the two roles genuinely different URLs.

## Secrets the deploy has to supply

Each is a `type: SECRET` entry in the spec with no committed value, except the last two: nothing
selects Spaces, so the spec declares neither them nor the three non-secret keys that come with them.
Choosing `spaces` means adding all five.

**Only the first four block a boot, and that is deliberate.** `config.Load()` refuses to start when
a capability is switched on and its credential is empty, so the committed spec ships every optional
one **off** — `AI_*_PROVIDER` at `disabled`, `MAIL_PROVIDER` at `console`,
`RATE_LIMIT_TRUSTED_PROXY_HOPS` at `0`. Fill a secret, then turn its capability on; doing it the
other way round produces a deploy that never starts, and `ci.deploy-spec.yml` boots the image on the
spec's own settings to keep that true.

| Key                             | Needed when                                                        | Shape                               |
| ------------------------------- | ------------------------------------------------------------------ | ----------------------------------- |
| `DATABASE_URL`                  | Always                                                             | `coti_app`'s URL, `sslmode=require` |
| `DATABASE_ADMIN_URL`            | Always                                                             | `doadmin`'s URL, `sslmode=require`  |
| `AUTH_JWT_SECRET`               | Always                                                             | ≥ 32 characters                     |
| `STORAGE_LOCAL_SIGNING_SECRET`  | While `STORAGE_PROVIDER=local`                                     | ≥ 32 characters                     |
| `CHANNEL_CONFIG_ENCRYPTION_KEY` | To configure any intake channel                                    | 32 bytes of base64                  |
| `AI_ANTHROPIC_API_KEY`          | `AI_LLM_PROVIDER=anthropic`                                        | Provider key                        |
| `AI_OPENAI_API_KEY`             | `AI_EMBEDDINGS_PROVIDER` or `AI_TRANSCRIPTION_PROVIDER` = `openai` | Provider key                        |
| `MAIL_SMTP_USERNAME`            | `MAIL_PROVIDER=smtp`                                               | The mailbox                         |
| `MAIL_SMTP_PASSWORD`            | `MAIL_PROVIDER=smtp`                                               | A Google App Password               |
| `STORAGE_ACCESS_KEY`            | `STORAGE_PROVIDER=spaces`                                          | Spaces key                          |
| `STORAGE_SECRET_KEY`            | `STORAGE_PROVIDER=spaces`                                          | Spaces secret                       |

Three more are marked `SECRET` in the spec without being credentials, purely to keep a value out of
a public file: `MAIL_SMTP_HOST`, `MAIL_FROM_ADDRESS` — which must be the mailbox itself or Google
rewrites the `From` header — and `RATE_LIMIT_TRUSTED_PROXY_CIDRS`, the platform's forwarding range.

Three of them behave differently from the rest and it is worth knowing which:

- **`AUTH_JWT_SECRET` is symmetric**, so anything holding it can mint a token for any account. The
  backoffice deliberately does not get it; it forwards the bearer and asks `GET /v1/me`.
- **`CHANNEL_CONFIG_ENCRYPTION_KEY` is not a boot requirement.** Unset, the API runs and only
  _storing_ a channel credential is refused with 503. Rotating it makes every already-sealed
  credential unreadable, and nothing re-seals them.
- **`STORAGE_LOCAL_SIGNING_SECRET` is a credential, not a convenience** — it is what stops a storage
  link being forged, so a deployment that shares it shares every stored file.

The switches to flip once their secrets are in, none of which the first deploy needs:
`AI_LLM_PROVIDER=anthropic`, `AI_EMBEDDINGS_PROVIDER` and `AI_TRANSCRIPTION_PROVIDER` to `openai`,
`MAIL_PROVIDER=smtp` with `MAIL_FROM_ADDRESS` (which must be the mailbox itself or Google rewrites
the `From` header), and `RATE_LIMIT_TRUSTED_PROXY_HOPS=1`.

**`RATE_LIMIT_TRUSTED_PROXY_HOPS` and `RATE_LIMIT_TRUSTED_PROXY_CIDRS` are a startup error unless
both are set**, in either direction: hop counting is only spoof-resistant for a request that really
transited the declared chain, and CIDRs with the hop count at 0 mean the header is never read. The
backoffice is one of those proxies — its calls are server-side, so until a hop is declared every
user in the product shares one rate-limit allowance, which is the one switch worth flipping early.
`WEB_TRUSTED_PROXY_HOPS` on the backoffice moves with it, and what each is worth depends on how many
proxies the platform actually puts in front; the spec leaves both at the `.env.example` default.

The full list of keys, with defaults and what each bounds, is in the four `.env.example` files.

## First deploy, in order

1. Create the **Managed Postgres** cluster (PG 16 or 17). Nothing else can be done first.
2. `CREATE ROLE coti_app …` on it, as above. Doing this **before** anything deploys is what makes
   the first deploy succeed rather than fail at the api component.
3. Create the app from `.do/app.yaml`, filling in the database component's `cluster_name`, `db_name`
   and `db_user`, and the four secrets a boot needs: both database URLs, `AUTH_JWT_SECRET` and
   `STORAGE_LOCAL_SIGNING_SECRET`. The rest can wait. `NEXT_PUBLIC_API_URL` is still a placeholder.
4. The `migrate` PRE_DEPLOY job applies the chain as `doadmin`; the grants and RLS policies land on
   the role from step 2, whose password it leaves alone.
5. Read the app's URL, set `NEXT_PUBLIC_API_URL` to `<url>/api`, and redeploy so the frontends
   rebuild around it. `STORAGE_LOCAL_API_BASE_URL` and `WEB_BACKOFFICE_URL` need no second pass —
   they are bound to `${APP_URL}` and resolve at runtime.
6. Fill the optional secrets and flip their switches: mail, then the two AI vendors, then the
   rate-limit proxy pair. Each is a restart, not a rebuild.
7. Register the first account, then embed its catalog — `/api/bin/catalog-embed --account <uuid>`
   from a console on the api component — and build the vector index once there are rows
   (`pnpm db:vector-index`, see [catalog.md](catalog.md)). It is deliberately not in the chain: on
   an empty table an ivfflat index is degenerate.
8. Add the scheduled job's cron once there is a job registered to run; `cmd/scheduled-job --list` is
   empty until a feature registers one.

## What CI already proves

`.github/workflows/ci.docker.yml` builds all three images on every PR that touches them or what they
copy, and boots every one of them. The api job goes further: it asserts the four binaries are in the
image and applies the whole migration chain **from inside it** against a real Postgres, which is the
PRE_DEPLOY job's exact command.

`.github/workflows/ci.deploy-spec.yml` covers the spec itself: `doctl apps spec validate
--schema-only`, which needs no account, plus the three things a schema check cannot see — that no
`type: SECRET` entry carries a value, that every component builds `/` with a Dockerfile that
exists, and that every ingress rule names a component the spec declares. Its second job **boots the
api image on the spec's own settings**, with only those four secrets filled, and requires `/ready`.
That is the one check that catches a capability switched on beside an empty credential, which is a
deploy that never starts and looks like a valid spec until it is applied.

So the image half of a deploy is continuously verified, and the spec is checked for the mistakes
that are checkable offline. What remains unverified is the platform half — whether the app the spec
describes actually comes up — and that cannot be checked without an app.
