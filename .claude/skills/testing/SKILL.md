---
name: testing
description: Where tests live, how to run them, and what to test in the Coti repo. Use when writing or running tests.
---

# Testing (Coti)

## Current state

- **API tests (Go 1.25):** Co-located with the code under test in
  `apps/api/internal/...` — Go convention. Each `<file>.go` may have a sibling
  `<file>_test.go`. Integration tests are gated behind a `//go:build integration`
  tag; cross-package end-to-end tests live under `apps/api/internal/integration/`.
- **Script tests (`scripts/`):** Node's built-in runner (`node:test` + `node:assert`),
  co-located as `<name>.test.mjs`. No dependency to install. See
  [Operational scripts](#operational-scripts).
- **Web tests (backoffice / webapp / `@repo/ui`):** **Vitest + jsdom**, co-located as
  `<file>.test.ts` / `<file>.test.tsx` beside the code under test. All three packages
  share one config from `@repo/vitest-config`. See [Web testing](#web-testing).
- **CI:** `.github/workflows/ci.api.yml` runs `gofmt` check, `go vet`,
  `golangci-lint`, `go build`, and `go test ./...` on API PRs; the web workflows run
  lint + `check-types` + test + build, and `ci.ui.yml` does the same for the design
  system; `ci.scripts.yml` covers `scripts/` and the root manifest; and `ci.skills.yml` covers the
  two skill trees, where its whole job is `diff -r` — the mirrors are byte-equal by contract and
  nothing else could catch a teammate editing one side only.
  Two workflows carry a **second job** that stands up PostgreSQL + pgvector and applies the
  migration chain: the API's runs the integration suite, which guards tenant isolation, and
  the scripts' runs the commands for real. Both gate merges rather than being a local-only
  courtesy. Run them locally too before pushing anything that touches SQL or tenant scoping;
  it is faster than waiting for CI to tell you.
- **Every workflow is path-filtered**, so a directory nothing watches gets no checks at all —
  a PR touching only it goes green having run nothing. Adding a top-level directory means
  adding or widening a workflow in the same change. Each workflow also lists **itself** in its
  `paths`, so editing one is covered by the run it configures. **None of them is a required
  check, deliberately:** under path filters a check that never runs for a given PR stays pending
  forever and blocks the merge instead of passing it.
- **Each one runs on a pull request into `main`/`dev` and on a push to either**, on the same
  paths. The push half is what checks the merge commit: a pull-request run tests a preview of the
  merge, which goes stale the moment the base branch moves under it, and without the push run
  nothing at all would notice a broken `dev` — the branch everyone else starts from. It is
  deliberately **not** wired to required status checks: with path filters a check that never runs
  for a given PR stays pending forever and blocks the merge instead of passing it.

## Running tests

```bash
# From apps/api
go test ./...                                   # all (non-integration) tests
go test -run TestQuoteService_Create ./internal/services   # one test
go test -race ./...                             # detect data races
go test -cover ./...                            # coverage summary
# Integration tests. Both roles are required: the restricted one is what the app
# uses, the owner one seeds fixtures past row level security.
TEST_DATABASE_URL=postgres://coti_app:coti_app@localhost:5432/coti?sslmode=disable \
TEST_DATABASE_ADMIN_URL=postgres://coti:coti@localhost:5432/coti?sslmode=disable \
  go test -tags=integration ./...

# From repo root
pnpm test:api                                   # go test ./... in apps/api
pnpm test:scripts                               # node --test over scripts/
pnpm test:web                                   # vitest in backoffice + webapp + @repo/ui
pnpm test                                       # test:scripts + test:api + test:web

# One web package, or one file
pnpm --filter backoffice run test
pnpm --filter @repo/ui run test:watch
pnpm --filter backoffice run test lib/api/client.test.ts
pnpm --filter backoffice run test:coverage
```

Before pushing, run `pnpm check` (api: `go build` + `go vet`; web: `check-types`)
and the relevant test suite. CI gates on the unit suite; the integration suite is
yours to run locally. `golangci-lint` also lints `_test.go` files (config enables `tests`),
so keep test code vet-clean.

## What to test (API)

The API is layered — `internal/{ai,config,delivery/http,domain,mail,ratelimit,repository,services}`
(see the `api-layering` skill). Test each layer for what it owns.

**Unit test** (no DB, no HTTP, no live providers):

- `internal/services/*` — business logic, quoting/pricing rules, validation,
  calculations. This is where most tests belong.
- `internal/domain/*` — value objects and pure domain logic.
- Pure helpers.
- **Mock collaborators through their interfaces**, not the concrete type:
  - the **AI provider** (`internal/ai`) — never call a live model from a test;
  - **repositories** — a service test uses an in-memory fake repo, not Postgres.

  Define the interface where the **consumer** needs it (the service package) and
  place small in-test fakes next to the test file.

**Integration test** (real Postgres + pgvector):

- `internal/repository/*` — exercise real SQL against a **real Postgres with the
  `pgvector` extension**. The API uses raw `database/sql` + `pgx` (no ORM), so
  the SQL, scanning, and vector queries are exactly what has to be verified.
- Critical end-to-end paths in `internal/integration/` — handler → service →
  repository → DB (tenant scoping, transaction atomicity, `ON CONFLICT` upserts,
  pgvector similarity ordering).
- **Never mock the database.** Use a throwaway Postgres via
  [testcontainers-go](https://pkg.go.dev/github.com/testcontainers/testcontainers-go)
  with a pgvector-enabled image (or a dedicated test `DATABASE_URL`). Mock/prod
  SQL divergence defeats the entire point of these tests.
- Gate them with `//go:build integration` so `go test ./...` stays fast by
  default and the DB is only required under `-tags=integration`.

**Don't test:**

- Framework behaviour (Gin routing, pgx driver internals).
- Trivial passthrough CRUD with no logic — covered transitively by integration
  tests on richer flows.

## File structure

```
apps/api/
└── internal/
    ├── domain/
    │   └── quote_test.go                   # unit (pure domain / value-object)
    ├── services/
    │   ├── quote_service.go
    │   └── quote_service_test.go           # unit (fakes for repo + ai provider)
    ├── repository/
    │   ├── product_repo.go
    │   └── product_repo_test.go            # integration (//go:build integration)
    └── integration/
        └── quote_flow_test.go              # cross-package end-to-end
```

## Style

- **Table-driven tests** for anything with several input/output cases:

  ```go
  func TestLineTotal(t *testing.T) {
      t.Parallel()
      cases := []struct {
          name      string
          qty       string // decimal string, NUMERIC(14,2)
          unitPrice string // decimal string, NUMERIC(14,2)
          markupBps int64  // basis points
          want      string // decimal string, computed by hand
      }{
          {"no markup", "3", "1000.00", 0, "3000.00"},
          {"10pct markup", "2", "1000.00", 1000, "2200.00"},
          {"zero qty", "0", "1000.00", 1000, "0.00"},
      }
      for _, tc := range cases {
          tc := tc
          t.Run(tc.name, func(t *testing.T) {
              t.Parallel()
              qty := decimal.RequireFromString(tc.qty)
              unitPrice := decimal.RequireFromString(tc.unitPrice)
              got := LineTotal(qty, unitPrice, tc.markupBps)
              if !got.Equal(decimal.RequireFromString(tc.want)) {
                  t.Fatalf("LineTotal(%s, %s, %d) = %s, want %s",
                      tc.qty, tc.unitPrice, tc.markupBps, got, tc.want)
              }
          })
      }
  }
  ```

- **t.Helper()** in shared assertion/setup helpers so failures point at the caller.
- **t.Parallel()** in independent tests; **don't** parallelize tests that share
  mutable state (e.g. ones hitting the same DB rows).
- **Assertions:** the standard library is enough;
  `github.com/stretchr/testify/require` / `assert` are welcome when they cut real
  noise — keep one consistent style per package.
- **Naming:** `TestSubject_Scenario_Expectation` for unit tests
  (`TestQuoteService_Create_DuplicateRfqReturnsConflict`). Use `t.Run`
  subtests for case labels.

## Fixtures & assertions

- Place test factories next to the package they support
  (`func makeQuote(...) Quote`).
- For integration tests, connect through the package's `testDB(t) *DB` helper: it reads both role
  URLs, skips the test when they are absent, and registers `t.Cleanup(db.Close)`. Return a live
  pool — never a mock.
- **Teardown ordering is a rule, not a detail.** `t.Cleanup` runs **after** the test body's `defer`s,
  so a pool a `defer` closed is already gone by the time the deletes run — register the close with
  `t.Cleanup` too. And a teardown that deletes real rows **never discards its error**, because a
  failed delete leaves rows behind and the suite still passes: route every one through `mustCleanup`,
  which fails the test when the delete cannot happen — a function in `internal/repository`, a method
  on `env` in `internal/integration`.
- **One row, one owner.** Let the seed that created a row remove it. A second per-test teardown for
  the same row is what puts a delete ahead of a foreign key still pointing at it, and an inline delete
  at the end of a test body is skipped by any `t.Fatal` above it — so it belongs in `t.Cleanup` or
  nowhere.
- **The suite leaves the database as it found it, and CI checks that.** The integration job seeds
  nothing, so every table must be empty once the suite finishes; a step counts all of them and fails
  naming whatever is left. Locally, compare **every** table before and after rather than a hand-picked
  few; a subset says nothing about the tables it does not name.
- **Compute expected values by hand.** Assert against manually derived numbers;
  never call the function under test (or its formula) a second time to produce
  the "expected" value — that only proves the code equals itself.
- **AI provider responses:** use fixed fake payloads from a fake implementing the
  provider interface — never a live model call.
- **Time-sensitive logic** (quote expiry, `válida hasta`, timestamps): inject a
  clock (`func() time.Time`) so tests don't depend on `time.Now()`.

## Operational scripts

`scripts/` runs against the database as the **owner role**, so it is the surface with the
most privilege and it gets tests like any other. Conventions:

- **Split the logic out of the executable.** The command file reads argv, prints, and picks
  an exit code; everything else lives in `scripts/lib/<name>.mjs`, which imports cleanly with
  no side effects. A module that calls `process.exit` or prints cannot be tested, so it
  raises instead and the command decides what that means.
- **Node's own runner**, `node --test` over `<name>.test.mjs` files co-located with the code.
  No runner dependency — do not add Vitest or Jest here; Vitest is reserved for the web apps.
- **Pure tests always run**; database-backed tests **skip themselves** when
  `TEST_DATABASE_ADMIN_URL` is absent, the way the Go integration suite skips without its two
  URLs. Use `describe('...', { skip: URL ? false : 'reason' }, ...)` so the skip prints its
  reason instead of vanishing.
- **Exercise the real command as a subprocess** (`spawnSync(process.execPath, [script, ...])`)
  and assert on **what changed in the database and the exit code**, not only on stdout.
  Testing the extracted function alone leaves the argv wiring and the exit codes uncovered,
  which is where a command-line typo lands first.
- **Create and remove your own fixture rows.** A script test runs against whatever database it
  is pointed at, so leaving rows behind dirties a dev database.

```bash
pnpm test:scripts    # from repo root; DB-backed tests skip without TEST_DATABASE_ADMIN_URL
TEST_DATABASE_ADMIN_URL=postgres://coti:coti@localhost:5433/coti?sslmode=disable pnpm test:scripts
```

`.github/workflows/ci.scripts.yml` watches `scripts/**`, `package.json` and `pnpm-lock.yaml`.
It exists because every other workflow is path-filtered to an app directory: without it a
change touching only these paths reaches `dev` with no check having run at all.

## Web testing

**Vitest + jsdom** in all three frontend packages — `apps/backoffice`, `apps/webapp` and
`packages/ui`. Tests are **co-located** beside the code under test as `<file>.test.ts`
(`.test.tsx` when it renders), the same shape the API and `scripts/` already use.

**One shared config: `@repo/vitest-config`.** Each package's `vitest.config.ts` is three
lines re-exporting it, the way `eslint.config.js` consumes `@repo/eslint-config`. Put a
setting that should hold everywhere in the shared package, not in one app. What it carries:

- **`server-only` is aliased to a stub.** Next resolves that marker in its own bundler and
  ships no package for it, so any module importing it — the API client, the session, every
  `lib/api/*` — is unresolvable under a plain runner without the alias.
- **Path aliases come from each package's own `tsconfig.json`** (`resolve.tsconfigPaths`),
  so `@/lib/...` resolves in a test exactly as it does in the app, with no second copy of
  the alias map to drift.
- **A `ResizeObserver` stub**, because jsdom implements none and Radix measures with one the
  moment a `Checkbox`, `Switch` or `RadioGroup` mounts — without it any test rendering a form
  dies.
- **A `cookieJar()` double**, from `@repo/vitest-config/cookies`. Use it rather than hand-rolling
  one: it reproduces Next's `delete`, which is a **set to `''`** and not a removal, so a reader
  that mishandles the blank fails here instead of in a browser.
- **A `schemaText()` double**, from `@repo/vitest-config/schema-text`, for the translator pair a
  form schema takes. `schemaText(true)` tags each message with the catalog it came from
  (`field:…` / `shared:…`), which is how a test asserts that "empty" and "malformed" resolve to
  different messages without hard-coding Spanish.
- **`isMessageShown` / `isMessageHeld`**, from `@repo/vitest-config/form-messages`. A form message is
  held and faded on its way out, so it is still in the DOM after it clears —
  `queryByText(...)` going null is the **defect**, not the expectation. These read `aria-hidden`
  instead, which is the one definition of "shown".

### Fake timers, and the one that bites

Anything counting down needs `vi.useFakeTimers()`, and there is a trap in how you enable it:

- **`{ shouldAdvanceTime: true }` is required** for Testing Library's `waitFor` to resolve at all —
  without it nothing drives the poll and every wait in the file times out.
- **But it advances fake time twice**: once with real elapsed time and again with each explicit
  `advanceTimersByTimeAsync`. Anything reading the wall clock then moves at roughly double speed and
  lands on a number the test cannot predict.
- So **assert the contract, not the tick** — that the control is shut with a number on it, that the
  number falls, and that it opens once the wait has passed. A test pinned to `N - 1` is testing the
  harness.

### What to test

- **`lib/api/*` mapping.** The API speaks snake_case and the component tree speaks
  camelCase; that boundary is the highest-value target in either app, because an unmapped
  field surfaces as `undefined` deep in a screen rather than as an error. Assert the whole
  mapped object, so a field added to the raw type but not the mapper fails here.
- **`lib/auth/*` and `config/routes.ts`.** Session and reachability logic: token expiry,
  the proxy-hop count, `safeNextPath`. This is where a security bug is cheapest to catch.
- **Schema factories** (`form-schema.ts`) — including that a message resolves through the
  translator rather than being baked in.
- **`@repo/ui` components** — the contract a caller depends on: `type="button"` by default,
  `aria-busy` and self-disabling on `PendingButton`, `asChild` passthrough. Not the styling.
- **Don't test** framework behaviour, or a component's exact class list.

### Assert behaviour, not ICU glyphs

The formatters are locale-bound, and `Intl` output changes with the Node build: the compact
separator is a **non-breaking** space, and `es-AR` renders a 12-hour clock. A test pinned to
`'1,5 M'` or `'14:30'` goes red on an ICU upgrade with nothing broken. Assert what the module
decides — rounding, sign, the date-only anchoring, the zone the day is computed in — and use
`\s` rather than a literal space.

### Coverage

Reported, never enforced — `pnpm --filter <pkg> run test:coverage` locally, and each web workflow
publishes its own number into the **job summary**, the way `ci.api.yml` does, so it needs no log
dive. No threshold anywhere, deliberately: one set before the suites are real is met by writing
tests that assert nothing.

`ci.backoffice.yml` and `ci.webapp.yml` each run their own app's suite, and **`ci.ui.yml`**
covers the design system — which until it existed was only ever _built_ in CI, never linted,
type-checked or tested.

## Related skills

- `api-layering` — which layer owns what logic (decides where a test belongs).
- `agent-workflow` — when in the task lifecycle to write and run tests.
- `commit` — use `test:` for test-only commits.
