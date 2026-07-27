# Coti — Claude context

Coti is a B2B SaaS for AI-assisted quoting at mid-sized building-materials
suppliers ("corralones") in Argentina. It ingests informal RFQs (WhatsApp,
email, audio, photo, PDF, Excel, public web link), runs an AI pipeline that
extracts line items and matches them against the supplier's own catalog +
pricing, and hands the sales rep a structured, review-ready quote. Human review
is always in the flow — a copilot, not an autopilot. (UTN FRBA final project,
K5053 Grupo 5, 2026.)

Monorepo: `apps/backoffice` (Next.js, authenticated), `apps/webapp` (Next.js,
public), `apps/api` (Go + Gin, layered), `packages/ui` (shared shadcn design
system). See `README.md` for the full command list.

## Start here

Always read the **agent-workflow** skill first. It tells you which other skills
to load and covers checks, docs, the skill mirror, and commit rules.

## Where the knowledge lives (source hierarchy)

**`docs/internal/` is the local source of truth for what the code needs.** Before
implementing something, read the relevant doc there. It is **gitignored** — kept
in sync with Notion and shared with the team out of band (direct handoff /
Notion), not via git. This is deliberate: the Claude web Project and every
teammate's local checkout (and their agents) work from the same source of truth
without internal planning entering git history. **If your checkout doesn't have
`docs/internal/`, get it from the team or Notion.** Project docs that _are_
committed live in `docs/public/` and `docs/technical/` (for later).

Flow is one-directional: **decision (conversation / Notion) → `docs/internal/` → code.**

- **Product/architecture decisions** — live source is Notion ("Decisiones de
  producto y arquitectura cerradas"); `docs/internal/product/decisiones-cerradas.md`
  is a local mirror. If they diverge, **Notion wins**.
- **Work state (tickets)** — lives in Notion, not replicated here. A ticket points
  _to_ `docs/internal/`, not the other way around.
- **Executable data model** — the goose migrations in `apps/api/migrations/`.
  `docs/internal/data/schema.sql` is the design reference the migrations are built from.

`docs/internal/` map: `product/` (what it is, scope, nomenclature, closed
decisions), `domain/` (business rules: states, discount engine, AI pipeline,
follow-up, ingest), `architecture/` (stack, layers, technical decisions),
`data/` (`schema.sql`, physical-model notes, DER), `conventions/` (git, code,
glossary). Start at `docs/internal/README.md` if unsure.

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
pnpm db:init      # start Postgres (+pgvector) and migrate the schema to head
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

## Product invariants (the code MUST enforce these)

Not style — hard rules. If a task asks you to violate one, stop and flag it.

- **AI never writes to prod.** Chain: AI proposes → backend validates → seller approves.
- **AI never calculates money.** Amounts come from the deterministic discount engine.
- **AI never initiates client contact.** It only produces drafts for the seller.
- **Every AI proposal is validated against the state×intention matrix** before it
  materializes (`docs/internal/domain/estados.md`).
- **AI output uses a forced schema** (structured outputs / tool use), never JSON
  requested in a prompt.
- **`quote.current_status` is backend-exclusive**, recomputed on each transition.
  Never edited by a human or the AI.
- **Unmatched items are flagged** (`quote_item.match_status = NO_MATCH`), never discarded.
- **Multi-branch isolation** is validated on every input.
- **1-to-1 rfq→quote:** never create a second quote for the same RFQ.

## Architecture rules

- **Layered Go API (ports & adapters).** Backend code lives under
  `internal/{ai,config,delivery/http,domain,repository,services}`. Request flow is
  `handler → service → repository → DB`. Services depend only on interfaces (ports)
  — the AI provider, repositories — never on concrete adapter packages. Adapter
  wiring lives only in `cmd/api/main.go`. See `api-layering`.
- **English identifiers, native PG enums.** Tables/columns/types/functions/endpoints
  are English (`account`, `branch`, `app_user`, `product`, `quote`, `rfq`, …); docs
  are Spanish and cite identifiers in English. Enums are native PostgreSQL types with
  **English UPPERCASE** values (labels are frontend i18n only).
- **Raw SQL, no ORM.** Persistence uses `database/sql` + `pgx`/`pgxpool`. SQL is always
  parameterized (`$1`), never string-built. Rows scanned explicitly into domain structs.
- **UUID v4 PKs** (`gen_random_uuid()`), no autoincrement.
- **Money & quantities are `NUMERIC(14,2)`** — handle as decimal strings end to end
  (never float, never int64 centavos).
- **Service-owned transactions.** Services open/commit (`pool.Begin()` →
  `tx.Commit()`/`tx.Rollback()`); repositories take a querier/tx and **never** commit.
- **Multi-tenancy is a hard rule, enforced twice.** Every tenant-scoped table carries
  `account_id` — child tables included — and every query filters by it (plus `branch_id` on
  branch-scoped tables: `channel`, `rfq`, `quote`, `combo`, `branch_product`, `product_price`).
  Postgres RLS is the second net: the API connects as a `NOBYPASSRLS` role, so a query missing
  its `account_id` predicate returns zero rows instead of another tenant's data. Cross-account
  exposure is a P0 bug.
- **The catalog is account-scoped.** `product`, `product_synonym`, and `product_alternative`
  belong to the account; per-branch availability and stock live in `branch_product`, price in
  `product_price`. One product row, one embedding, per account.
- **Semantic catalog search via pgvector.** Matching uses `product.embedding`
  (`VECTOR(1536)`) with distance operators; `product_synonym` improves it; embedding
  generation lives behind the `internal/ai` provider. See `api-layering`.
- **Configurable thresholds.** Numeric values with operational meaning (limits,
  intervals, sizes, TTLs — e.g. quote expiry, default 7 days) are env-var-backed with
  defaults in `apps/api/internal/config` (backend) or app `lib/config.ts` (frontend).
  No hardcoded thresholds in business logic; cryptographic constants are exempt.
- **A duration env var carries its unit in the key name** — `_SECONDS`, `_MINUTES`,
  `_HOURS`, or `_DAYS` — so the env file holds a plain integer
  (`AUTH_ACCESS_TTL_MINUTES=15`). `internal/config` derives the multiplier from the
  suffix and treats a duration key without one as a configuration error.
- **Config validation reports every problem at once**, not the first, so a bad deploy
  is diagnosed in one pass instead of one restart per typo.
- **Migrations are the only executable path.** A schema change ships a goose migration
  in `apps/api/migrations/` AND updates the consolidated reference schema under
  `apps/api/database/` in the same PR. Only migrations ever touch a database —
  `pnpm db:init` builds a fresh DB by running them, so local dev exercises the same
  chain as production. The reference schema is what humans and agents read to know the
  current shape; it is never applied.

## Conventions

- **Docs Spanish, code English.** UI copy is Argentine Spanish via next-intl
  (`es-AR`, single locale). Commits and PRs are in English.
- **Git:** GitFlow (simplified) — `main` (prod) ← `dev` (integration, default) ←
  ephemeral `feat/`, `fix/`, `enhancement/`, `refactor/`, `hotfix/` (kebab-case).
  Commits `type: imperative description`. See `commit` / `pr-format`.
- **"MVP" is banned.** The deliverable is "Release 1 — Piloto Comercial" /
  "Producto v1.0". See `docs/internal/product/nomenclatura.md`.

## Hard rules

- Never `git add .` or `git add -A` — stage files individually by name.
- Never stage `.claude/plans/`, `.claude/settings.json`, or `.claude/settings.local.json`.
- Never stage `docs/internal/` — it is gitignored (internal, Notion-synced).
- Never stage temporary `.md` files unless the user explicitly names them.
- PRs target `dev` (GitFlow), never `main` — except `hotfix/*`. Every PR carries a
  label and needs at least one approval. See `pr-format`.
- The Husky pre-commit hook runs `lint-staged` only (Prettier + ESLint on staged
  files). It does **not** run type/build checks — run `pnpm check` (and `pnpm test:api`)
  yourself before pushing; don't push code that fails them.

## Working a task

1. Read the `docs/internal/` doc(s) the task references (or find the relevant one).
2. Respect the product invariants above.
3. Branch with the right prefix; commits in format; PR to `dev`; tests included.
4. If a task contradicts a closed decision or an invariant, **stop and flag it** before coding.

## Tone

Direct and technical, no filler. If an approach is wrong or something doesn't add up,
say so and propose the fix — don't patch over a broken design. The team and all
project docs are in Argentine Spanish; match that when writing docs or talking to the team.
