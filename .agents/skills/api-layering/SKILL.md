---
name: api-layering
description: Backend API structure and where to create files (handlers, services, repositories, DTOs, domain, AI adapters, utils). Use when adding endpoints, new features, or organizing code in apps/api.
---

# API layering (Coti backend)

`apps/api` is Go 1.25 + Gin, laid out as **layered / hexagonal** (ports and adapters). Layers depend inward — handlers → services → domain — and external concerns (the AI extraction/embedding/handler providers, and anything else off-process) sit behind Go interfaces (ports) implemented by adapters.

The data layer is **raw `database/sql` semantics over `pgx` / `pgxpool`** against PostgreSQL + pgvector. There is **no GORM and no ORM**: every query is explicit SQL with `$1` placeholders, and every row is scanned by hand into a domain struct. Do not add an ORM.

Two schema facts drive every type decision below, so internalize them first:

- **Every primary key is a UUID v4** (`UUID PRIMARY KEY DEFAULT gen_random_uuid()`, pgcrypto). In Go that is `uuid.UUID` (`github.com/google/uuid`) or `pgtype.UUID` — **never `int64`**.
- **Money and quantities are `NUMERIC(14,2)`** (`price`, `subtotal`, `total`, `amount`, `min_price`, `unit_price_snapshot`, `price_snapshot`, `action_value`, `value`, `quantity`, `stock`). Carry them in a decimal type (`github.com/shopspring/decimal`, or `pgtype.Numeric`) and expose them as a **decimal string** on the wire — never `float64`, never `int64` centavos. See `api-methods-entities`.

## Flow and layers

Request flow: **handler → service → repository → DB**.

- **internal/delivery/http/handler/** — HTTP only: bind the DTO, call the service, translate the result or domain error into a Gin response. No business logic, no SQL.
- **internal/delivery/http/dto/** — Request/response structs (the wire contract; `json` tags snake_case). Bound via `c.ShouldBindJSON`, validated with `binding:` tags, mapped to/from domain types **in the handler**.
- **internal/delivery/http/middleware/** — Gin middleware (JWT auth, tenant resolution that puts `account_id` and the active `branch_id` on the context, request logging, rate limiting). Wired by the router setup.
- **internal/services/** — Business logic: orchestrate use cases, call repositories and AI ports. Work in **domain** types; no HTTP, no SQL strings. **Owns the transaction boundary** (see below). **Owns every business invariant** — the state×intention validation of AI proposals, the deterministic discount math, the derivation of `quote.current_status`. Depends only on ports (interfaces), never on a concrete adapter package.
- **internal/repository/** — Data access: explicit SQL through a `Querier` (a `*pgxpool.Pool` or a `pgx.Tx`) handed in by the service. Scans rows into domain structs. **Never** calls `Commit`/`Rollback`.
- **internal/domain/** — Domain types, value objects, enums, domain errors, and the **port interfaces** the services consume (e.g. the AI `Embedder` / `RFQExtractor` / `ChangeRequestHandler`). Imports no other `internal/` package.
- **internal/ai/** — Adapters implementing the domain AI ports (RFQ extraction, embedding generation, catalog re-ranking, change-request handling). All provider-specific logic (SDK calls, prompt assembly, structured-output schemas, response parsing) lives here, behind the port. Per-provider subpackages when there is more than one (e.g. `internal/ai/openai/`).
- **internal/mail/** — Adapters implementing the domain `Mailer` port. Same shape as `internal/ai`: transport-specific logic lives here and nothing above the port knows which provider is bound. The `console` adapter logs the message instead of sending it, and is the default.
- **internal/ratelimit/** — The counters behind the rate-limit middleware, which consumes them through its own `Limiter` interface. In-memory today; a shared store swaps in behind the same interface once there is more than one instance.
- **internal/config/** — Env loading (`godotenv` is loaded in `main`) + defaults; the one place for every configurable threshold (match cutoffs, top-K, timeouts, default expiry days).
- **internal/utils/** — Generic, entity-agnostic helpers reused across features.
- **cmd/** — One directory per binary, each a composition root: read config, open the pools, take the AI adapters from `internal/ai/provider`, inject repos + adapters into services, and do the binary's job. **A binary with no cross-account job opens `repository.NewTenantDB`, not `NewDB`** — the restricted pool alone, on a type that has no `CrossAccount` or `AdminTx`, so the boundary is checked by the compiler. `cmd/api` builds the router and serves HTTP; `cmd/catalog-embed` vectorizes one account's catalog; `cmd/scheduled-job` runs one unit of scheduled work and exits, with the deployment platform owning the schedule so a frequency is never compiled in. No business logic in any of them. **A job too long for a request budget is a command, not a route** — embedding a whole catalog outruns `SERVER_WRITE_TIMEOUT_SECONDS` several times over.

Do not put business logic in handlers or SQL strings in services. Do not let HTTP types (request/response bodies) leak into domain or repositories.

## Transaction rules

### Every request-scoped query runs in a tenant-scoped transaction

Row level security reads the account from `app.current_account_id`, a **transaction-local** GUC. A query on the bare pool therefore runs outside that scope, matches no policy, and **silently reads zero rows** — so "reads can skip the transaction" does not hold here. `repository.DB.InTenantTx` is the only path for request-scoped access: it begins the transaction, sets the GUC, runs your function, and commits or rolls back.

```go
// Service — one tenant-scoped transaction per use case.
func (s *QuoteService) Archive(ctx context.Context, tenant domain.Tenant, quoteID uuid.UUID) error {
    return s.db.InTenantTx(ctx, tenant, func(q repository.Querier) error {
        quote, err := s.quoteRepo.GetByID(ctx, q, tenant.AccountID, quoteID)
        if err != nil {
            return err
        }
        if err := s.quoteRepo.Archive(ctx, q, tenant.AccountID, quote.ID); err != nil {
            return err
        }
        return s.statusRepo.Append(ctx, q, tenant.AccountID, quote.ID, /* ... */)
    })
}
```

Repositories accept the `Querier` — the read/write surface shared by `*pgxpool.Pool` and `pgx.Tx` — and run their SQL on it. They **never** call `Begin`, `Commit`, or `Rollback`.

### Cross-account access is explicit and rare

Three operations legitimately span accounts, and they use `db.CrossAccount()` (the owner pool, which bypasses RLS): the follow-up cron, login by email (the account is unknown until the user is found), and resolving a `quote_send.public_token` for the public webapp. The token flow resolves the account there and then continues through `InTenantTx` — it does not keep querying on the owner pool. **Any other use is a cross-tenant leak.**

`DB.AdminConn` is a third door onto the owner pool, and it exists for one reason: a Postgres advisory lock is **session-scoped**, so a scheduled job that took one through the pool would leave it on whichever connection served that call and release it on whichever served the next. It holds a single connection for the length of the run instead.

```go
// internal/repository/querier.go

// Querier is the read/write surface shared by *pgxpool.Pool and pgx.Tx, so a
// repository method behaves identically inside or outside a transaction.
type Querier interface {
    Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
    Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
    QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}
```

### Bootstrapping an account is the one write that starts outside a tenant

Creating an account cannot be tenant-scoped — the account does not exist yet, so there is no scope to set. It runs on `db.AdminTx`, the owner pool, and is the only write allowed to. Everything afterwards is tenant-scoped.

```go
// Service — owns the transaction. Registering an account seeds its head-office
// branch, its admin user, and the user↔branch link as one atomic unit.
func (s *AccountService) RegisterAccount(ctx context.Context, in domain.NewAccount) (*domain.Account, error) {
    tx, err := s.db.AdminTx(ctx)
    if err != nil {
        return nil, err
    }
    defer tx.Rollback(ctx) // no-op after Commit.

    account, err := s.accountRepo.Create(ctx, tx, in.Account)
    if err != nil {
        return nil, err
    }
    branch, err := s.branchRepo.Create(ctx, tx, account.ID, in.HeadOfficeBranch)
    if err != nil {
        return nil, err
    }
    admin, err := s.userRepo.Create(ctx, tx, account.ID, in.Admin)
    if err != nil {
        return nil, err
    }
    if err := s.userBranchRepo.Link(ctx, tx, admin.ID, branch.ID); err != nil {
        return nil, err
    }
    if err := tx.Commit(ctx); err != nil {
        return nil, err
    }
    return account, nil
}
```

### Keep external calls out of the transaction

AI extraction, embedding, and change-request handling are slow and can fail on their own timeline. Run them **before** you open the transaction, then open a short transaction only around the writes. Never hold a `pgx.Tx` open across an AI provider call.

Quote generation is the canonical shape: **extract → embed → match happen first**, in memory; only the persist runs in a transaction, and the whole persist is one atomic unit.

```go
// GenerateQuote — external work first, then one short transaction.
items, err := s.extractor.Extract(ctx, rfq.RawText)            // external (AI).
if err != nil {
    return nil, err
}
embs, err := s.embedder.Embed(ctx, descriptionsOf(items))      // external (AI).
if err != nil {
    return nil, err
}
matched := s.matchCatalog(catalog, embs)                       // in-memory.

// Then one InTenantTx: create quote (current_status = DRAFT) → create
// quote_version v1 (is_immutable = false) → insert quote_items → recompute
// quote_version.total → set quote.current_version_id → append rfq_status_change.
// The quote is born here, at RECEIVED → GENERATED, because extracted items have
// nowhere else to live: there is no rfq_item table.
```

## Performance rules

### Never query inside a loop (N+1)

Need data for N items? Fetch it in **one batch query** before the loop, then look it up from a map. Never call a repository method inside a `for`.

```go
// BAD — N+1: one query per quote item.
for _, it := range items {
    p, _ := s.productRepo.GetByID(ctx, s.pool, accountID, it.ProductID)
}

// GOOD — batch load, then loop in memory. quote_item.product_id is nullable
// (NO_MATCH items carry no product), so skip the nil ones.
ids := make([]uuid.UUID, 0, len(items))
for _, it := range items {
    if it.ProductID != nil {
        ids = append(ids, *it.ProductID)
    }
}
products, err := s.productRepo.GetByIDs(ctx, s.pool, accountID, ids) // map[uuid.UUID]domain.Product
for _, it := range items {
    if it.ProductID == nil {
        continue
    }
    p := products[*it.ProductID]
}
```

When you add a repository method that will be called in a loop, **always add a batch variant** (accepts a `[]uuid.UUID`, returns `map[uuid.UUID]T` or `map[uuid.UUID][]T` keyed by ID). See `api-methods-entities`.

### Parallelize independent external calls with errgroup

When you make N independent external calls — e.g. the embedding provider takes one text per request, or you extract from several `rfq_attachment` rows at once — use `golang.org/x/sync/errgroup`, not a sequential loop. A single `pgx.Tx` is **not** safe for concurrent use, so **parallelize the external calls, then persist sequentially.**

```go
// GOOD — parallel embed, sequential persist.
g, gctx := errgroup.WithContext(ctx)
embs := make([]pgvector.Vector, len(items))
for i := range items {
    i := i
    g.Go(func() error {
        v, err := s.embedder.EmbedOne(gctx, items[i].RequestedDescription)
        if err != nil {
            return err
        }
        embs[i] = v
        return nil
    })
}
if err := g.Wait(); err != nil {
    return nil, err
}
// ...then match / write with embs, one connection at a time.
```

Prefer a single batched provider call (`Embed([]string)`) when the provider supports it — even better than errgroup.

### Use ON CONFLICT for idempotent writes, batch for bulk writes

Never SELECT-then-INSERT/UPDATE by hand. Use `INSERT ... ON CONFLICT (...)` for idempotent writes — but the conflict target must be a **real unique constraint** from the schema (e.g. `uq_app_user_email` on `(account_id, email)`, `uq_user_branch` on `(user_id, branch_id)`, `uq_client_tag` on `(client_id, tag_id)`, `uq_quote_version` on `(quote_id, version_number)`). Do not invent a conflict target the schema doesn't back.

```go
// Idempotent link — tagging a client is safe to repeat.
_, err := q.Exec(ctx,
    `INSERT INTO client_tag (client_id, tag_id)
     VALUES ($1, $2)
     ON CONFLICT (client_id, tag_id) DO NOTHING`,
    clientID, tagID)
```

For many rows, use a **multi-row `INSERT`** or `pgx.Batch` — never a per-row loop of `Exec`. Inserting the line items of a new `quote_version` is one statement:

```go
// Bulk insert quote_items for one version — one round-trip, not one per line.
// (quote_item has no updated_at; rows on a FROZEN version are immutable.)
_, err := q.Exec(ctx,
    `INSERT INTO quote_item
        (version_id, product_id, requested_description, quantity, unit,
         unit_price_snapshot, min_price_snapshot, subtotal, confidence_score, match_status)
     SELECT $1, u.product_id, u.requested_description, u.quantity, u.unit,
            u.unit_price_snapshot, u.subtotal, u.confidence_score, u.match_status
     FROM unnest($2::uuid[], $3::text[], $4::numeric[]) AS u(product_id, requested_description, quantity)`,
    versionID /* ... */)
```

### Prefer expressive operations over accumulator loops

When intent maps to sum / max / filter / group, write it that way with `slices` helpers or a small typed helper. Reserve `for` for early `break`, per-iteration side effects, or where it is genuinely clearer.

## Semantic catalog search (pgvector)

Catalog matching is the core of the RFQ pipeline. `product.embedding` is `VECTOR(1536)` (pgvector extension `"vector"`); `product_synonym` holds curated regional/colloquial terms for lexical fallback (mitigates the synonym-matching risk). The ivfflat index is created **after** data load — it is commented out in the schema on purpose.

- Query with pgvector's distance operators: **`<=>` cosine**, **`<->` L2**. Order by the distance and `LIMIT` the top-K. Coti uses `<=>` (cosine) as the default — bake the K and any cutoff in `internal/config`, not inline.
- Pass the query vector as a `pgvector.Vector` bind param (`pgvector.NewVector([]float32{...})`), never string-interpolate it.
- Combine semantic hits with `product_synonym` matches in the **repository**; the service decides confidence, not the SQL.
- **Every search is account-scoped, and filtered to what the branch carries** — `product` hangs off `account`, so filter `WHERE p.account_id = $n` and join `branch_product` for the active branch alongside the vector order. Because an ANN scan filters _after_ ordering, over-fetch (`LIMIT k * over_fetch_factor`, both in `internal/config`) and trim in the service, or the branch filter can leave you short of K. One over-fetch is not a guarantee: the service widens the fetch until it has K or a wider one returns nothing new. And set `ivfflat.probes` for the transaction — the database visits one partition per scan by default, which recalls too little to survive the filter.
- **Unmatched lines are flagged, never discarded.** A line with no acceptable match becomes a `quote_item` with `product_id` NULL and `match_status = NO_MATCH`; ambiguous ones get `AMBIGUOUS`. Every item carries a `confidence_score`. The search returns candidates; the service assigns `match_status`.

```go
// SearchByEmbedding returns the closest catalog matches to an RFQ line embedding,
// scoped to the account and filtered to what the branch carries, by cosine distance.
func (r *ProductRepository) SearchByEmbedding(ctx context.Context, q Querier, accountID, branchID uuid.UUID, emb pgvector.Vector, limit int) ([]domain.ProductMatch, error) {
    rows, err := q.Query(ctx,
        `SELECT p.id, p.canonical_name, p.embedding <=> $1 AS distance
         FROM product p
         JOIN branch_product bp ON bp.product_id = p.id AND bp.branch_id = $3
         WHERE p.account_id = $2 AND p.is_active = TRUE AND bp.is_active = TRUE
         ORDER BY p.embedding <=> $1
         LIMIT $4`,
        emb, accountID, branchID, limit)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var matches []domain.ProductMatch
    for rows.Next() {
        var m domain.ProductMatch
        if err := rows.Scan(&m.ProductID, &m.CanonicalName, &m.Distance); err != nil {
            return nil, err
        }
        matches = append(matches, m)
    }
    return matches, rows.Err()
}
```

**Embedding generation never lives in the repository or the SQL.** The repository receives an already-computed `pgvector.Vector`; the service gets it from the AI port (below).

## AI providers — ports and adapters

Coti is a **human-in-the-loop copilot**. Two pipelines touch external models: the RFQ pipeline (extract informal RFQ text → line items → embed → match against the catalog → assemble a review-ready `quote`) and the change-request handler (interpret a client message → propose a typed action on an existing quote). Both talk to models only through domain ports.

- **Ports** are Go interfaces in **`internal/domain`** (so both services and adapters import them without a cycle). Three are the **provider layer** — one per external capability, because no provider covers all three:

  ```go
  // StructuredGenerator asks a language model for one answer shaped by req.Schema and
  // decodes it into out. Text, images and documents all ride req.Input, so vision and
  // PDF need no port of their own.
  type StructuredGenerator interface {
      Generate(ctx context.Context, req GenerationRequest, out any) (*GenerationUsage, error)
  }

  // Embedder produces EmbeddingDimension-wide embeddings for catalog and RFQ text.
  type Embedder interface {
      Embed(ctx context.Context, texts []string) ([]pgvector.Vector, error)
  }

  // Transcriber turns a recording into text.
  type Transcriber interface {
      Transcribe(ctx context.Context, audio Audio) (string, error)
  }
  ```

  **Feature ports sit on top of those, never beside them.** `RFQExtractor` and `ChangeRequestHandler` are the domain's vocabulary for what the engine does; their adapters own the prompt and the schema and reach the model through `StructuredGenerator`. A feature adapter never talks to a provider SDK directly.

- **Adapters** implement the ports in **`internal/ai/`**, one subpackage per provider (`anthropic`, `openai`). Prompt assembly, SDK calls and response parsing live in the subpackage; **the policy every provider shares lives in the package root** — `Retry`, the usage log, the `Disabled*` stand-ins, `RetryableStatus`, `RetryAfter`, `Fail`. Do not reimplement retry or status classification per adapter: that is one decision, and a copy per provider is a copy to keep in sync.
- **Which adapter answers is decided in `internal/ai/provider`, once.** It sits above the adapters, so every binary that needs a model makes the same choices from the same settings; a `switch` copied into a second command is a second place to edit when a provider is added. Adding one is a new subpackage plus a `case` there.
- **An adapter is handed only its own settings.** `config.AIConfig` exposes `Anthropic()`, `Embeddings()` and `Transcription()`, each carrying one provider's key, model, timeout and the shared retry policy. Passing the whole config would put every provider's credential in every adapter, one `%+v` away from a log.
- **The adapter says which of its failures are transient; the shared loop only reads the mark.** `ai.Retryable` / `ai.RetryableAfter` for a rate limit, a provider fault, a dropped connection or an answer that missed the schema; `ai.Rejected` for a request the provider would not serve — a bad schema, model or key, or a safety refusal. Then `ai.Fail` decides what the caller sees: `Rejected` and a cancelled caller stay as they are, everything else becomes `domain.ErrAIUnavailable`. **A fault of ours must never surface as a provider outage**, or a client is invited to retry something that cannot succeed.
- **Every capability defaults to `disabled`**, and the stand-in refuses with `ErrAIUnavailable`. A checkout with no keys has to boot; the failure belongs on the call that needed a model, not on startup. A stub that answers with invented data is the one thing this layer exists to prevent.
- **Each external call is bounded per attempt, and the chain is not.** `AI_*_TIMEOUT_SECONDS` caps one attempt, so the worst case is that times `AI_MAX_ATTEMPTS` plus the backoff — which outruns `SERVER_WRITE_TIMEOUT_SECONDS`. An AI call does not belong inline in a request handler on that budget.
- The **service depends on the interface**, so swapping providers is a one-line change in `internal/ai/provider`. It never imports `internal/ai`.

See `docs/technical/ai-providers.md` for the whole layer, including what a retry costs and where the vector codec is registered, and `docs/technical/catalog.md` for what is done with the vectors.

These invariants are **enforced in the service layer**, and the code must make them true — they are product invariants, not style:

- **Schema-forced output.** AI proposals come back via structured outputs / tool use with a closed enum, never a "return me JSON" prompt. The catalog carries an explicit `NO_MATCH` escape so "I can't resolve this" is a structurally valid answer, not an invented action.
- **The backend validates every proposal against the state×intention matrix** (see `estados.md` in the domain docs) **before materializing anything.** The change-request handler only runs from `SENT` onward; on `ACCEPTED` / `REJECTED` / archived everything is blocked; reopening reactivates the _same_ quote, never a duplicate. The chain is invariant: **AI proposes → backend validates → seller approves.**
- **The AI never calculates money.** It may reference or suggest a typed `promotion` rule; the deterministic discount engine computes every amount (see below).
- **AI output is persisted as a reviewable draft** — a new `quote_version` with `is_immutable = false` — and **never auto-sent**. The client only ever sees what a seller approved.
- **Level-1 handler decisions are logged** in `handler_decision` (client input → AI interpretation → AI proposal → seller decision: `APPROVED_AS_IS` / `EDITED` / `REJECTED` / `MANUAL_OVERRIDE`). This is append-only pilot-metrics raw material; write it whenever the handler runs.

## Deterministic discount engine

Discounts are **backend-computed and deterministic — never the LLM.** Keep the math in the service (or a `<feature>_helpers.go` beside it), never in SQL and never in an adapter.

- **`promotion`** is a reusable rule (hangs off `account_id`, optional `branch_id`); **`quote_discount`** is one application of a discount to a `quote_version` (with `quote_discount_item` as the bridge for `ITEM` / `ITEM_SET` scope). Two entities, do not conflate.
- The automatic sweep runs on `GENERATED → QUOTED` and after every item change; it applies active promotions that match the quote as-is.
- `ON_TOTAL` is computed on the **net** (line discounts first, then total). Round to 2 decimals per discount. The result never goes negative.
- **The floor is `quote_item.min_price_snapshot`, never the live `product_price.min_price`.** The evaluator reads only frozen values; otherwise re-sweeping one version after a price change returns a different total for the same items.
- **The floor is optional, and absent is the common case.** `min_price` is nullable and most accounts never set one, so a null snapshot means _no floor_ — never a floor of zero, which would let a discount drive the line to nothing.
- **The floor binds the sweep, not people.** A seller may price a line below it by hand; that is warned and recorded in the version's `comment`, and the service does not reject it.
- Conflicts resolve by `is_exclusive` + `priority` (higher priority wins; tie → larger discount; not stackable by default).
- `quote_version.total = Σ quote_item.subtotal − Σ quote_discount.amount`. `quote_item` itself holds its price snapshots and **no discount** — the discount is its own entity.

## Multi-tenancy — non-negotiable

Coti is multi-tenant: one `account` = one corralón (its brand), with one or more `branch` branches. **Cross-account data exposure is a P0.** Every read and write to a tenant-scoped table is filtered by its tenancy column:

- **Every tenant-scoped table carries `account_id`** — including child tables (`product_price`, `quote_version`, `quote_item`, `quote_discount`, …). It is denormalized on purpose: it makes every policy and every `WHERE` a flat predicate instead of a join up the parent chain.
- **Branch-scoped tables also carry `branch_id`:** `channel`, `rfq`, `quote`, `combo`, `branch_product`, `product_price`. The catalog itself (`product`, `product_synonym`, `product_alternative`) is **account-scoped** — one product row per account, with per-branch availability, stock, and price in `branch_product` / `product_price`.
- **Postgres RLS is the second net.** Every tenant-scoped table has `ENABLE ROW LEVEL SECURITY` and a policy comparing `account_id` to `app_current_account_id()`, which reads a per-transaction GUC. The API connects as a restricted `NOBYPASSRLS` role, so a query that forgets its `account_id` predicate returns **zero rows instead of another tenant's data**. Branch scoping stays in the application (an admin legitimately reads across branches); only the account boundary is enforced in the database.
- **The GUC is set per transaction, by `InTenantTx`, via `set_config`.** `pgxpool` reuses connections, so the account cannot come from a per-connection hook — it changes per request. `InTenantTx` issues `SELECT set_config('app.current_account_id', $1, true)` as the transaction's first statement; `is_local = true` scopes it to the transaction so it cannot leak to whoever borrows the connection next. Note this is **not** `SET LOCAL`: that form accepts no bind parameters, which would force interpolating a request-derived value into SQL. A missing GUC fails closed (no rows) — never assume it carried over.

Rules:

- Repository methods take `accountID uuid.UUID` and/or `branchID uuid.UUID` as relevant and add it to **every** query — including the pgvector search. RLS is a backstop, not a substitute: keep the explicit predicate so queries stay index-friendly and readable.
- Services read `account_id` / `branch_id` from the request context populated by the tenant middleware — **never** from the request body.
- Do not expose a repository method that loads tenant-scoped data without a tenant argument. If a genuinely cross-tenant query is ever needed (internal tooling), name it with an explicit `CrossAccount` suffix so the missing filter is intentional and reviewable.

## The authentication boundary

Three pieces, in this order, and the order matters:

1. **`middleware.Authenticate(verifier, resolver)`** runs for every `/v1` route. It verifies the token's signature and hands the claims to `AuthService.ResolveTenant`, which checks everything the signature cannot — the user exists and is active, the session epoch is current, and the requested branch is one this caller may use — then calls `SetTenant`. A request with **no** Authorization header passes through unauthenticated rather than being rejected, so a public route can still see who the caller is when they happen to be logged in.
2. **`middleware.RequireTenant()`** guards the authenticated group and returns 401 when no tenant was resolved.
3. **`middleware.RequireAdmin()`** goes after `RequireTenant` on admin-only routes.

The token's signature covers `account_id`, which is what lets the middleware build a tenant scope **before** reading anything from the database — otherwise you need an account to run a query and a query to learn the account. The session-epoch check costs one indexed primary-key read per authenticated request; that is the price of immediate logout, and it is deliberate.

**The active branch is not a token claim.** A seller switches branch without re-authenticating, so it arrives in the `X-Branch-Id` header and is resolved per request — and **validated** against `branch.account_id` plus `user_branch` (admins skip the assignment check). This validation is load-bearing: RLS guards the account boundary, not the branch one, so a branch id taken from a request and trusted would let a caller read another branch of their own account. An absent header means account-wide; a present-but-inaccessible branch is a **403**, never a silent downgrade to account-wide, because the caller must not end up reading everything while believing they are scoped to one branch. A malformed value is a 400.

**By the time a service sees `Tenant.BranchID`, it is already validated.** Filter by it; do not re-check it.

**A rule about a value lives with the value, not with the route that happens to write it.**
`domain.PasswordPolicy` is the shape to copy: one type in `internal/domain`, built from config once
per service, and called by **every** path that stores a password — signup, admin user creation, the
self-service change, the recovery reset. Four routes had the same length check inline before, which
is three chances for one of them to fall behind. Two details of that policy generalise: a limit
imposed by a library is expressed in the library's own unit (bcrypt stops at 72 **bytes**, so the
cap is bytes and not characters, or the hash fails a write the input check should have refused), and
a rule that only applies when a value is **chosen** is not applied when it is merely **compared** —
logging in checks nothing, so a policy tightened later cannot lock out an account that predates it.

## Translating domain errors to HTTP

`handler.Respond(c, err)` is the **single** mapping point from a domain error to a status code. Services return `domain.ErrNotFound` / `ErrConflict` / `ErrUnauthenticated` / `ErrLocked` / `ErrForbidden` / `ErrImmutable` / `ErrInvalidInput`; the handler calls `Respond` and never picks a status itself. Anything unmapped becomes a 500 with a generic body and the real error attached to the request log — an unmapped error is a bug, and its text may not be safe to show a client.

**Every error also carries a stable `code`, and that is the part a client reads.** The status says how a request failed; the code says which rule refused it, which one status cannot when a route answers 422 for several reasons. `domain.CodeOf` derives a default from the sentinel; a service tags a specific one with `domain.WithCode(domain.CodeLastActiveBranch, fmt.Errorf("%w: …", domain.ErrInvalidInput))`, which leaves `errors.Is` matching the sentinel so nothing above changes. Tag a refusal the moment a caller has to tell it from a sibling on the same status, and add the constant to `internal/domain/error_code.go` rather than writing a literal at the call site.

**The `error` string is for a log, never for a screen** — the frontend owns its own wording, so a rewording is not a breaking change. And **a code must never distinguish what the status deliberately does not**: login answers `UNAUTHENTICATED` for a wrong password, an unknown address and a disabled user alike, because a code per case would hand back the enumeration the shared 401 exists to withhold.

`domain.ErrNotFound` covers "does not exist" **and** "belongs to another account". Under row level security those are indistinguishable, and they must stay that way: a distinct response would confirm another tenant's data exists.

## `quote.current_status` — backend-exclusive derived cache

The visible lifecycle is **split across two entities** — `rfq.status` (`RECEIVED`, `GENERATED`) and `quote.current_status` (`QUOTED`, `SENT`, `CHANGE_REQUESTED`, `ACCEPTED`, `REJECTED`) — not one status field. `quote.current_status` is a **derived cache written exclusively by the backend**: recompute it and append a `quote_status_change` row **in the same transaction** on every transition. A human or the AI **never** sets it directly. `rfq → quote` is 1-to-1 (enforced by `UNIQUE(rfq_id)`); never create a second quote for an RFQ. `archived_at` and `needs_followup` are orthogonal flags, not states.

## Where to create files

- **New feature (e.g. `quote`):** one file per layer — `internal/delivery/http/handler/quote_handler.go`, `internal/services/quote_service.go`, `internal/repository/quote_repo.go`, `internal/delivery/http/dto/quote_dto.go`, and `internal/domain/quote.go` for the types, enums, and errors.
- **New endpoint in an existing feature:** add the route + handler in the existing handler file, and the service/repository methods in the existing files. Split into a new file only when a file gets large or the sub-domain is clearly separate (e.g. `rfq` and `quote` are separate features, not one).
- **Schema change:** add a goose migration under `apps/api/migrations/` (run with `pnpm db:migrate`) — the only executable path — _and_ update the reference schema under `apps/api/database/` to match. The struct, the SQL column list, and the migration must agree — including the native enum type when you touch one.

## Utils vs helpers

- **utils** — generic, not tied to one entity or service, reused across features → `internal/utils/` (e.g. `internal/utils/timeutil`).
- **helpers** — tied to one service or entity (RFQ parsing helpers, quote-total math, the discount evaluator) → a file **next to that feature**, e.g. `internal/services/quote_helpers.go` used only by `quote_service.go`. Prefer one `<feature>_helpers.go` over a generic "helpers" package.

Rule: used by one service/entity only → **helper** file beside it. Generic and reusable → **utils**.

## Symbol ordering

In any file/package exporting several symbols, list public surface first, then internal helpers; alphabetical within a group unless a clear flow (CRUD, request lifecycle) dictates otherwise. Per-layer method ordering is in `api-methods-entities`.

## Verifying

Build and vet before you commit: `pnpm check:api` (runs `go build` + `go vet ./...`); golangci-lint (errcheck, govet, staticcheck, ineffassign, unused) runs in CI. See the `testing`, `commit`, and `pr-format` skills for the rest of the loop, and `agent-workflow` for the branch/ticket flow.

## Directory layout (apps/api/)

```
apps/api/
├── cmd/
│   ├── api/
│   │   └── main.go                     # composition root: config, pgxpool, adapters, router
│   ├── catalog-embed/
│   │   └── main.go                     # offline job: vectorize one account's catalog
│   └── scheduled-job/
│       └── main.go                     # one scheduled sweep, invoked by the platform's cron
├── internal/
│   ├── config/                         # env loading + defaults; all thresholds (top-K, cutoffs, timeouts)
│   ├── domain/                         # entities, value objects, enums, errors, AI port interfaces
│   ├── services/                       # <feature>_service.go (business logic; owns transactions + invariants)
│   ├── repository/                     # querier.go + <entity>_repo.go (raw pgx SQL; never commits)
│   ├── delivery/
│   │   └── http/
│   │       ├── handler/                # <feature>_handler.go (Gin handlers)
│   │       ├── middleware/             # JWT auth, tenant (account/branch) resolution, logging, rate limit
│   │       └── dto/                    # request/response DTOs (snake_case json + binding tags)
│   ├── ai/                             # shared policy (retry, usage log, Disabled* stand-ins)
│   │   ├── anthropic/                  # StructuredGenerator: schema-forced generation, vision, PDF
│   │   ├── openai/                     # Embedder + Transcriber
│   │   └── provider/                   # binds each port to its adapter; used by every cmd/
│   ├── mail/                           # adapters for the Mailer port (console by default)
│   ├── ratelimit/                      # request counters behind middleware.Limiter
│   └── utils/                          # generic, entity-agnostic helpers
├── database/                           # reference schema, read not applied (native enums, pgvector, UUID PKs)
└── migrations/                         # goose SQL migrations — the executable source (pnpm db:migrate)
```
