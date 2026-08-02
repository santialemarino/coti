---
name: agent-workflow
description: Orchestrator workflow for agents working in the Coti repo. Read this FIRST; it tells you which skills to load and how to operate (checks, compliance audit, docs, conventions, mirrors).
---

# Agent workflow (Coti repo)

Coti is a B2B, AI-assisted quoting platform for building-materials suppliers in Argentina — a Turborepo + pnpm monorepo (Node >=22, TS 5.9, React 19) with a Go API. **Read this skill before any substantive work.** It is the dispatcher: it routes you to the specialized skills and defines the operating loop every change follows.

## The operating loop

For any non-trivial change:

1. **Load the relevant skills** (section 1) before writing code — so your changes follow repo conventions from the first line, not after review.
2. **Make the change** in the right app/layer.
3. **Run lints / checks / tests** as needed (section 2).
4. **Run the exhaustive pre-finish compliance audit** (section 3) against every changed file. Not optional.
5. **Keep docs and memory current** (section 4).
6. **Commit** per the **commit** skill, and open a PR per **pr-format** when the work is a shippable unit.

## 1. Load skills first

Read and apply the skill(s) that match what you're touching:

| Skill                    | Load when                                                                                                                                                                |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **api-layering**         | Touching `apps/api` — where files go (`internal/delivery/http`, `services`, `repository`, `domain`, `ai`, `config`) and the handler → service → repository request flow. |
| **api-methods-entities** | Touching `apps/api` — method order, Go doc comments, domain structs, DTOs, and raw-SQL query conventions.                                                                |
| **web-structure**        | Touching `apps/backoffice` or `apps/webapp` — where pages, components, and config live; directory layout; `packages/ui` usage.                                           |
| **web-components-pages** | Adding a page or component to either web app — colocation, declaration order, style, comments.                                                                           |
| **commit**               | Creating a commit — message format (`type: imperative description`), types, staging rules.                                                                               |
| **pr-format**            | Opening a pull request — title, body, branch name, base branch, labels.                                                                                                  |
| **testing**              | Writing or running tests — where they live, how to run them, what to cover.                                                                                              |

Use the **api-\*** skills for the backend, the **web-\*** skills for either frontend, and **both** when a change spans API and web. Know which web app you're in: **backoffice** is the authenticated vendor/admin app (port 3000); **webapp** is the public, no-auth customer app (port 3001) — they are separate surfaces with different auth assumptions.

**Deferred skills — do NOT route to these yet:** `ux-motion` and `e2e-testing` are intentionally not set up. There is no substantial UI to motion-design or drive end-to-end today. Add them when the UI matures; until then, verify web changes by the means the web skills describe, not by reaching for a skill that doesn't exist.

## 2. Lints and checks before commit

The Husky pre-commit hook runs **`lint-staged` only** (Prettier + ESLint on staged files) — it does **not** run type or build checks. So the hook alone will not catch a broken build or type error; that's on you. Before pushing, run the checks yourself from repo root:

- `pnpm lint` (and `pnpm lint:fix` to auto-fix) — ESLint on the web apps + `packages/ui`, `go vet` on the API.
- `pnpm format:check` (or `pnpm format` to fix) — Prettier on web, `gofmt` on the API.
- `pnpm check` — runs `check:api` (the API's `go build ./...` + `go vet ./...`) + `check:web` (`turbo run check-types` across `backoffice` + `webapp`).
- `pnpm test:api` / `pnpm test:web` (once web tests exist).

Treat `pnpm check` + `pnpm test:api` as a manual pre-push gate: never push code that would fail them. After any non-trivial change, run `pnpm check` once before committing.

## 3. Exhaustive compliance check before finishing

After implementation and before committing, audit every changed or created file against the relevant skills. This catches what lints and type checks miss — N+1 queries, wrong method order, missing tenant scoping, convention drift. Go through the explicit checklists.

### API (api-layering + api-methods-entities) — Go 1.25 + Gin, RAW `database/sql` + pgx, NO GORM

- **Layer boundaries respected.** Request flows handler → service → repository, no skipping. Handlers (`internal/delivery/http`) parse/validate DTOs and map to/from domain; services (`internal/services`) hold business logic; repositories (`internal/repository`) run SQL only. A handler never calls a repository directly; a repository never imports a service.
- **Transactions.** Repositories **never** begin, commit, or roll back. **Services own the transaction:** begin at the service layer, pass the `pgx.Tx` into the repository calls, then commit on success / roll back on error. Single-statement writes may use the pool directly; any multi-step write goes in one transaction.
- **RAW SQL is parameterized.** Every query uses positional placeholders (`$1`, `$2`, …) with args passed to `Query`/`Exec`/`QueryRow`. **Never** build SQL by string concatenation or `fmt.Sprintf` with request-derived values — that is a SQL-injection defect, not a style nit. Dynamic ID sets use `= ANY($1)` over a slice, never interpolation.
- **Multi-tenant scoping on every query.** Every read and write against a tenant-scoped table filters by `account_id` (and `branch_id` where the table is branch-scoped). The scope value comes from the authenticated context and is passed explicitly down handler → service → repository — never trusted from the request body. A query missing its `account_id` / `branch_id` predicate is a cross-tenant data leak; treat it as a bug, not an oversight.
- **No N+1 / batch.** No queries inside loops. Load related rows in one round trip — `= ANY($1)` over collected IDs, a JOIN, or `pgx.Batch` — before iterating. Independent external calls run concurrently (e.g. `golang.org/x/sync/errgroup`).
- **Method order.** Repositories and services follow get (List, GetByID) → Create → Save → Update → Delete → other. Handlers follow CRUD order. Exported symbol lists within a file are alphabetical when no flow dictates order.
- **DTOs.** Request/response structs live in the delivery layer, carry `json:"snake_case"` tags and `binding:"..."` validation, and bind via `c.ShouldBindJSON`. Domain ↔ DTO mapping happens at the handler boundary, never in services or repositories.
- **pgvector.** Semantic catalog search uses the `product.embedding VECTOR(1536)` column via vector operators; any embedding written must match that dimension.
- **Comments.** A `//` doc comment sits above every exported func/type, is a full sentence that starts with the symbol name and ends with a period. No narration on trivial code.
- **A schema change ships a goose migration in the same PR.** Any table/column/index/enum/type change adds a goose migration under `apps/api/migrations/` (create with `pnpm db:create-migration`) AND updates the consolidated reference schema under `apps/api/database/` in the same PR. **Migrations are the only executable path** — `pnpm db:init` builds a fresh DB by running them, so the chain must always rebuild from zero. The reference schema is what humans and agents read to know the current shape: keep its CREATE statements matching the migrated result, never apply it directly, and never add migration-style comments to it.

### Web (web-structure + web-components-pages) — Next.js (backoffice + webapp), React 19, `@repo/ui`

- **Tailwind class order:** display/flex → sizing → alignment → padding → gap → bg/border → rounded → state → typography.
- **Typography tokens:** no raw `text-*` / `font-*` utilities — use the type-scale tokens defined by the web skills.
- **File + in-component declaration order** follows the web skills (roughly: consts → metadata → props → state → derived → effects → handlers → return).
- **API boundary mapping:** the API speaks `snake_case` JSON; the web speaks `camelCase`. Map at the data/fetch boundary — never let `snake_case` leak into components.
- **Strings / translations** handled per the web skill's string convention (Coti ships Spanish-first for the Argentine market).
- **Env parity:** every new `process.env.NEXT_PUBLIC_*` read has a matching entry in `.env.example` (in the right topical group) AND in the local `.env`.
- **shadcn primitives left as-is:** the vendored primitives in `packages/ui` are not forked — compose and wrap them, don't edit the primitive.

### Both

- **Everything written into the repo is English.** Code, comments, SQL, `docs/technical/`,
  `docs/public/`, READMEs, scripts, CI, commits and PR bodies. Two exceptions, and they are
  different in kind: **UI copy is Argentine Spanish** (next-intl, `es-AR`) because it _is_
  the product, and **`docs/internal/` is Spanish** because it is the academic material that
  syncs with Notion. **Never mix two languages inside one file or one generated document** —
  a spec whose routes are English and whose root description is Spanish is a defect.
- **Comments earn their place.** The bar is _would a competent reader get this wrong without
  it?_ One line, two if genuinely needed; if it needs a paragraph it is a `docs/technical/`
  section and the comment points at it. Comment a non-obvious **why**, a constraint that
  looks arbitrary, or a footgun. **Never** narrate rejected alternatives ("uses X rather than
  Y because…"), tell a bug's story ("nobody noticed because…"), restate the signature, or
  describe how something used to be — a versioned file reads as if it had always been this
  way, because that is what git is for. Go doc comments on exported symbols stay, at one
  line, starting with the symbol name. When in doubt, leave it out.
- **Remove the `.gitkeep`** when you add the first real file to a scaffolded directory that only held one, so the folder's real content is the only content.
- **Scope awareness:** know which app you're in (`apps/api` vs `apps/backoffice` vs `apps/webapp`) and don't touch unrelated code. Use the right tooling (`go`/`pnpm db:*` in the API, `pnpm --filter <app>` for a web app).
- **No dead imports** left over from deleted files.
- **Docs and memory** updated per section 4.

## 4. Keep docs and memory current

Docs describe **how things work now**, not "what we changed" (no changelog-style prose in READMEs or the canonical schema).

- **On every change:** if it affects setup, structure, a flow, or how to run/check something, update the relevant README (root, `apps/api`, a web app) so it still says how things work. One source of truth; no drift.
- **DB schema:** add the goose migration under `apps/api/migrations/` (the only executable path) AND keep the reference schema under `apps/api/database/` matching it — see the API audit checklist above.

### Documentation tiers (`docs/`)

| Tier          | Path              | Committed       | Audience                        | Content                                                                                |
| ------------- | ----------------- | --------------- | ------------------------------- | -------------------------------------------------------------------------------------- |
| **Internal**  | `docs/internal/`  | No (gitignored) | Developer/agent                 | Plans, architecture, phase specs, costs, decisions                                     |
| **Public**    | `docs/public/`    | Yes             | Anyone (non-technical friendly) | Product / solution overview, key features                                              |
| **Technical** | `docs/technical/` | Yes             | Developers/agents               | How things work: architecture detail, auth, env vars, DB/schema, AI/embeddings, Docker |

Update the tier a change affects: a design decision or open question → `docs/internal/`; a user-facing capability → `docs/public/`; an implementation detail (new endpoint, auth change, new env var, embeddings/AI change, Docker change) → `docs/technical/`.

### Skills and memory

- **Skills:** update a skill only when you change a **convention** or **structure** (a new layer, a new place for components, a new comment style). Adding a feature that follows existing conventions usually needs no skill edit.
- **Memory:** after finishing a unit of work, update your project status memory — move what you completed into "built," drop it from "remaining/next," and record anything new that surfaced. Then sweep the rest of your memory: re-read each file and confirm it's still accurate (the index still lists every file; no rule was superseded by a convention you just established). Skip memory updates for trivial fixes. Manage memory per your own agent's memory rules — do not write to another agent's memory.

## 5. What NOT to commit

- **Never stage `.claude/plans/`, `.claude/settings.json`, `.claude/settings.local.json`** — local agent state, gitignored.
- **Never stage `docs/internal/`** — internal docs (plans, architecture, phase specs, costs, decisions) are gitignored.
- **Never stage temporary markdown files** (scratch notes, plan drafts, ad-hoc `.md`) or anything under `screenshots/` unless the user explicitly asks to commit a specific file by name.
- **Stage files individually by name** — never `git add .` or `git add -A`, so stray files aren't swept in.

## 6. Skill mirrors (`.claude/skills/` ↔ `.agents/skills/`)

Skills live in two mirrored trees: Claude Code reads `.claude/skills/`, Codex reads `.agents/skills/` (plus root `AGENTS.md`). The mirrors are kept **byte-equal** — there is no intentional drift. If you see a difference between the two trees that isn't a transient mid-edit state, treat it as a bug.

Rules when editing a mirrored skill:

- **Edit BOTH mirrors in the same pass**, then verify with `diff .claude/skills/<name>/SKILL.md .agents/skills/<name>/SKILL.md` (expect no output).
- **Never `cp` one mirror over the other.** Even though byte-equality is the goal, targeted edits + a `diff` check are safer — a blind copy silently clobbers per-agent fixes that snuck in mid-edit.
- **Use agent-agnostic language** for anything that varies per agent. "Manage per your project status memory" reads correctly for both Claude and Codex; "update Claude memory files" would force drift.
- **No cross-references that resolve on only one side** — no `[[memory-link]]` syntax, no by-name references to gitignored files only one agent has. When you need a per-agent convention, inline the rule's content in both mirrors instead.
- When in doubt whether content should mirror, **mirror it**.

## 7. Worktrees and parallel work

The repo ships `conductor.json` and `.worktreeinclude`, so work often happens inside a git worktree (a Conductor workspace or any `git worktree`). Follow your global worktree rules for that (never check out a branch already checked out elsewhere; edit shared/gitignored state in the main checkout; `.env` copied into a worktree is local to it). This skill does not restate Conductor mechanics — there is intentionally no repo-local Conductor how-to.

To split a body of work across multiple worktrees/agents, use the **parallel-planner** skill to decompose it into conflict-free units and emit a ready-to-paste prompt per unit.

## 8. Other habits

- **Hooks:** don't remove or weaken the `lint-staged` pre-commit hook without a clear reason. Note it does not run type/build checks — CI (gofmt/vet/build on the API, lint/check-types/build on the web apps) and your manual `pnpm check` are what catch those.
- **Paths and config:** the API uses `apps/api` as its cwd and the `internal/` Go convention; the web apps use `@/` aliases. Follow the structure and skills so new code lands in the right place.
- **Database:** manage schema with the `db:*` scripts — `pnpm db:init`, `pnpm db:migrate` / `:down` / `:status`, `pnpm db:create-migration`, `pnpm db:reset`.
