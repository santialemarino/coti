---
name: commit
description: Commit message format and staging rules for the Coti repo. Use when creating a git commit.
---

# Commit conventions (Coti)

## Format

```
type: imperative description
```

- **Lowercase**, no period at the end
- **Imperative mood:** "add", "fix", "implement", "remove" — not "added", "fixes", "implementing"
- **One line.** No body. A PR carries a handful of commits at most, so the subject is the whole message. Put context in the PR description or the Notion ticket, not in a commit body.

## Types

| Type       | When to use                                    |
| ---------- | ---------------------------------------------- |
| `feat`     | New feature or behaviour                       |
| `fix`      | Bug fix                                        |
| `enh`      | Enhancement to existing functionality          |
| `refactor` | Code change with no behaviour change           |
| `docs`     | README, skill, or doc-only change              |
| `chore`    | Config, tooling, deps, migrations plumbing, CI |
| `test`     | Adding or fixing tests                         |
| `style`    | Formatting only (gofmt / prettier, no logic)   |

## Scopes (optional)

A scope in parens is allowed when a commit is clearly isolated to one area. Use
either an **app/package** or a **domain area** — never both.

- Apps / packages: `api`, `backoffice`, `webapp`, `ui`
- Domain areas: `product`, `rfq`, `quote` (and similar quoting-domain nouns)

```
feat(quote): add pgvector product matching to draft flow
fix(api): return 409 when rfq reference already exists
enh(backoffice): show margin breakdown in quote line editor
```

Drop the scope when a change spans several areas or the type already says enough.

## Staging

- Stage files **individually by name** — never `git add .` or `git add -A`.
- Review `git status` first; stage only the files that belong to this commit.

## What NOT to stage

- `.claude/plans/`, `.claude/settings.json`, `.claude/settings.local.json` — local agent state, gitignored
- `docs/internal/` — internal docs (architecture, plans, costs, phase specs, decisions), gitignored
- `screenshots/` — local artifacts, gitignored
- Temporary or scratch `.md` files not explicitly requested
- Generated / vendored output (`bin/`, `.next/`, `node_modules/`)

## Branches

GitFlow (simplified): `main` (production) ← `dev` (integration) ← ephemeral
work branches, lowercase kebab-case: `feat/*`, `fix/*`, `enhancement/*`,
`refactor/*`, `hotfix/*`. Never commit or push directly to `main` or `dev` —
open a PR into `dev`.

## Examples

```
feat: implement rfq-to-quote draft generation
fix: prevent duplicate product rows on catalog re-import
enh: improve pgvector similarity ranking for product matching
refactor: extract quote pricing into services layer
docs: document goose migration workflow in api readme
chore: add pgvector extension to docker-compose postgres
test: add unit tests for margin calculation helper
style: gofmt repository package
```
