---
name: pr-format
description: PR title, body, branch name, base branch, and label conventions for Coti. Use when creating a pull request.
---

# PR format (Coti)

Coti follows a simplified GitFlow. Tickets live in **Notion** — a PR links its Notion ticket page.

## Branch name

Format: `{type}/{kebab-case-description}` — lowercase, kebab-case.

Types match the branch/commit types:

| Type        | Prefix         |
| ----------- | -------------- |
| feature     | `feat/`        |
| bug fix     | `fix/`         |
| enhancement | `enhancement/` |
| refactor    | `refactor/`    |
| hotfix      | `hotfix/`      |

Examples: `feat/semantic-catalog-search`, `fix/quote-total-rounding`, `enhancement/vendor-dashboard-filters`.

## Base branch (GitFlow)

- **Every PR targets `dev`** — the integration branch. Never open a PR against `main` from a feature branch.
- **Exception:** a `hotfix/*` branch targets **`main`** (then gets merged back down to `dev`), per GitFlow.
- **Always confirm the base branch before `gh pr create`** — check with `gh pr create --base dev ...` explicitly rather than relying on the repo default. `main` is production; a stray feature PR against it is a mistake.

Every PR carries a **label** (see below) and needs **at least one approval** before merge.

## PR title

Title Case, imperative, concise. No ticket prefix. Often starts with an imperative verb (`Implement`, `Add`, `Fix`, `Restructure`) but noun-first is acceptable. Keep it short enough for the GitHub list view — the body H1 is the detailed version.

Examples:

- `Implement Semantic Catalog Search with pgvector Embeddings`
- `Fix Cross-Tenant Leak in Quote Repository Queries`
- `Add Vendor Dashboard Filters to Backoffice`

## PR body

```
# [{TYPE}] {Detailed Title}

## Card Link

{Notion ticket page URL}

## Summary

{One-paragraph overview.}

{**Bold section headers with file paths** — detailed bullet points per change area.}

{**Translations:** — when user-facing strings/i18n keys were added or changed.}
{**Env vars:** — when environment variables were added or removed.}

## ⚠️ Migration Required          ← only when the PR changes the DB schema

{goose command + the migration SQL.}

## Acceptance Criteria

{Pulled from the Notion ticket.}

## Screenshots & Recordings       ← opt-in only; see the asset policy below
```

### TYPE values

Uppercase in the body header, matching the branch type:

| Type        | Header          |
| ----------- | --------------- |
| feat        | `[FEAT]`        |
| fix         | `[FIX]`         |
| enhancement | `[ENHANCEMENT]` |
| refactor    | `[REFACTOR]`    |
| hotfix      | `[HOTFIX]`      |
| docs        | `[DOCS]`        |
| chore       | `[CHORE]`       |
| test        | `[TEST]`        |

Compound types are acceptable in rare cases (e.g. `[DOCS/ENHANCEMENT]`).

### H1 title vs PR title

The H1 in the body is the **detailed** version — it can be longer and more descriptive than the PR title. The PR title is the short version for the GitHub list view.

- PR title: `Implement Semantic Catalog Search`
- H1: `# [FEAT] Semantic Catalog Search — pgvector Embeddings, Query Endpoint & Backoffice UI`

### Card Link

Coti tracks tickets in **Notion**. Paste the **Notion ticket page URL** here. Use `N/A` only when there is genuinely no ticket (e.g. a spontaneous chore). Do not invent a URL.

### Summary

- Lead with a one-paragraph overview of what and why.
- Group changes under **bold headers** that name the affected area with file paths in backticks. Format: `**Description — (path/to/file.go):**` or `**Backend — new endpoint under '/quotes' (internal/delivery/http/..., internal/services/...):**`.
- Be detailed — close to implementation-plan level. Mention handler/service/repository names, component names, file paths, patterns.
- End with cross-cutting sub-sections when applicable:
  - **`Translations:`** — when user-facing strings / i18n keys were added or changed.
  - **`Env vars:`** — when environment variables were added or removed. List each key, its default, and which `.env.example` was updated, so reviewers don't have to grep for new env surface.
- Do not include a Docs sub-section — documentation changes are part of the feature; the code summary already conveys what changed.

**What NOT to include in the summary:**

- **Meta / repo-plumbing changes.** Don't mention edits to skills, `CLAUDE.md`, `AGENTS.md`, `README.md`, `.claude/`, or `.agents/` contents.
- **Internal-doc changes.** Don't mention `docs/internal/` updates (gitignored).
- **External-platform actions.** Don't describe things done in Notion, Slack, etc. as changes "we made" in the PR.

### ⚠️ Migration Required

Include this section **only** when the PR changes the database schema, placed between Summary and Acceptance Criteria. Coti uses **goose** migrations. Show both the command reviewers run and the migration SQL:

````
This PR adds a goose migration. After merging and pulling:

```bash
pnpm db:migrate
```

Migration (`apps/api/migrations/00XX_add_product_embedding.sql`):

```sql
-- +goose Up
ALTER TABLE product ADD COLUMN embedding VECTOR(1536);

-- +goose Down
ALTER TABLE product DROP COLUMN embedding;
```
````

The canonical schema under `apps/api/database/` must be updated in the same PR to reflect the new state.

### Acceptance Criteria

Pull the acceptance criteria from the linked Notion ticket and list them here. Use `N/A` only when the ticket has none (or there is no ticket).

### Screenshots & Recordings

**Off by default — opt in per PR.** Omit this section unless the user explicitly asks for assets on the current PR. (There is little product UI to capture yet, and backend/API/docs-only PRs never carry screenshots.) When a UI change lands, verifying it still matters — only pasting the captured assets into the PR body is what's toggled off.

**When the user explicitly asks for assets on this PR:**

- Include only for UI changes (backoffice/webapp). Use `<img>` tags with `width`/`height`/`alt`. Videos as bare asset URLs.
- Agents **cannot** drag-and-drop into the GitHub PR editor (that upload needs a browser session cookie + CSRF token). To host assets from the CLI without committing them to the repo, use a GitHub **prerelease**:

  ```bash
  gh release create pr-<num>-screenshots --prerelease \
    --title "PR #<num> — <feature> screenshots" \
    --notes "Assets for PR #<num>. Not a version release." \
    file1.png feature.webm
  ```

  Assets are then reachable at `https://github.com/<owner>/<repo>/releases/download/pr-<num>-screenshots/<filename>` — paste those into `<img src="...">` tags. Mention the prerelease tag in the section so reviewers know where the assets live. Delete the tag after merge if you don't want them piling up.

- If the user would rather paste images themselves (Cmd+V / drag-drop on github.com produces `user-attachments/assets/<uuid>` URLs that also render `.webm`/`.mp4` as an embedded player), that's the gold-standard path — leave a placeholder for them.

## Labels

Apply from the repo's label set. **Every PR needs at least one label**, and multiple labels are standard — always add the layer label(s) too.

**Type labels:**

| Type        | Label           |
| ----------- | --------------- |
| feat        | `feature`       |
| fix         | `bug fix`       |
| enhancement | `enhancement`   |
| refactor    | `refactor`      |
| hotfix      | `hotfix`        |
| docs        | `documentation` |
| chore       | `chore`         |
| test        | `test`          |

**Layer labels (monorepo) — apply every layer the PR touches:**

| Layer touched      | Label        |
| ------------------ | ------------ |
| `apps/api/`        | `api`        |
| `apps/backoffice/` | `backoffice` |
| `apps/webapp/`     | `webapp`     |
| `packages/ui/`     | `ui`         |

A PR can carry several layer labels (e.g. a change spanning the API and the backoffice gets `api` + `backoffice`).

## Example

**Title:** `Implement Semantic Catalog Search with pgvector Embeddings`

**Base:** `dev` · **Labels:** `feature`, `api`, `backoffice`

**Body:**

````
# [FEAT] Semantic Catalog Search — pgvector Embeddings, Query Endpoint & Backoffice UI

## Card Link

https://www.notion.so/coti/Semantic-catalog-search-abc123

## Summary

Adds semantic search over the product catalog so vendors can find `product` rows by meaning, not just exact text. Products carry an OpenAI embedding; a new endpoint runs a vector similarity query scoped to the tenant.

**Backend — new endpoint and service (`internal/delivery/http/product_handler.go`, `internal/services/product_service.go`, `internal/repository/product_repository.go`):**
- `GET /quotes/catalog/search?q=...` embeds the query, runs a `product.embedding <=> $1` similarity search filtered by `account_id` / `branch_id`, and returns the top matches.
- Repository uses parameterized SQL only; the service owns the transaction and passes the tenant scope from the authenticated context.

**Backoffice — search UI (`apps/backoffice/app/catalog/...`):**
- New search box wired to the endpoint, mapping the API's snake_case response to camelCase at the data boundary.

**Env vars:**
- `OPENAI_API_KEY` (no default) — added to `.env.example` and the local `.env`; read by `internal/config`.

## ⚠️ Migration Required

After merging and pulling:

```bash
pnpm db:migrate
```

Migration (`apps/api/migrations/00XX_add_product_embedding.sql`):

```sql
-- +goose Up
ALTER TABLE product ADD COLUMN embedding VECTOR(1536);
CREATE INDEX idx_product_embedding ON product USING ivfflat (embedding vector_cosine_ops);

-- +goose Down
DROP INDEX idx_product_embedding;
ALTER TABLE product DROP COLUMN embedding;
```

## Acceptance Criteria

- Vendor can search the catalog by natural-language description and get relevant products.
- Results are scoped to the vendor's own account/branch — no cross-tenant rows.
- Empty query returns a validation error, not a full scan.
````
