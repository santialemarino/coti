# Coti — Codex context

Coti is a B2B SaaS for AI-assisted quoting at mid-sized building-materials
suppliers ("corralones") in Argentina. It ingests informal RFQs (WhatsApp,
email, public web link), runs an AI pipeline that extracts line items and
matches them against the supplier's catalog + pricing, and returns a
review-ready quote. Human review is always in the flow — a copilot, not an
autopilot.

Monorepo: `apps/backoffice` (Next.js, authenticated), `apps/webapp` (Next.js,
public), `apps/api` (Go + Gin, layered), `packages/ui` (shared shadcn design
system).

## Start here

Read the `agent-workflow` skill first when doing substantive local work. Codex
skills live under `.agents/skills/`; Claude skills remain under
`.claude/skills/`. The two trees are kept byte-for-byte identical.

## Key commands

```bash
pnpm dev
pnpm dev:docker
pnpm build
pnpm db:init
pnpm db:migrate
pnpm lint
pnpm lint:fix
pnpm format
pnpm format:check
pnpm check
pnpm check:api
pnpm check:web
pnpm test:api
```

API-specific (run from `apps/api/` or via `pnpm --filter api ...`):

```bash
go run ./cmd/api
pnpm check:api
pnpm test:api
gofmt -w .
```

## Canonical docs

- Repo overview and quickstart: `README.md`.
- Design source of truth: `docs/internal/architecture.md` (gitignored).
  Schemas, endpoint shapes, the AI pipeline, conventions, and the Go package
  layout live there.
- Decisions log: `docs/internal/decisions.md` (gitignored).
- Public docs: `docs/public/` (product/solution).
- Technical docs: `docs/technical/` (env vars, Docker, auth flow, data model,
  AI/RFQ pipeline).
- Internal docs (`docs/internal/`) are gitignored and must not be committed.

## Tickets

Work is tracked in **Notion**. A PR links its Notion ticket page URL as the Card
Link and pulls acceptance criteria from it. See `pr-format`.

## Hard rules

- Never `git add .` or `git add -A`; stage files individually by name.
- Never stage `.claude/plans/`, `.claude/settings.json`, or
  `.claude/settings.local.json`.
- Never stage `docs/internal/`.
- Never stage temporary markdown files unless the user explicitly names them.
- PRs target `dev` (GitFlow), never `main`, except `hotfix/*`. Every PR carries
  a label and needs at least one approval.
- The Husky pre-commit hook runs `lint-staged` only (Prettier + ESLint on
  staged files); it does not run type/build checks. Run `pnpm check` and
  `pnpm test:api` yourself before pushing.

## Architecture rules

- Use API skills for backend work: `api-layering` and `api-methods-entities`.
- Use web skills for frontend work: `web-structure` and `web-components-pages`
  (both cover `apps/backoffice` and `apps/webapp`).
- Layered Go API (ports & adapters):
  `internal/{ai,config,delivery/http,domain,repository,services}`. Services
  depend only on ports (interfaces), never on concrete adapter packages.
  Adapter wiring lives only in `cmd/api/main.go`.
- Persistence is raw `database/sql` + `pgx` (no ORM); SQL is always
  parameterized, never string-built.
- Services own transaction boundaries; repositories never commit.
- Every query is scoped by `cuenta_id` (and `sucursal_id` where relevant) —
  cross-account exposure is a P0 bug.
- Semantic catalog search uses `producto.embedding` (pgvector); embedding
  generation lives behind the `internal/ai` provider.
- Avoid N+1 queries. Add batch repository methods when data is needed in loops.
- API comments above function/type definitions end with periods.
- Web code uses the `@repo/ui` typography token scale; avoid raw typography
  classes when tokens exist.
- Configurable thresholds are env-var-backed with defaults in
  `apps/api/internal/config` (backend) or app `lib/config.ts` (frontend). No
  hardcoded thresholds in business logic.
- Per-phase migrations: a schema change ships a goose migration in
  `apps/api/migrations/` AND updates the canonical `apps/api/database/` schema
  in the same PR.

## Review guidelines

- Flag P0/P1 issues only unless the PR asks for a broader review.
- Treat cross-account data leakage, auth/session regressions, data-loss bugs,
  broken DB schema updates, unparameterized SQL, and transaction mistakes as
  high priority.
- Check backend layer boundaries, repository/service method order, raw-SQL
  parameterization, and that repositories never commit.
- Check frontend routing, typography tokens, component ordering, the
  snake_case↔camelCase API boundary, and shared UI reuse across both apps.
- If schema changes were made, verify migrations represent a clean rebuild from
  zero and the canonical schema matches.
- If behavior, setup, or public API changed, verify the relevant docs were
  updated.
- Treat missing tests as high priority when calculations, persistence, auth,
  API contracts, or shared UI behavior changed.
