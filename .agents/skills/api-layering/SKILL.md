---
name: api-layering
description: Backend API structure and where to create files (handlers, services, repositories, DTOs, domain, AI adapters, utils). Use when adding endpoints, new features, or organizing code in apps/api.
---

# API layering (Coti backend)

`apps/api` is Go 1.25 + Gin, laid out as **layered / hexagonal** (ports and adapters). Layers depend inward — handlers → services → domain — and external concerns (the AI extraction/embedding providers, and anything else off-process) sit behind Go interfaces (ports) implemented by adapters.

The data layer is **raw `database/sql` semantics over `pgx` / `pgxpool`** against PostgreSQL + pgvector. There is **no GORM and no ORM**: every query is explicit SQL with `$1` placeholders, and every row is scanned by hand into a domain struct. Do not add an ORM.

## Flow and layers

Request flow: **handler → service → repository → DB**.

- **internal/delivery/http/handler/** — HTTP only: bind the DTO, call the service, translate the result or domain error into a Gin response. No business logic, no SQL.
- **internal/delivery/http/dto/** — Request/response structs (the wire contract; `json` tags snake_case). Bound via `c.ShouldBindJSON`, validated with `binding:` tags, mapped to/from domain types **in the handler**.
- **internal/delivery/http/middleware/** — Gin middleware (JWT auth, tenant resolution that puts `cuenta_id`/`sucursal_id` on the context, request logging, rate limiting). Wired by the router setup.
- **internal/services/** — Business logic: orchestrate use cases, call repositories and AI ports. Work in **domain** types; no HTTP, no SQL strings. **Owns the transaction boundary** (see below). Depends only on ports (interfaces), never on a concrete adapter package.
- **internal/repository/** — Data access: explicit SQL through a `Querier` (a `*pgxpool.Pool` or a `pgx.Tx`) handed in by the service. Scans rows into domain structs. **Never** calls `Commit`/`Rollback`.
- **internal/domain/** — Domain types, value objects, enums, domain errors, and the **port interfaces** the services consume (e.g. the AI `Embedder` / `RFQExtractor`). Imports no other `internal/` package.
- **internal/ai/** — Adapters implementing the domain AI ports (RFQ extraction, embedding generation, catalog re-ranking). All provider-specific logic (SDK calls, prompt assembly, response parsing) lives here, behind the port. Per-provider subpackages when there is more than one (e.g. `internal/ai/openai/`).
- **internal/config/** — Env loading (`godotenv` is loaded in `main`) + defaults; the one place for every configurable threshold (match cutoffs, top-K, timeouts).
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
// Service — owns the transaction.
func (s *CuentaService) RegistrarCuenta(ctx context.Context, in domain.AltaCuenta) (*domain.Cuenta, error) {
    tx, err := s.pool.Begin(ctx)
    if err != nil {
        return nil, err
    }
    defer tx.Rollback(ctx) // no-op after Commit

    cuenta, err := s.cuentaRepo.Create(ctx, tx, in.Cuenta)
    if err != nil {
        return nil, err
    }
    sucursal, err := s.sucursalRepo.Create(ctx, tx, cuenta.ID, in.SucursalCasaMatriz)
    if err != nil {
        return nil, err
    }
    if _, err := s.usuarioRepo.Create(ctx, tx, cuenta.ID, in.Admin); err != nil {
        return nil, err
    }
    if err := s.usuarioSucursalRepo.Vincular(ctx, tx, in.Admin.ID, sucursal.ID); err != nil {
        return nil, err
    }
    if err := tx.Commit(ctx); err != nil {
        return nil, err
    }
    return cuenta, nil
}
```

### Keep external calls out of the transaction

AI extraction and embedding are slow and can fail on their own timeline. Run them **before** you open the transaction, then open a short transaction only around the writes. Never hold a `pgx.Tx` open across an AI provider call.

## Performance rules

### Never query inside a loop (N+1)

Need data for N items? Fetch it in **one batch query** before the loop, then look it up from a map. Never call a repository method inside a `for`.

```go
// BAD — N+1: one query per RFQ line.
for _, l := range lineas {
    p, _ := s.productoRepo.GetByID(ctx, s.pool, cuentaID, l.ProductoID)
}

// GOOD — batch load, then loop in memory.
ids := make([]int64, 0, len(lineas))
for _, l := range lineas {
    ids = append(ids, l.ProductoID)
}
productos, err := s.productoRepo.GetByIDs(ctx, s.pool, cuentaID, ids) // map[int64]domain.Producto
for _, l := range lineas {
    p := productos[l.ProductoID]
}
```

When you add a repository method that will be called in a loop, **always add a batch variant** (accepts a slice of IDs, returns `map[int64]T` or `map[int64][]T` keyed by ID). See `api-methods-entities`.

### Parallelize independent external calls with errgroup

When you make N independent external calls — e.g. the embedding provider takes one text per request, or you extract from several RFQ attachments at once — use `golang.org/x/sync/errgroup`, not a sequential loop. A single `pgx.Tx` is **not** safe for concurrent use, so **parallelize the external calls, then persist sequentially.**

```go
// GOOD — parallel embed, sequential persist.
g, gctx := errgroup.WithContext(ctx)
embs := make([]pgvector.Vector, len(items))
for i := range items {
    i := i
    g.Go(func() error {
        v, err := s.embedder.EmbedOne(gctx, items[i].Descripcion)
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

### Use ON CONFLICT for upserts, batch for bulk writes

Never SELECT-then-INSERT/UPDATE by hand. For catalog imports, synonym syncs, or re-ingesting an RFQ, use `INSERT ... ON CONFLICT (...) DO UPDATE`. For many rows, use a multi-row `INSERT` or `pgx.Batch` — never a per-row loop of `Exec`.

```go
_, err := q.Exec(ctx,
    `INSERT INTO producto (cuenta_id, sucursal_id, codigo, nombre, unidad, precio_centavos, updated_at)
     VALUES ($1, $2, $3, $4, $5, $6, now())
     ON CONFLICT (cuenta_id, codigo) DO UPDATE
       SET nombre = EXCLUDED.nombre,
           unidad = EXCLUDED.unidad,
           precio_centavos = EXCLUDED.precio_centavos,
           updated_at = now()`,
    p.CuentaID, p.SucursalID, p.Codigo, p.Nombre, p.Unidad, p.PrecioCentavos)
```

### Prefer expressive operations over accumulator loops

When intent maps to sum / max / filter / group, write it that way with `slices` helpers or a small typed helper. Reserve `for` for early `break`, per-iteration side effects, or where it is genuinely clearer.

## Semantic catalog search (pgvector)

Catalog matching is the core of the RFQ pipeline. `producto.embedding` is `VECTOR(1536)` (pgvector extension `"vector"`); `sinonimo_producto` holds curated synonyms for lexical fallback.

- Query with pgvector's distance operators: **`<=>` cosine**, **`<->` L2**. Order by the distance and `LIMIT` the top-K. Coti uses `<=>` (cosine) as the default — bake the K and any cutoff in `internal/config`, not inline.
- Pass the query vector as a `pgvector.Vector` bind param (`pgvector.NewVector([]float32{...})`), never string-interpolate it.
- Combine semantic hits with `sinonimo_producto` matches in the **repository**; the service decides confidence, not the SQL.
- **Every search is tenant-scoped** — `WHERE cuenta_id = $n` (and `sucursal_id` when the catalog is per-branch) alongside the vector order.

```go
// SearchByEmbedding returns the closest catalog matches to an RFQ line embedding,
// scoped to the account, ordered by cosine distance.
func (r *ProductoRepository) SearchByEmbedding(ctx context.Context, q Querier, cuentaID int64, emb pgvector.Vector, limit int) ([]domain.ProductoMatch, error) {
    rows, err := q.Query(ctx,
        `SELECT id, nombre, precio_centavos, embedding <=> $1 AS distancia
         FROM producto
         WHERE cuenta_id = $2
         ORDER BY embedding <=> $1
         LIMIT $3`,
        emb, cuentaID, limit)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var matches []domain.ProductoMatch
    for rows.Next() {
        var m domain.ProductoMatch
        if err := rows.Scan(&m.ProductoID, &m.Nombre, &m.PrecioCentavos, &m.Distancia); err != nil {
            return nil, err
        }
        matches = append(matches, m)
    }
    return matches, rows.Err()
}
```

**Embedding generation never lives in the repository or the SQL.** The repository receives an already-computed `pgvector.Vector`; the service gets it from the AI port (below).

## AI providers — ports and adapters

The RFQ pipeline (extract informal RFQ text → line items → embed → match against the catalog → assemble a review-ready `cotizacion`) talks to external models only through domain ports:

- **Ports** are Go interfaces in **`internal/domain`** (so both services and adapters import them without a cycle):

  ```go
  // RFQExtractor parses an informal RFQ (WhatsApp/email/web) into structured line items.
  type RFQExtractor interface {
      Extract(ctx context.Context, raw string) ([]ItemExtraido, error)
  }

  // Embedder produces 1536-dim embeddings for catalog and RFQ text.
  type Embedder interface {
      Embed(ctx context.Context, texts []string) ([]pgvector.Vector, error)
  }
  ```

- **Adapters** implement them in **`internal/ai/`** (per-provider subpackage when there is more than one). All prompt assembly, SDK calls, retries, and response parsing live there.
- The **service depends on the interface**, so swapping providers is a one-line change in `cmd/api/main.go`. It never imports `internal/ai`.
- Coti is a **human-in-the-loop copilot**: the pipeline produces a _draft_ quote for a sales rep to review. Services must persist AI output as a reviewable `cotizacion` in a draft state — never auto-send.

## Multi-tenancy — non-negotiable

Coti is multi-tenant: one `cuenta` = one corralón, with one or more `sucursal` branches. **Every read and write to a tenant-scoped table filters by `cuenta_id`, and by `sucursal_id` too when the data is per-branch.**

- Repository methods take `cuentaID int64` (and `sucursalID int64` where relevant) and add it to **every** query — including the pgvector search and every `ON CONFLICT` key.
- Services read `cuenta_id` / `sucursal_id` from the request context populated by the tenant middleware — never from the request body.
- Do not expose a repository method that loads tenant-scoped data without a `cuentaID` argument. If a genuinely cross-tenant query is ever needed (internal tooling), name it with an explicit `CrossCuenta` suffix so the missing filter is intentional and reviewable.

## Where to create files

- **New feature (e.g. `cotizacion`):** one file per layer — `internal/delivery/http/handler/cotizacion_handler.go`, `internal/services/cotizacion_service.go`, `internal/repository/cotizacion_repo.go`, `internal/delivery/http/dto/cotizacion_dto.go`, and `internal/domain/cotizacion.go` for the types, enums, and errors.
- **New endpoint in an existing feature:** add the route + handler in the existing handler file, and the service/repository methods in the existing files. Split into a new file only when a file gets large or the sub-domain is clearly separate.
- **Schema change:** update the canonical schema at `apps/api/database/` _and_ add a goose migration under `apps/api/migrations/` (run with `pnpm db:migrate`). The struct, the SQL column list, and the migration must agree.

## Utils vs helpers

- **utils** — generic, not tied to one entity or service, reused across features → `internal/utils/` (e.g. `internal/utils/timeutil`).
- **helpers** — tied to one service or entity (RFQ parsing helpers, quote-total math) → a file **next to that feature**, e.g. `internal/services/cotizacion_helpers.go` used only by `cotizacion_service.go`. Prefer one `<feature>_helpers.go` over a generic "helpers" package.

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
│   ├── services/                       # <feature>_service.go (business logic; owns transactions)
│   ├── repository/                     # querier.go + <entity>_repo.go (raw pgx SQL; never commits)
│   ├── delivery/
│   │   └── http/
│   │       ├── handler/                # <feature>_handler.go (Gin handlers)
│   │       ├── middleware/             # JWT auth, tenant (cuenta/sucursal) resolution, logging, rate limit
│   │       └── dto/                    # request/response DTOs (snake_case json + binding tags)
│   ├── ai/                             # adapters for RFQExtractor / Embedder ports (per-provider subpackages)
│   └── utils/                          # generic, entity-agnostic helpers
├── database/                           # canonical schema (source of truth; pgvector extension)
└── migrations/                         # goose SQL migrations (pnpm db:migrate)
```
