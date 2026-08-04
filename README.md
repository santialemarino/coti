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
| `pnpm docs:api`                   | Regenerate the OpenAPI spec from handlers   |

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
