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
- **internal/config/** — Env loading (`godotenv` is loaded in `main`) + defaults; the one place for every configurable threshold (match cutoffs, top-K, timeouts, default expiry days).
- **internal/utils/** — Generic, entity-agnostic helpers reused across features.
- **cmd/api/main.go** — Composition root: read config, open the `pgxpool.Pool`, construct AI adapters, inject repos + adapters into services, build the router, start the server. **The only place a port is bound to an adapter.** No business logic.

Do not put business logic in handlers or SQL strings in services. Do not let HTTP types (request/response bodies) leak into domain or repositories.

## Transaction rules

### The service owns the transaction; repositories never commit

Repositories accept a `Querier` — the read/write surface shared by `*pgxpool.Pool` and `pgx.Tx` — and run their SQL on it. They **never** call `Begin`, `Commit`, or `Rollback`. The service decides the boundary: one transaction per use case, so a multi-step write is atomic by default.

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

### Multi-step writes: Begin / defer Rollback / Commit

Open the transaction in the service, `defer tx.Rollback(ctx)` immediately (a no-op once committed — ignore its `ErrTxClosed`), pass `tx` to every repository call, and `Commit` last. Reads that don't need to be in the transaction can use the pool directly.

```go
// Service — owns the transaction. Registering an account seeds its head-office
// branch, its admin user, and the user↔branch link as one atomic unit.
func (s *AccountService) RegisterAccount(ctx context.Context, in domain.NewAccount) (*domain.Account, error) {
    tx, err := s.pool.Begin(ctx)
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

// Then: Begin → create quote → create quote_version (v1 draft) → insert
// quote_items → recompute quote_version.total → set quote.current_version_id →
// append quote_status_change → recompute quote.current_status → Commit.
```

## Performance rules

### Never query inside a loop (N+1)

Need data for N items? Fetch it in **one batch query** before the loop, then look it up from a map. Never call a repository method inside a `for`.

```go
// BAD — N+1: one query per quote item.
for _, it := range items {
    p, _ := s.productRepo.GetByID(ctx, s.pool, branchID, it.ProductID)
}

// GOOD — batch load, then loop in memory. quote_item.product_id is nullable
// (NO_MATCH items carry no product), so skip the nil ones.
ids := make([]uuid.UUID, 0, len(items))
for _, it := range items {
    if it.ProductID != nil {
        ids = append(ids, *it.ProductID)
    }
}
products, err := s.productRepo.GetByIDs(ctx, s.pool, branchID, ids) // map[uuid.UUID]domain.Product
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
// (quote_item is append-only: no updated_at, no in-place edits.)
_, err := q.Exec(ctx,
    `INSERT INTO quote_item
        (version_id, product_id, requested_description, quantity, unit,
         unit_price_snapshot, subtotal, confidence_score, match_status)
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
- **Every search is branch-scoped** — `product` hangs off `branch`, so filter `WHERE branch_id = $n` alongside the vector order.
- **Unmatched lines are flagged, never discarded.** A line with no acceptable match becomes a `quote_item` with `product_id` NULL and `match_status = NO_MATCH`; ambiguous ones get `AMBIGUOUS`. Every item carries a `confidence_score`. The search returns candidates; the service assigns `match_status`.

```go
// SearchByEmbedding returns the closest catalog matches to an RFQ line embedding,
// scoped to the branch, ordered by cosine distance.
func (r *ProductRepository) SearchByEmbedding(ctx context.Context, q Querier, branchID uuid.UUID, emb pgvector.Vector, limit int) ([]domain.ProductMatch, error) {
    rows, err := q.Query(ctx,
        `SELECT id, canonical_name, embedding <=> $1 AS distance
         FROM product
         WHERE branch_id = $2 AND is_active = TRUE
         ORDER BY embedding <=> $1
         LIMIT $3`,
        emb, branchID, limit)
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

- **Ports** are Go interfaces in **`internal/domain`** (so both services and adapters import them without a cycle):

  ```go
  // RFQExtractor parses an informal RFQ (WhatsApp/email/web) into structured line items.
  type RFQExtractor interface {
      Extract(ctx context.Context, raw string) ([]RFQLine, error)
  }

  // Embedder produces 1536-dim embeddings for catalog and RFQ text.
  type Embedder interface {
      Embed(ctx context.Context, texts []string) ([]pgvector.Vector, error)
  }

  // ChangeRequestHandler interprets a client change request against the CLOSED
  // action catalog and returns a typed, schema-validated proposal — never free JSON.
  type ChangeRequestHandler interface {
      Propose(ctx context.Context, in ChangeRequestInput) (ChangeProposal, error)
  }
  ```

- **Adapters** implement them in **`internal/ai/`** (per-provider subpackage when there is more than one). All prompt assembly, SDK calls, retries, and response parsing live there.
- The **service depends on the interface**, so swapping providers is a one-line change in `cmd/api/main.go`. It never imports `internal/ai`.

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
- `ON_TOTAL` is computed on the **net** (line discounts first, then total). Round to 2 decimals per discount. The result **floors at `product_price.min_price`** and never goes negative.
- Conflicts resolve by `is_exclusive` + `priority` (higher priority wins; tie → larger discount; not stackable by default).
- `quote_version.total = Σ quote_item.subtotal − Σ quote_discount.amount`. `quote_item` itself holds a price snapshot and **no discount** — the discount is its own entity.

## Multi-tenancy — non-negotiable

Coti is multi-tenant: one `account` = one corralón (its brand), with one or more `branch` branches. **Cross-account data exposure is a P0.** Every read and write to a tenant-scoped table is filtered by its tenancy column:

- **Account-scoped tables carry `account_id`:** `branch`, `app_user`, `client`, `tag`, `promotion`.
- **Branch-scoped tables carry `branch_id`:** `product`, `channel`, `rfq`, `quote`, `combo` (and `promotion` optionally, `branch_id` NULL = whole account). A `branch` always resolves to an `account`, so branch scoping implies account isolation once the branch is validated against the caller's account.
- **Tables one level down inherit tenancy through their parent FK** — `product_price` → `product`, `product_synonym` → `product`, `combo_item` → `combo`, `quote_version`/`quote_item` → `quote`. Scope them by joining to the parent or by resolving the parent first; there is no `account_id`/`branch_id` column on them.

Rules:

- Repository methods take `accountID uuid.UUID` and/or `branchID uuid.UUID` as relevant and add it to **every** query — including the pgvector search and every `WHERE` on a child table's parent.
- Services read `account_id` / `branch_id` from the request context populated by the tenant middleware — **never** from the request body.
- Do not expose a repository method that loads tenant-scoped data without a tenant argument. If a genuinely cross-tenant query is ever needed (internal tooling), name it with an explicit `CrossAccount` suffix so the missing filter is intentional and reviewable.

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
│   └── api/
│       └── main.go                     # composition root: config, pgxpool, adapters, router
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
│   ├── ai/                             # adapters for RFQExtractor / Embedder / ChangeRequestHandler ports
│   └── utils/                          # generic, entity-agnostic helpers
├── database/                           # reference schema, read not applied (native enums, pgvector, UUID PKs)
└── migrations/                         # goose SQL migrations — the executable source (pnpm db:migrate)
```
