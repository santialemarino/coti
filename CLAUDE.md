# Coti — Claude context

Coti is a B2B SaaS for AI-assisted quoting at mid-sized building-materials
suppliers ("corralones") in Argentina. It ingests informal RFQs (WhatsApp,
email, public web link), runs an AI pipeline that extracts line items and
matches them against the supplier's own catalog + pricing, and hands the sales
rep a structured, review-ready quote. Human review is always in the flow — a
copilot, not an autopilot.

Monorepo: `apps/backoffice` (Next.js, authenticated), `apps/webapp` (Next.js,
public), `apps/api` (Go + Gin, layered), `packages/ui` (shared shadcn design
system). See `README.md` for the full command list.

## Start here

Always read the **agent-workflow** skill first. It tells you which other skills
to load and covers checks, docs, the skill mirror, and commit rules.

## Codex compatibility

Codex uses `AGENTS.md` for always-loaded repository instructions and
`.agents/skills/` for repo-scoped skills. The `.claude/skills/` and
`.agents/skills/` trees are kept **byte-for-byte identical** — when you change a
shared convention, update both and re-verify with `diff` (see agent-workflow).

## Key commands (from repo root)

```bash
pnpm dev          # start backoffice (3000) + webapp (3001) + api (8000)
pnpm dev:docker   # start full stack in Docker (postgres + api + both webs)
pnpm build        # build everything (UI must build before first web dev)
pnpm db:init      # start Postgres (+pgvector) and apply canonical schema
pnpm db:migrate   # apply pending goose migrations
pnpm lint         # ESLint (web) + go vet (api)
pnpm lint:fix     # ESLint auto-fix; API runs gofmt + go vet
pnpm format       # Prettier (web) + gofmt (api)
pnpm format:check # check formatting; fails if unformatted (CI)
pnpm check        # check:api + check:web (run before pushing)
pnpm check:api    # apps/api: go build + go vet
pnpm check:web    # backoffice + webapp: tsc --noEmit (turbo check-types)
pnpm test:api     # apps/api: go test ./...
```

API-specific (run from `apps/api/` or via `pnpm --filter api ...`):

```bash
go run ./cmd/api          # run the API directly
pnpm check:api            # build + vet
pnpm test:api             # go test ./...
gofmt -w .                # format Go files
```

## Canonical docs

- **Repo overview & quickstart:** `README.md`.
- **Design source of truth:** `docs/internal/architecture.md` (gitignored).
  Schemas, endpoint shapes, the AI pipeline, conventions, and the Go package
  layout live there. When implementing, treat it as the spec.
- **Decisions log:** `docs/internal/decisions.md` (gitignored). Why each design
  choice was made and what upgrades are deferred.
- **Public docs:** `docs/public/` (product/solution, anyone-readable).
- **Technical docs:** `docs/technical/` (env vars, Docker, auth flow, data
  model, the AI/RFQ pipeline).

## Architecture rules

- **Layered Go API (ports & adapters).** Backend code lives under
  `internal/{ai,config,delivery/http,domain,repository,services}`. The request
  flow is `handler → service → repository → DB`. Services depend only on
  interfaces (ports) — the AI provider, repositories — never on concrete adapter
  packages. Adapter wiring lives only in `cmd/api/main.go`. See `api-layering`.
- **Raw SQL, no ORM.** Persistence uses `database/sql` + `pgx`/`pgxpool`, not an
  ORM. SQL is always parameterized (`$1`), never string-built. Rows are scanned
  explicitly into domain structs.
- **Service-owned transactions.** Services open and commit transactions
  (`pool.Begin()` → `tx.Commit()`/`tx.Rollback()`); repositories accept a
  querier/tx and **never** commit. Multi-step writes are atomic by default.
- **Multi-tenancy is a hard rule.** Every query is scoped by `cuenta_id`
  (account) and, where relevant, `sucursal_id` (branch). Cross-account data
  exposure is a P0 bug.
- **Semantic catalog search via pgvector.** Product matching uses
  `producto.embedding` (`VECTOR(1536)`) with distance operators; embedding
  generation lives behind the `internal/ai` provider. See `api-layering`.
- **Configurable thresholds.** Numeric values with operational meaning
  (limits, intervals, sizes, TTLs) are env-var-backed with defaults in
  `apps/api/internal/config` (backend) or app `lib/config.ts` (frontend). No
  hardcoded thresholds in business logic; cryptographic constants are exempt.
- **Per-phase migrations.** A schema change ships a goose migration in
  `apps/api/migrations/` AND updates the canonical `apps/api/database/` schema
  in the same PR; the canonical schema stays a clean rebuild-from-zero.

## Hard rules

- Never `git add .` or `git add -A` — stage files individually by name.
- Never stage `.claude/plans/`, `.claude/settings.json`, or
  `.claude/settings.local.json` — gitignored for a reason.
- Never stage `docs/internal/` — internal docs are gitignored.
- Never stage temporary `.md` files unless the user explicitly names them.
- PRs target `dev` (GitFlow), never `main` — except `hotfix/*`. Every PR carries
  a label and needs at least one approval. See `pr-format`.
- The Husky pre-commit hook runs `lint-staged` only (Prettier + ESLint on
  staged files). It does **not** run type/build checks — run `pnpm check` (and
  `pnpm test:api`) yourself before pushing; don't push code that fails them.
