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
- **Web tests (backoffice / webapp):** **NOT YET SET UP.** No runner is installed
  and `pnpm test:web` currently just echoes a placeholder. See
  [Web testing](#web-testing-not-yet-set-up) for the intended convention — do not
  assume any web test infrastructure exists until it is scaffolded.
- **CI:** `.github/workflows/ci.api.yml` runs `gofmt` check, `go vet`, and
  `go build` on API PRs; the web workflows run lint + `check-types` + build. **No
  test job is wired into CI yet** — tests run locally. Add the `go test` step to
  CI once the first suites land.

## Running tests

```bash
# From apps/api
go test ./...                                   # all (non-integration) tests
go test -run TestQuoteService_Create ./internal/services   # one test
go test -race ./...                             # detect data races
go test -cover ./...                            # coverage summary
go test -tags=integration ./...                 # include integration tests (needs a test DB)

# From repo root
pnpm test:api                                   # go test ./... in apps/api
pnpm test                                       # test:api + test:web
```

Before pushing, run `pnpm check` (api: `go build` + `go vet`; web: `check-types`)
and the relevant test suite — CI does not yet gate on tests, so the local run is
the gate. `golangci-lint` also lints `_test.go` files (config enables `tests`),
so keep test code vet-clean.

## What to test (API)

The API is layered — `internal/{ai,config,delivery/http,domain,repository,services}`
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
- For integration tests, define a `setupTestDB(t *testing.T) *pgxpool.Pool`
  helper that spins up (or connects to) the pgvector test DB, runs goose
  migrations, and registers `t.Cleanup(...)` to drop the schema or roll back a
  transaction. Return a live pool — never a mock.
- **Compute expected values by hand.** Assert against manually derived numbers;
  never call the function under test (or its formula) a second time to produce
  the "expected" value — that only proves the code equals itself.
- **AI provider responses:** use fixed fake payloads from a fake implementing the
  provider interface — never a live model call.
- **Time-sensitive logic** (quote expiry, `válida hasta`, timestamps): inject a
  clock (`func() time.Time`) so tests don't depend on `time.Now()`.

## Web testing (NOT YET SET UP)

No web test runner exists in the repo yet. When it is added, the intended
convention is:

- **Vitest** for unit tests of pure functions / hooks / utilities in `backoffice`
  and `webapp`.
- **jsdom** (via Vitest) for component tests.

Until that scaffolding lands, `pnpm test:web` is a no-op placeholder — do not
write web tests assuming a runner is present, and don't claim web coverage exists.

## Related skills

- `api-layering` — which layer owns what logic (decides where a test belongs).
- `agent-workflow` — when in the task lifecycle to write and run tests.
- `commit` — use `test:` for test-only commits.
