# Coti

Coti is a B2B SaaS platform for AI-assisted quoting at mid-sized building
materials suppliers ("corralones") in Argentina. It receives informal purchase
requests (RFQs) over WhatsApp, email, or a public web link, runs them through an
AI pipeline that extracts line items and matches them against the supplier's own
catalog and pricing, and hands the sales rep a structured, ready-to-review quote.

Human review is always in the main flow — Coti is a sales copilot, not an
autopilot.

## Stack

| Layer            | Technology                                             |
| ---------------- | ------------------------------------------------------ |
| Frontend         | Next.js (React 19) — backoffice + customer webapp      |
| Styling          | Tailwind v4 (CSS-first) + the `@repo/ui` design system |
| Backend API      | Go + Gin — REST/JSON, layered architecture             |
| Database         | PostgreSQL + `pgvector` (semantic catalog search)      |
| Monorepo         | Turborepo + pnpm workspaces                            |
| Containerization | Docker / docker-compose                                |
| CI/CD            | GitHub Actions                                         |

## Layout

```
apps/
  backoffice   Next.js — vendor & admin web app (authenticated)
  webapp       Next.js — customer-facing public web app (no auth)
  api          Go + Gin — REST API, business logic, persistence, AI orchestration
packages/
  ui                 Shared design system (@repo/ui) — tokens, type scale, motion,
                     primitives; also the single Tailwind entry for the monorepo
  eslint-config      Shared ESLint flat configs (@repo/eslint-config)
  typescript-config  Shared tsconfig bases (@repo/typescript-config)
docker/        Dockerfiles for each deployable
scripts/       DB init + goose migration helpers
docs/          public/ and technical/ documentation
```

Additional shared packages (types, constants, hooks) are added under
`packages/` as the product grows.

## Prerequisites

- Node.js >= 22 (`.nvmrc` pins 22)
- pnpm 10 (`corepack enable` recommended)
- Go 1.25+ (for `apps/api`)
- Docker (for Postgres and container builds)

## Quickstart

```bash
# 1. Install JS/TS dependencies
pnpm install

# 2. Resolve Go modules
(cd apps/api && go mod tidy)

# 3. Configure environment
cp .env.example .env
cp apps/api/.env.example apps/api/.env
cp apps/backoffice/.env.example apps/backoffice/.env
cp apps/webapp/.env.example apps/webapp/.env

# 4. Start Postgres (with pgvector) and migrate the schema to head
pnpm db:init

# 5. Run everything in dev
pnpm dev
```

| App        | URL                   |
| ---------- | --------------------- |
| backoffice | http://localhost:3000 |
| webapp     | http://localhost:3001 |
| api        | http://localhost:8000 |

Outbound mail goes to the log by default. To read a real message — a password-reset or
address-verification link — start the Mailpit sandbox, set `MAIL_PROVIDER=smtp` in
`apps/api/.env` (its `MAIL_SMTP_*` keys already point at Mailpit), and open the inbox:

```bash
docker compose up -d mailpit   # included in pnpm dev:docker
```

| Service      | URL                   |
| ------------ | --------------------- |
| mailpit UI   | http://localhost:8025 |
| mailpit SMTP | localhost:1025        |

## Common scripts

| Command                           | What it does                                |
| --------------------------------- | ------------------------------------------- |
| `pnpm dev`                        | Build `@repo/ui`, then run all apps (Turbo) |
| `pnpm build`                      | Build all apps and packages                 |
| `pnpm lint`                       | Lint all workspaces                         |
| `pnpm check`                      | Type-check the API and web apps             |
| `pnpm dev:docker`                 | Bring up the full stack via docker-compose  |
| `pnpm db:migrate`                 | Apply Go (goose) migrations                 |
| `pnpm db:create-migration <name>` | Scaffold a new migration                    |
| `pnpm db:vector-index`            | Build the catalog's vector index            |
| `pnpm docs:api`                   | Regenerate the OpenAPI spec from handlers   |
| `pnpm test:rfq`                   | Run the RFQ engine unit suite verbosely     |
| `pnpm test:rfq:response`          | Print one representative RFQ JSON response  |

### RFQ engine tests

The fast RFQ suite uses fixed AI doubles and needs no credentials or database:

```bash
pnpm test:rfq
pnpm test:rfq:response
```

The integration suite exercises real PostgreSQL, pgvector, tenant isolation, hybrid matching,
draft persistence, and quote valuation. Both database roles are required; without them the tests
skip themselves and print the reason.

```bash
TEST_DATABASE_URL=postgres://coti_app:coti_app@127.0.0.1:5432/coti?sslmode=disable \
TEST_DATABASE_ADMIN_URL=postgres://coti:coti@127.0.0.1:5432/coti?sslmode=disable \
  pnpm test:rfq:integration
```

## API specification

Generated from the handler annotations with swaggo/swag and committed under
`apps/api/docs/`; CI regenerates and fails on a diff, so the annotations stay
verified rather than decorative. With the API running, the UI is at
http://localhost:8000/swagger/index.html — it is not served in production.

See [docs/technical/api-specification.md](docs/technical/api-specification.md).

## Database

Two connection roles. `DATABASE_URL` is the restricted, `NOBYPASSRLS` role the API
uses for request-scoped queries; `DATABASE_ADMIN_URL` is the owner, used by goose
migrations, the operational scripts, the follow-up cron, and the pre-auth lookups
that legitimately span accounts. Every tenant-scoped table carries `account_id` and
enforces it with a row level security policy reading a per-transaction GUC, so a
query missing its predicate returns zero rows rather than another tenant's data.

Closing or reopening a corralón is an operational script rather than an endpoint:
`pnpm db:account:deactivate --account <uuid>` cuts every session in the account, and
`pnpm db:account:activate --account <uuid>` restores it.

See [docs/technical/database.md](docs/technical/database.md).

## AI providers

Three ports in `apps/api/internal/domain` — schema-forced generation, embeddings and
transcription — with adapters under `apps/api/internal/ai/`, one subpackage per provider, bound
in `apps/api/internal/ai/provider` and nowhere else. Each capability is selected on its own (no
provider covers all three) and each arrives disabled, so a checkout with no keys still boots and
the engine refuses the calls that needed a model. Turning one on makes its key required at
startup.

See [docs/technical/ai-providers.md](docs/technical/ai-providers.md).

## The RFQ pipeline

`POST /v1/rfqs/text-drafts` turns one informal order into a quote a seller can review: the text is
stored first, then read into materials with a forced schema, then matched against the branch's
catalog, then written as a `DRAFT` quote with one line per material. A material the message gives no
defensible quantity for comes back on the schema's escape value and is still written to the quote —
flagged for the seller, never dropped. `GET /v1/channels` lists the branch's intake routes, and a
development-only route simulates an inbound WhatsApp message through the same pipeline.

See [docs/technical/rfq-pipeline.md](docs/technical/rfq-pipeline.md).

## File storage

One port in `apps/api/internal/domain` — upload, download, signed link — with adapters under
`apps/api/internal/storage/`, bound in `apps/api/internal/storage/provider` and nowhere else.
`STORAGE_PROVIDER` selects one: `local` keeps objects on the filesystem and serves them through
the API's own signed-link route, `spaces` stores them in an S3-compatible bucket that signs and
serves its own. `local` is the default and it genuinely works, so a checkout with no bucket
still stores, signs, serves and expires a file; selecting `spaces` makes its five credentials
required at startup. Object keys are canonical, relative paths and both adapters refuse anything
else, so an object stored through one stays reachable through the other.

See [docs/technical/file-storage.md](docs/technical/file-storage.md).

## Design system

`packages/ui` (`@repo/ui`) holds the tokens, type scale and motion vocabulary both web
apps consume. Its `src/styles/index.css` is the single Tailwind entry for the monorepo,
compiled to `dist/index.css`; each app's `globals.css` imports only that, so preflight
is emitted once. The colour ramp is derived from the logo — the wordmark ink, the dot
over the i, and both stops of the isotype gradient are exact tokens.

`@repo/ui` ships its CSS prebuilt, so rebuild it after changing a component's
classNames (`pnpm --filter @repo/ui build`, or its `dev` watcher). `pnpm dev` does that
first, so a cold start can't race it.

See [docs/technical/design-system.md](docs/technical/design-system.md).

## Branching & commits

GitFlow (simplified): `main` (production) ← `dev` (integration) ← ephemeral
`feat/*`, `fix/*`, `enhancement/*`, `refactor/*`, `hotfix/*` branches
(lowercase kebab-case). Commits follow `type: imperative description`
(`feat`, `fix`, `refactor`, `docs`, `chore`). No direct pushes to `main` or
`dev`; every PR carries a label and needs at least one approval.

## License

Proprietary — see [LICENSE](./LICENSE). All rights reserved.
