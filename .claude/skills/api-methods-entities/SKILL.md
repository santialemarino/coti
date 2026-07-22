---
name: api-methods-entities
description: API method order, comments, and entity conventions (DTOs, domain structs, enums) for raw pgx/SQL. Use when adding or reviewing API code in apps/api.
---

# API methods and entities (Coti backend)

`apps/api` is Go 1.25 + Gin over raw `database/sql` semantics with `pgx` / `pgxpool` against PostgreSQL + pgvector. **No GORM, no ORM** — explicit SQL, explicit scans. This skill covers method ordering, entity/struct conventions, and comments. For layer responsibilities, transactions, and the AI/pgvector patterns, see `api-layering`.

## Method order

**Repository:** (1) reads — general first (`List`, `GetByIDs`), then single (`GetByID`, `GetByEmail`). (2) `Create`. (3) `Update` / `Save`. (4) `Delete`. (5) anything else (upserts, semantic search, links) in the neatest order. Each read has a batch sibling — see below.

**Service:** CRUD → same order as the repository. A flow → the flow's order (e.g. `IngestarRFQ` → `ExtraerItems` → `MatchearCatalogo` → `GenerarCotizacion`). Entity-specific methods after the general ones.

**Handler:** CRUD → same order as the repository. A flow → endpoints in flow order. Otherwise: list/context, get by id, create, update, delete.

**More than one entity in a file:** **restart the order for each entity.** In `cotizacion_repo.go` handling quotes then line items: cotizacion block (list → get → create → update → delete), then `linea_cotizacion` block (list → create → …). Applies to handlers, services, and repositories.

## Pattern per layer

- **Handler:** bind the DTO (`c.ShouldBindJSON(&body)` with `binding:` tags), map DTO → domain, call the service, map domain → response DTO (`c.JSON(http.StatusOK, resp)`), or translate a domain error to a status code. No business logic, no SQL.
- **Service:** orchestrate the use case, call repositories and AI ports, work in domain types. Return domain errors (`domain.ErrNotFound`, `domain.ErrForbidden`); the handler maps them. **Owns the transaction** — `pool.Begin` / `tx.Commit`, passing `tx` to repos (see `api-layering`).
- **Repository:** SQL on the `Querier` it receives — `q.Query`, `q.QueryRow`, `q.Exec`. Scan rows into domain structs. No business rules. **Never** `Commit`/`Rollback`.

## Repository query patterns

### Explicit column lists, hand-scanned rows

Always name columns in `SELECT` (never `SELECT *`) and scan in the same order into the domain struct. The column list, the scan order, and the struct fields must agree with the canonical schema in `apps/api/database/`.

```go
// GetByID loads one product for an account. Returns domain.ErrNotFound if absent.
func (r *ProductoRepository) GetByID(ctx context.Context, q Querier, cuentaID, id int64) (*domain.Producto, error) {
    var p domain.Producto
    err := q.QueryRow(ctx,
        `SELECT id, cuenta_id, sucursal_id, codigo, nombre, unidad, precio_centavos, created_at, updated_at
         FROM producto
         WHERE cuenta_id = $1 AND id = $2`,
        cuentaID, id,
    ).Scan(&p.ID, &p.CuentaID, &p.SucursalID, &p.Codigo, &p.Nombre, &p.Unidad, &p.PrecioCentavos, &p.CreatedAt, &p.UpdatedAt)
    if errors.Is(err, pgx.ErrNoRows) {
        return nil, domain.ErrNotFound
    }
    if err != nil {
        return nil, err
    }
    return &p, nil
}
```

### Always provide batch variants

Every method that fetches by a single ID gets a batch sibling taking a slice, so callers never loop (N+1). Use `= ANY($n)` for the id set, and return a map keyed by ID when the caller looks up by ID.

```go
// Single.
func (r *ProductoRepository) GetByID(ctx context.Context, q Querier, cuentaID, id int64) (*domain.Producto, error)

// Batch — add this whenever the single version could be called in a loop.
func (r *ProductoRepository) GetByIDs(ctx context.Context, q Querier, cuentaID int64, ids []int64) (map[int64]domain.Producto, error)
```

```go
rows, err := q.Query(ctx,
    `SELECT id, cuenta_id, sucursal_id, codigo, nombre, unidad, precio_centavos, created_at, updated_at
     FROM producto
     WHERE cuenta_id = $1 AND id = ANY($2)`,
    cuentaID, ids)
```

### ON CONFLICT for upserts; batch for bulk writes

Never SELECT-then-INSERT/UPDATE by hand. Use `INSERT ... ON CONFLICT (...) DO UPDATE` (see `api-layering`). Bulk writes use a multi-row `INSERT` or `pgx.Batch`, never a per-row `Exec` loop. Every `ON CONFLICT` key includes `cuenta_id`.

### The service commits, not the repository

Repositories run on the `Querier` handed in (pool or tx) and never commit. Multi-step writes are wrapped in a service-level transaction. See the transaction rules in `api-layering`.

## Service patterns

### Batch-load before loops

Load all related data **before** iterating, then work in memory only.

```go
productos, err := s.productoRepo.GetByIDs(ctx, s.pool, cuentaID, ids) // map[int64]domain.Producto
if err != nil {
    return nil, err
}
for _, l := range lineas {
    p := productos[l.ProductoID]
    // ...
}
```

### Parallelize independent external calls

Independent AI/embedding calls run via `golang.org/x/sync/errgroup`, then results are persisted sequentially (a `pgx.Tx` is not concurrency-safe). Pattern in `api-layering`.

## DTOs (HTTP contract)

- **Request** and **response** structs live in `internal/delivery/http/dto/`. Bind requests via `ShouldBindJSON`; validate with `binding:` tags (`required`, `min`, `max`, `email`, `oneof`, `dive`). `json` tags are **snake_case** — the Coti API wire convention.
- Map DTO ↔ domain **at the handler boundary** with a small mapper (`toCotizacionResponse(c domain.Cotizacion) CotizacionResponse`). Never bind a request straight into a domain struct, and never serialize a domain struct straight to the wire.
- One doc comment per struct naming its role and the route it serves. Give a field a comment only when the name doesn't already say it.

```go
// CreateProductoRequest is the body for POST /v1/productos.
type CreateProductoRequest struct {
    SucursalID     int64  `json:"sucursal_id" binding:"required"`
    Codigo         string `json:"codigo" binding:"required,min=1,max=64"`
    Nombre         string `json:"nombre" binding:"required,min=1,max=255"`
    Unidad         string `json:"unidad" binding:"required,oneof=unidad m2 kg bolsa metro litro"`
    PrecioCentavos int64  `json:"precio_centavos" binding:"required,gt=0"` // price in ARS centavos
}

// ProductoResponse is returned by list, get, create, and update.
type ProductoResponse struct {
    ID             int64     `json:"id"`
    CuentaID       int64     `json:"cuenta_id"`
    SucursalID     int64     `json:"sucursal_id"`
    Codigo         string    `json:"codigo"`
    Nombre         string    `json:"nombre"`
    Unidad         string    `json:"unidad"`
    PrecioCentavos int64     `json:"precio_centavos"`
    CreatedAt      time.Time `json:"created_at"`
}
```

## Domain structs and enums (raw SQL — no ORM)

Domain structs in `internal/domain/` are plain Go — **no `gorm` tags, no `TableName()`, no ORM base struct.** They map to tables purely through the explicit column lists in repository SQL.

- **Fields:** plain Go types matching the column. IDs are `int64`. Money is stored and carried as **`int64` centavos** (ARS minor units) — never `float64` — to avoid rounding. Tenant keys `CuentaID` / `SucursalID` are explicit fields on every tenant-scoped struct.
- **Timestamps:** `time.Time` holding **naive UTC** (columns are `TIMESTAMP` storing UTC; the app is the source of `now()` in Go or `now()` in SQL — pick one per column and be consistent). Use `*time.Time` **only** when the column is genuinely nullable.
- **Nullable columns:** `*T` or a `pgtype` (e.g. `pgtype.Text`) — chosen per column, documented when not obvious.
- **Embeddings:** `pgvector.Vector` for `VECTOR(1536)` columns; the repository binds it directly.
- **Enums:** a typed `string` plus a `const (...)` block. Validate values in the service (or a DTO `oneof` tag) — Postgres stores them as plain text.

```go
// Producto is a catalog item owned by an account branch, matched against RFQ lines.
type Producto struct {
    ID             int64
    CuentaID       int64
    SucursalID     int64
    Codigo         string
    Nombre         string
    Unidad         UnidadMedida
    PrecioCentavos int64           // price in ARS centavos
    Embedding      pgvector.Vector // VECTOR(1536); semantic catalog search
    CreatedAt      time.Time
    UpdatedAt      time.Time
}

// CanalRFQ is the channel an RFQ arrived through.
type CanalRFQ string

const (
    CanalWhatsApp CanalRFQ = "whatsapp"
    CanalEmail    CanalRFQ = "email"
    CanalWeb      CanalRFQ = "web"
)

// EstadoCotizacion is the lifecycle state of a quote. Coti is human-in-the-loop:
// the AI pipeline lands a quote in EstadoBorrador for a sales rep to review.
type EstadoCotizacion string

const (
    EstadoBorrador  EstadoCotizacion = "borrador"
    EstadoEnRevision EstadoCotizacion = "en_revision"
    EstadoEnviada   EstadoCotizacion = "enviada"
    EstadoAceptada  EstadoCotizacion = "aceptada"
    EstadoRechazada EstadoCotizacion = "rechazada"
)
```

## Comments — Go doc comments

Use **Go doc comments** (`//` block) **directly above** the definition. For an exported symbol, start the comment with the symbol name (Go convention) and **end with a period.**

- **Handler:** 1–2 lines — what the endpoint does; optional returns / error status.
- **Service:** 1–2 lines — what it does; optional "Returns: …".
- **Repository:** 1–2 lines per method; one doc comment above the struct (`// ProductoRepository owns persistence for catalog products.`) and one above the constructor (`// NewProductoRepository builds a ProductoRepository.`).
- **DTOs:** one line naming role + route — `// CreateProductoRequest is the body for POST /v1/productos.`, `// UpdateProductoRequest is the body for PATCH /v1/productos/:id. Partial update; only provided fields are written.`, `// ProductoResponse is returned by list, get, create, and update.`
- **Domain structs / enums:** one line — `// Producto is a catalog item owned by an account branch...`, `// CanalRFQ is the channel an RFQ arrived through.`

No redundant comments (`// id is the id`), and no narration inside function bodies unless the implementation genuinely needs explaining. Exceptions to the end-with-a-period rule: inline comments on the same line as code, and short noun-phrase section labels (e.g. `// --- RFQ ---`).

## Verifying

`pnpm check:api` (`go build` + `go vet ./...`) must pass; golangci-lint (errcheck, govet, staticcheck, ineffassign, unused) runs in CI. See `testing` for tests, `commit` for the `type: imperative description` message, and `pr-format` / `agent-workflow` for the GitFlow branch and PR flow.
