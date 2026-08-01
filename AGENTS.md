# Coti — Codex context

Coti is a B2B SaaS for AI-assisted quoting at mid-sized building-materials
suppliers ("corralones") in Argentina. It ingests informal RFQs (WhatsApp,
email, audio, photo, PDF, Excel, public web link), runs an AI pipeline that
extracts line items and matches them against the supplier's catalog + pricing,
and returns a review-ready quote. Human review is always in the flow — a
copilot, not an autopilot. (UTN FRBA final project, K5053 Grupo 5, 2026.)

Monorepo: `apps/backoffice` (Next.js, authenticated), `apps/webapp` (Next.js,
public), `apps/api` (Go + Gin, layered), `packages/ui` (shared shadcn design
system).

## Start here

Read the `agent-workflow` skill first when doing substantive local work. Codex
skills live under `.agents/skills/`; Claude skills remain under `.claude/skills/`.
The two trees are kept byte-for-byte identical.

## Where the knowledge lives (source hierarchy)

`docs/internal/` is the local source of truth for what the code needs; read the
relevant doc before implementing. It is **gitignored** — synced with Notion and
shared with the team out of band (not via git), so the Claude web Project and
every teammate's checkout (and their agents) share one source of truth without
internal planning entering git history. **If your checkout lacks `docs/internal/`,
get it from the team or Notion.** Committed project docs go in `docs/public/` and
`docs/technical/`.

One-directional flow: **decision (conversation / Notion) → `docs/internal/` → code.**

- Product/architecture decisions: live source is Notion; `docs/internal/product/decisiones-cerradas.md` is a local mirror — **Notion wins** on divergence.
- Work state (tickets): lives in Notion, not replicated here.
- Executable data model: the goose migrations in `apps/api/migrations/`, with `apps/api/database/01_create_tables.sql` as the consolidated reference for the current shape. `docs/internal/data/schema.sql` no longer holds DDL — it keeps the ES↔EN mapping and the pending DER changes.

Map: `product/`, `domain/`, `architecture/`, `data/`, `conventions/`. Start at `docs/internal/README.md`.

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

## Product invariants (the code MUST enforce these)

Hard rules, not style. If a task asks you to violate one, stop and flag it.

- AI never writes to prod: AI proposes → backend validates → seller approves.
- AI never calculates money — the deterministic discount engine does.
- AI never initiates client contact; it only drafts for the seller.
- Every AI proposal is validated against the state×intention matrix before it materializes (`docs/internal/domain/estados.md`).
- AI output uses a forced schema (structured outputs / tool use), never prompt-requested JSON.
- `quote.current_status` is backend-exclusive, recomputed on each transition; never edited by a human or the AI.
- Unmatched items are flagged (`quote_item.match_status = NO_MATCH`), never discarded.
- Multi-branch isolation is validated on every input.
- 1-to-1 rfq→quote: never create a second quote for the same RFQ.

## Tickets

Work is tracked in **Notion**. A PR links its Notion ticket page URL as the Card
Link and pulls acceptance criteria from it. See `pr-format`.

## Architecture rules

- Use API skills for backend work: `api-layering` and `api-methods-entities`.
- Use web skills for frontend work: `web-structure` and `web-components-pages` (both cover `apps/backoffice` and `apps/webapp`).
- Layered Go API (ports & adapters): `internal/{ai,config,delivery/http,domain,repository,services}`. Services depend only on ports; adapter wiring only in `cmd/api/main.go`.
- English identifiers; native PostgreSQL enums with English UPPERCASE values (labels are frontend i18n).
- Persistence is raw `database/sql` + `pgx` (no ORM); SQL always parameterized. UUID v4 PKs.
- Money & quantities are `NUMERIC(14,2)` — decimal strings end to end, never float or int64 centavos.
- Services own transaction boundaries; repositories never commit.
- Every query is scoped by `account_id` (and `branch_id` for branch-scoped tables) — cross-account exposure is a P0 bug.
- Avoid N+1 queries; add batch repository methods when data is needed in loops.
- Semantic catalog search uses `product.embedding` (pgvector); embedding generation lives behind the `internal/ai` provider. The catalog is account-scoped; per-branch availability and stock live in `branch_product`.
- Multi-tenancy is enforced twice: `account_id` on every tenant-scoped table plus Postgres RLS under a `NOBYPASSRLS` role.
- API comments above function/type definitions end with periods.
- Web code uses the `@repo/ui` design tokens; avoid raw typography classes when tokens exist.
- Configurable thresholds are env-var-backed with defaults in `apps/api/internal/config` or app `lib/config.ts`. No hardcoded thresholds in business logic.
- Duration env vars carry their unit in the key (`_SECONDS`/`_MINUTES`/`_HOURS`/`_DAYS`) so the value is a plain integer; a duration key without a suffix is a config error. Config validation reports every problem at once.
- Migrations are the only executable path: a schema change ships a goose migration in `apps/api/migrations/` AND updates the consolidated reference schema under `apps/api/database/` in the same PR. `pnpm db:init` builds a fresh DB by running the migrations; the reference schema is read, never applied.

## Conventions & hard rules

- The whole codebase is English — code, comments, SQL, `docs/technical/`, `docs/public/`, READMEs, scripts, CI, commits and PRs. Two exceptions, different in kind: UI copy is Argentine Spanish via next-intl (`es-AR`, single locale) because it _is_ the product, and `docs/internal/` is Spanish because it is the academic material that syncs with Notion. Never mix two languages inside one file or one generated document.
- Comments earn their place: one line, two if genuinely needed, for a non-obvious _why_, an arbitrary-looking constraint, or a footgun. Go doc comments on exported symbols stay but stay to one line. Never narrate rejected alternatives, tell a bug's story, restate the signature, or describe how something used to be. When in doubt, leave it out.
- "MVP" is banned — it's "Release 1 — Piloto Comercial".
- GitFlow: `main` ← `dev` (default) ← ephemeral `feat/`, `fix/`, `enhancement/`, `refactor/`, `hotfix/` (kebab-case). Commits `type: imperative description`.
- Never `git add .`/`-A`; stage files individually by name.
- Never stage `.claude/plans/`, `.claude/settings*.json`, `docs/internal/`, or stray temporary markdown.
- PRs target `dev` (never `main`, except `hotfix/*`); each carries a label and needs one approval.
- The Husky pre-commit hook runs `lint-staged` only (no type/build checks). Run `pnpm check` and `pnpm test:api` before pushing.

## Review guidelines

- Flag P0/P1 issues only unless the PR asks for a broader review.
- Treat cross-account data leakage, auth/session regressions, data-loss bugs, broken schema updates, unparameterized SQL, transaction mistakes, and any violation of the product invariants as high priority.
- Check backend layer boundaries, repository/service method order, raw-SQL parameterization, `account_id`/`branch_id` scoping, and that repositories never commit.
- Check frontend routing, design tokens, component ordering, the snake_case↔camelCase API boundary, and shared UI reuse across both apps.
- If schema changed, verify the migration chain still rebuilds the DB from zero and the reference schema matches it.
- If behavior, setup, or public API changed, verify the relevant docs were updated.
- Treat missing tests as high priority when calculations, persistence, auth, API contracts, or shared UI behavior changed.

## Working a task

1. Read the `docs/internal/` doc(s) the task references.
2. Respect the product invariants.
3. Branch with the right prefix; commits in format; PR to `dev`; tests included.
4. If a task contradicts a closed decision or an invariant, stop and flag it before coding.

The team speaks Argentine Spanish, so match that when talking to them; everything written into the repo is English (see Conventions & hard rules).
