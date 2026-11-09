# Deployment

Customer Google sign-in redirects use the public `Host` header over HTTPS in production.
The reverse proxy must preserve that header, which also scopes customer API requests to the
registered supplier. The container's `localhost:3001` request URL must not become a browser
redirect. Development retains HTTP supplier subdomains under `localhost`.

## Browser routes and the versioned API

Reserve `/api/v1` for the Go API and rewrite that prefix to `/v1/`. Keep the trailing slash:
App Platform concatenates the remaining path without inserting a separator. Route `/api/ready` and
`/api/health` to the corresponding Go health endpoints. Other `/api/*` paths belong to the
Next.js backoffice and must follow its catch-all rule. In particular, browser calls to
`/api/channels` and `/api/rfqs/file-drafts` must reach their authenticated BFF handlers before
those handlers forward requests to `/api/v1/*`. Sending every `/api/*` request directly to Go
bypasses those handlers and makes file intake fail before any AI provider is called.

The public API base remains `https://<core-host>/api`; server-side callers append `/v1`.

## Neon with temporary hostnames

Without a custom domain, deploy two App Platform apps so staff cookies and customer pages have
separate origins. Neither template declares a paid DigitalOcean database or a Spaces bucket:

| Spec                | App           | Components                                           |
| ------------------- | ------------- | ---------------------------------------------------- |
| `.do/app.neon.yaml` | `coti-core`   | API, backoffice, migrations and seven scheduled jobs |
| `.do/portal.yaml`   | `coti-portal` | Customer webapp                                      |

Both clone the public GitHub repository over HTTPS, using `main` as the source branch. This avoids
requiring a repository-owner GitHub App installation. Redeploy explicitly after a release; this
source mode does not enable automatic deploys on push. Publish the integration through the repository's
review and release flow before creating the apps. App Platform cannot build uncommitted local files.
For a validation deployment, use the review branch in a local copy of each spec and switch both
apps to `main` after approval and release.

Each web service uses a fixed 1 GiB instance. At the published price of USD 10 per service per month,
the three services total USD 30 per month before scheduled jobs, usage-based services and taxes.
Confirm current prices in the DigitalOcean summary before deployment.

1. Authenticate the DigitalOcean CLI with `doctl auth init`.
2. Create the portal from `.do/portal.yaml`; its default hostname becomes the customer portal URL.
   The initial API URL is a placeholder and the portal cannot contact the API until step 5.
3. Fill the core template's encrypted settings outside git. Use the existing restricted Neon URL
   for `DATABASE_URL`, the direct owner URL for `DATABASE_ADMIN_URL`, and the storage settings from
   [file-storage.md](file-storage.md#pointing-it-at-neon-object-storage). Each API/job component
   receives its own settings; the web components do not receive database or storage credentials.
4. Set `WEB_WEBAPP_URL` on the core API to the portal's HTTPS URL, then create the core app. Its
   default hostname serves backoffice pages, browser BFF handlers, and the versioned API routes.
5. Replace `NEXT_PUBLIC_API_URL` on both web components with the literal core HTTPS URL plus `/api`.
   Keep `RUN_AND_BUILD_TIME` scope and rebuild both apps. Bindable runtime URLs do not replace
   values already compiled into the frontend.
6. Verify `/api/ready`, registration, login, private attachments and customer portal routing.
   Register the intended supplier hostname through the application's channel configuration.

The core template enables Anthropic, embeddings and transcription, so both AI provider keys are
required. The extraction and conversation jobs carry those provider settings too. Mail remains
`console` until an SMTP provider is configured; real mail delivery and externally authenticated
channels must be configured and tested before treating the deployment as fully operational.

The original `.do/app.yaml` remains the single-app, Managed PostgreSQL alternative described below.

## Single-app deployment alternative

This is the map of what an application deploy needs. The hosting target is **DigitalOcean**, chosen because the follow-up sweep
needs a scheduled job the platform owns (see [scheduled-jobs.md](scheduled-jobs.md)).

PostgreSQL can run externally on Neon. Follow [neon.md](neon.md) for that setup and omit the
Managed Postgres component from the deployment copy of the spec. The Managed Postgres instructions
below apply when choosing DigitalOcean as the database provider.

For Neon file storage, follow [file-storage.md](file-storage.md#pointing-it-at-neon-object-storage)
and set `STORAGE_PROVIDER=neon` with the same bucket credentials on `api` and
`attachment-extraction`. The committed spec's `local` default is only suitable for bootstrapping;
each application container has its own temporary disk.

`.do/app.yaml` is the committed app spec. This repository is public, so it carries the shape —
components, ports, and the settings that are neither credentials nor hostnames — while every
credential is a `type: SECRET` entry with no value, filled in the console. Four of them are needed
before the first deploy and the rest are not; see
[Secrets the deploy has to supply](#secrets-the-deploy-has-to-supply).

## Three products, and they are not interchangeable

| Product              | What it is                                            | Needed?                                       |
| -------------------- | ----------------------------------------------------- | --------------------------------------------- |
| **App Platform**     | Compute. Runs the Docker images.                      | **Yes** — this is the hosting.                |
| **Managed Postgres** | The database.                                         | **Yes**                                       |
| **Spaces**           | S3-compatible object storage for RFQ attachment files | **Yes, for any order that arrives as a file** |

Spaces is on the critical path for file orders. Under `local` a file lands on the disk of the `api`
container that received it, which a redeploy wipes and which `attachment-extraction` — a container
of its own that downloads every file it reads — never sees. The spec still ships
`STORAGE_PROVIDER=local` on both, so the app boots on its boot secrets alone and text orders work;
switch both to `spaces` together once the bucket's five keys are in. The Spaces adapter has never
run against a real bucket. See [file-storage.md](file-storage.md).

## Twelve components, one app

| Component                   | Type                    | Dockerfile                     | Port |
| --------------------------- | ----------------------- | ------------------------------ | ---- |
| `api`                       | Web Service             | `docker/api.Dockerfile`        | 8000 |
| `backoffice`                | Web Service             | `docker/backoffice.Dockerfile` | 3000 |
| `webapp`                    | Web Service             | `docker/webapp.Dockerfile`     | 3001 |
| `migrate`                   | Job, `kind: PRE_DEPLOY` | `docker/api.Dockerfile`        | —    |
| `quote-correction-learning` | Job, `kind: SCHEDULED`  | `docker/api.Dockerfile`        | —    |
| `quote-quality-evaluation`  | Job, `kind: SCHEDULED`  | `docker/api.Dockerfile`        | —    |
| `attachment-extraction`     | Job, `kind: SCHEDULED`  | `docker/api.Dockerfile`        | —    |
| `catalog-embedding`         | Job, `kind: SCHEDULED`  | `docker/api.Dockerfile`        | —    |
| `quote-auto-archive`        | Job, `kind: SCHEDULED`  | `docker/api.Dockerfile`        | —    |
| `conversation-batches`      | Job, `kind: SCHEDULED`  | `docker/api.Dockerfile`        | —    |
| `quote-followup`            | Job, `kind: SCHEDULED`  | `docker/api.Dockerfile`        | —    |
| the database                | Managed Postgres        | —                              | —    |

**Every component takes `source_dir: /`.** All three Dockerfiles build with the repository root as
their context — `docker-compose.yml` says `context: .`, and the web ones copy `pnpm-workspace.yaml`,
`turbo.json` and `packages/` — so a component pointed at `apps/api` fails at the first `COPY`. This
is the most likely first failure and it is the cheapest one to avoid.

The api image carries **four** binaries and the migration chain: `/api/bin/api`,
`/api/bin/scheduled-job`, `/api/bin/catalog-embed`, `/api/bin/goose`, and `/api/migrations`. That is
why one Dockerfile serves nine components.

**Instance sizing.** The spec uses `apps-s-1vcpu-1gb` (1 shared vCPU, 1 GiB, $12/month). Nothing has
been load-tested; it is the smallest size worth starting a Next.js server on, and the number the
cost estimate is built from. Jobs are billed only for the time they run.

## One domain, two hostnames

The backoffice and the public quote link answer on separate hostnames: the backoffice on the
primary domain, the webapp on `cotizacion.<domain>`. One registrar domain buys both: a subdomain
costs nothing on top of it, DigitalOcean DNS hosting is free, App Platform offers two public ingress
IPs free of charge, and a certificate is provisioned per hostname automatically. It stays one app
and one bill.

The primary domain also serves the public site — the landing at `/`, the privacy policy and the
terms — from the backoffice itself, so its pages share an origin with the session and can tell a
seller from a visitor (`docs/technical/public-site.md`). The webapp answers `noindex` and a robots
file that disallows everything: every page on it is one customer's quote.

**The session cookies are the reason.** `coti_access_token`, `coti_refresh_token` and `coti_branch`
are set with no `domain` attribute (`apps/backoffice/lib/auth/tokens.ts`), so a browser scopes them
to the exact host that set them and never sends them to a sibling. The client opening a
`quote_send.public_token` link therefore never carries a backoffice session, and the browser is what
guarantees it rather than anything in the code. A path under one domain puts the two on one origin
and turns the same separation into something the code has to keep getting right.

**The committed spec routes by path, and stays that way.** It names no hostname — the repository is
public — and path rules are what a fresh `doctl apps create` needs. So a first deploy runs on the
default `*.ondigitalocean.app` URL, a hostname that does not exist until the app does. Once a domain
is attached, the app's `ingress` becomes the block below, applied through the control panel or
`doctl apps update --spec` and never committed — so re-applying `.do/app.yaml` wholesale puts the
app back on path routing.

```yaml
domains:
  - domain: example.com
    type: PRIMARY
    zone: example.com
  - domain: cotizacion.example.com
    type: ALIAS
    zone: example.com

ingress:
  rules:
    - match:
        authority:
          exact: cotizacion.example.com
        path:
          prefix: /
      component:
        name: webapp
    - match:
        authority:
          exact: example.com
        path:
          prefix: /api/v1
      component:
        name: api
        rewrite: /v1/
    - match:
        authority:
          exact: example.com
        path:
          prefix: /api/ready
      component:
        name: api
        rewrite: /ready
    - match:
        authority:
          exact: example.com
        path:
          prefix: /api/health
      component:
        name: api
        rewrite: /health
    - match:
        authority:
          exact: example.com
        path:
          prefix: /
      component:
        name: backoffice
```

Every rule names the host it answers on, so **no rule order can route a `cotizacion.` request to
the backoffice** — which is the point, because the App Platform reference defines no order. On the
primary domain the versioned API and health prefixes overlap `/`, as in the committed spec. The default
`*.ondigitalocean.app` URL matches no rule at all once this is applied.

**`zone` is what makes DigitalOcean own the DNS**, and it needs the registrar's nameservers pointed
at `ns1`/`ns2`/`ns3.digitalocean.com` first; keeping DNS elsewhere means a CNAME per hostname to the
app's default domain, or App Platform's A records at an apex whose provider will not flatten one.
A domain that already carries a CAA record has to authorise both `letsencrypt.org` and `pki.goog`,
or the certificate is never issued.

**The API keeps `/api` on the primary domain.** `${APP_URL}` resolves to whichever domain is
`PRIMARY`, so `STORAGE_LOCAL_API_BASE_URL` and `WEB_BACKOFFICE_URL` follow the custom domain with
nothing to change — which is why `PRIMARY` belongs to the backoffice rather than the webapp. The
API declares no CORS, so whatever the webapp needs from it is fetched from its own server, the way
the backoffice already proxies every call.

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
run_command: /bin/sh -c 'exec /api/bin/goose -dir /api/migrations postgres "$DATABASE_ADMIN_URL" up'
```

**Not on API startup.** Startup migration races across instances, and a bad migration takes the app
down instead of failing the deploy. A `PRE_DEPLOY` job runs before the new containers take traffic,
once, and a failure stops the rollout with the old version still serving.

App Platform parses the run command into arguments without expanding environment variables.
The explicit shell expands the owner URL inside the container; `exec` preserves exit and signal handling.

The job needs `DATABASE_ADMIN_URL` and nothing else. goose is pinned as a tool dependency in
`apps/api/go.mod`, so the image's copy and the one `pnpm db:migrate` uses locally are the same
version by construction.

## Two gotchas that cost an afternoon

**`NEXT_PUBLIC_API_URL` is baked into the JavaScript bundle.** It is a build `ARG` in both web
Dockerfiles, so on App Platform it must be scoped **`RUN_AND_BUILD_TIME`**. Set as run-time only,
the deployed frontends call `http://localhost:8000` from the visitor's browser and there is no
server-side error to find.

`API_INTERNAL_URL` is runtime-only and used by the webapp server for its own API requests. In
Docker Compose it is `http://api:8000`; on App Platform it is `${APP_URL}/api`.

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

Each is a `type: SECRET` entry in the spec with no committed value. The five `STORAGE_*` bucket keys
are declared on `api` and on `attachment-extraction`, two of them credentials and the other three
kept out of a public file: `STORAGE_ENDPOINT`, `STORAGE_REGION` (`us-east-1` whichever datacenter
holds the Space) and `STORAGE_BUCKET`.

**The API needs the first four; the correction-learning job also needs the OpenAI key.**
`config.Load()` refuses to start when a capability is switched on and its credential is empty, so
the committed spec ships every optional one **off** — `AI_*_PROVIDER` at `disabled`,
`MAIL_PROVIDER` at `console`, `STORAGE_PROVIDER` at `local`, `RATE_LIMIT_TRUSTED_PROXY_HOPS` at
`0`. Fill a secret, then turn its capability on; doing it the other way round produces a deploy
that never starts, and `ci.deploy-spec.yml` boots the image on the spec's own settings to keep that
true.

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
| `STORAGE_ACCESS_KEY`            | `STORAGE_PROVIDER=spaces`, on `api` and `attachment-extraction`    | Spaces key                          |
| `STORAGE_SECRET_KEY`            | `STORAGE_PROVIDER=spaces`, on `api` and `attachment-extraction`    | Spaces secret                       |

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
`STORAGE_PROVIDER=spaces` on `api` and `attachment-extraction` together,
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
The customer webapp is the same case: its `WEB_TRUSTED_PROXY_HOPS` decides whether it forwards the
visitor's address, and until the API trusts it every portal customer shares one bucket on the
per-address limits — customer login and the contact verification and profile routes alike.

The full list of keys, with defaults and what each bounds, is in the four `.env.example` files.

## First deploy, in order

1. Create the **Managed Postgres** cluster (PG 16 or 17). Nothing else can be done first.
2. `CREATE ROLE coti_app …` on it, as above. Doing this **before** anything deploys is what makes
   the first deploy succeed rather than fail at the api component.
3. **Merge `dev` into `main`** — repo-side, so any time before the app is created. All eight
   components built from the repository carry `github.branch: main` with `deploy_on_push: true`, so
   the platform builds whatever `main` holds: the spec you apply and the code that gets built come
   from different places, and between releases `main` trails `dev`.
4. Create the app from `.do/app.yaml`, filling in the database component's `cluster_name`, `db_name`
   and `db_user`, both database URLs, `AUTH_JWT_SECRET`, `STORAGE_LOCAL_SIGNING_SECRET`, and
   `AI_OPENAI_API_KEY` for the scheduled correction retry. `NEXT_PUBLIC_API_URL` is still a
   placeholder.
5. The `migrate` PRE_DEPLOY job applies the chain as `doadmin`; the grants and RLS policies land on
   the role from step 2, whose password it leaves alone.
6. Attach the domain, if there is one, and apply the two-hostname block from
   [One domain, two hostnames](#one-domain-two-hostnames) **to the app, not to `.do/app.yaml`**.
   Before the next step, not after it: `NEXT_PUBLIC_API_URL` is baked into the bundle, so a domain
   attached afterwards leaves the frontends calling the platform URL until another rebuild.
7. Read the app's URL — the primary domain, once one is attached — set `NEXT_PUBLIC_API_URL` to
   `<url>/api` **on both web components**, and redeploy so the frontends rebuild around it. Setting
   it on one leaves the other calling `http://localhost:8000` from the visitor's browser.
   `STORAGE_LOCAL_API_BASE_URL` and `WEB_BACKOFFICE_URL` need no second pass — they are bound to
   `${APP_URL}` and resolve at runtime.
8. Fill the optional secrets and flip their switches: the Spaces bucket (before the first file
   order), mail, then the two AI vendors, then the rate-limit proxy pair. Each is a restart, not a
   rebuild. `AI_EMBEDDINGS_PROVIDER` is declared on `api` and on `catalog-embedding`: turn it on on
   both, or the job embeds nothing.
   `attachment-extraction` declares no AI keys at all and runs on the defaults, which are off: the
   language model, embeddings and their keys have to be added to it too, or a file order the sweep
   picks up fails at extraction even with AI on everywhere else.
9. Register the first account and let `catalog-embedding` embed its catalog within 15 minutes, or
   run `/api/bin/catalog-embed --account <uuid>` from a console on the api component to do it now
   (it is also what `--refresh-all` after a model change needs), and build the vector index once
   there are rows
   (`pnpm db:vector-index`, see [catalog.md](catalog.md)). It is deliberately not in the chain: on
   an empty table an ivfflat index is degenerate.
10. Confirm that each scheduled job runs every 15 minutes and that an empty run exits successfully
    without calling a provider.

## What CI already proves

`.github/workflows/ci.docker.yml` builds all three images on every PR that touches them or what they
copy, and boots every one of them. The api job goes further: it asserts the four binaries are in the
image and applies the whole migration chain **from inside it** against a real Postgres, which is the
PRE_DEPLOY job's exact command.

`.github/workflows/ci.deploy-spec.yml` covers the spec itself: `doctl apps spec validate
--schema-only`, which needs no account, plus the four things a schema check cannot see — that no
`type: SECRET` entry carries a value, that every component repeating `ARCA_ENVIRONMENT` carries the
api service's value, that every component builds `/` with a Dockerfile that exists, and that every
ingress rule names a component the spec declares. Its second job **boots the api image on the spec's
own settings**, with only those four secrets filled, and requires `/ready`. That is the one check
that catches a capability switched on beside an empty credential, which is a deploy that never
starts and looks like a valid spec until it is applied.

So the image half of a deploy is continuously verified, and the spec is checked for the mistakes
that are checkable offline. What remains unverified is the platform half — whether the app the spec
describes actually comes up — and that cannot be checked without an app.

## Enabling self-service customer subdomains

Development needs only the local API, backoffice on port 3000, and shared webapp on port 3001.
CUSTOMER_PORTAL_BASE_URL defaults to http://localhost:3001 in development and to an empty value
(disabled) in production. The browser receives it from the authenticated API; it never supplies an
arbitrary URL to save. NEXT_PUBLIC_PORTAL_AVAILABILITY_DELAY_SECONDS on the backoffice defaults to
1 and controls only the advisory availability debounce. Set CUSTOMER_GOOGLE_LOCAL_ORIGIN to
http://localhost:3001 for local supplier links and the development Google callback relay.

Production activation is a separate release operation. Obtain a domain, configure wildcard DNS and
HTTPS for its customer subdomains, preserve Host while routing all of them to the same shared
webapp, and verify customer cookie isolation. Only then set CUSTOMER_PORTAL_BASE_URL to the HTTPS
base origin (no path or port). Its presence asserts that wildcard infrastructure is ready; leave it
empty while using temporary platform URLs. Configure Google callback URIs per supplier before
enabling that sign-in method. No application or deployment is created per supplier.

Existing temporary hostnames remain associated with their current account and keep working while
self-service is disabled. The portal editor displays them as inherited, with no self-service
replacement. Do not replace a temporary hostname as part of enabling the base domain: coordinate
that migration with customers and link/callback owners first. Nothing in the development workflow
changes DigitalOcean settings, main, DNS, deployed data or storage.
