---
name: api-methods-entities
description: API method order, comments, and entity conventions (DTOs, domain structs, enums) for raw pgx/SQL. Use when adding or reviewing API code in apps/api.
---

# API methods and entities (Coti backend)

`apps/api` is Go 1.25 + Gin over raw `database/sql` semantics with `pgx` / `pgxpool` against PostgreSQL + pgvector. **No GORM, no ORM** — explicit SQL, explicit scans. This skill covers method ordering, entity/struct conventions, and comments. For layer responsibilities, transactions, multi-tenancy, and the AI/pgvector/discount patterns, see `api-layering`.

Three schema facts you carry into every type here (details below): **PKs are UUID v4** (`uuid.UUID`, never `int64`); **money and quantities are `NUMERIC(14,2)`** (a decimal type, exposed as a decimal string, never a float); **enums are native PostgreSQL enum types** (a typed Go string whose values match the DB values exactly, UPPERCASE English).

## Method order

**Repository:** (1) reads — general first (`List`, `GetByIDs`), then single (`GetByID`, `GetByEmail`). (2) `Create`. (3) `Update` / `Save`. (4) `Delete`. (5) anything else (upserts, semantic search, links) in the neatest order. Each read has a batch sibling — see below.

**Service:** CRUD → same order as the repository. A flow → the flow's order (e.g. `IngestRFQ` → `ExtractItems` → `MatchCatalog` → `GenerateQuote`). Entity-specific methods after the general ones.

**Handler:** CRUD → same order as the repository. A flow → endpoints in flow order. Otherwise: list/context, get by id, create, update, delete.

**More than one entity in a file:** **restart the order for each entity.** In `quote_repo.go` handling quotes then versions then line items: `quote` block (list → get → create → update → delete), then `quote_version` block, then `quote_item` block. Applies to handlers, services, and repositories.

## Pattern per layer

- **Handler:** bind the DTO (`c.ShouldBindJSON(&body)` with `binding:` tags), map DTO → domain, call the service, map domain → response DTO (`c.JSON(http.StatusOK, resp)`), or translate a domain error to a status code. No business logic, no SQL. The active `account_id` / `branch_id` come from the tenant context, never the body.
- **Service:** orchestrate the use case, call repositories and AI ports, work in domain types. Return domain errors (`domain.ErrNotFound`, `domain.ErrForbidden`); the handler maps them. **Owns the transaction** — `pool.Begin` / `tx.Commit`, passing `tx` to repos (see `api-layering`). **Owns every invariant** — state×intention validation, discount math, `quote.current_status` derivation.
- **Repository:** SQL on the `Querier` it receives — `q.Query`, `q.QueryRow`, `q.Exec`. Scan rows into domain structs. No business rules. **Never** `Commit`/`Rollback`.

## Repository query patterns

### Explicit column lists, hand-scanned rows

Always name columns in `SELECT` (never `SELECT *`) and scan in the same order into the domain struct. The column list, the scan order, and the struct fields must agree with the reference schema under `apps/api/database/`.

```go
// GetByID loads one product for an account. Returns domain.ErrNotFound if absent.
func (r *ProductRepository) GetByID(ctx context.Context, q Querier, accountID, id uuid.UUID) (*domain.Product, error) {
    var p domain.Product
    err := q.QueryRow(ctx,
        `SELECT id, account_id, code, canonical_name, unit, category, is_active, created_at, updated_at
         FROM product
         WHERE account_id = $1 AND id = $2`,
        accountID, id,
    ).Scan(&p.ID, &p.AccountID, &p.Code, &p.CanonicalName, &p.Unit, &p.Category, &p.IsActive, &p.CreatedAt, &p.UpdatedAt)
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

Every method that fetches by a single ID gets a batch sibling taking a slice, so callers never loop (N+1). Use `= ANY($n)` over a `[]uuid.UUID`, and return a `map[uuid.UUID]T` when the caller looks up by ID.

```go
// Single.
func (r *ProductRepository) GetByID(ctx context.Context, q Querier, accountID, id uuid.UUID) (*domain.Product, error)

// Batch — add this whenever the single version could be called in a loop.
func (r *ProductRepository) GetByIDs(ctx context.Context, q Querier, accountID uuid.UUID, ids []uuid.UUID) (map[uuid.UUID]domain.Product, error)
```

```go
rows, err := q.Query(ctx,
    `SELECT id, account_id, code, canonical_name, unit, category, is_active, created_at, updated_at
     FROM product
     WHERE account_id = $1 AND id = ANY($2)`,
    accountID, ids)
```

### ON CONFLICT for idempotent writes; batch for bulk writes

Never SELECT-then-INSERT/UPDATE by hand. Use `INSERT ... ON CONFLICT (...)` — but only against a **real unique constraint** from the schema (e.g. `uq_app_user_email` on `(account_id, email)`, `uq_client_tag` on `(client_id, tag_id)`, `uq_quote_version` on `(quote_id, version_number)`). Bulk writes use a multi-row `INSERT` or `pgx.Batch`, never a per-row `Exec` loop. Details and examples in `api-layering`.

### The service commits, not the repository

Repositories run on the `Querier` handed in (pool or tx) and never commit. Multi-step writes are wrapped in a service-level transaction. See the transaction rules in `api-layering`.

## Service patterns

### Batch-load before loops

Load all related data **before** iterating, then work in memory only. `quote_item.product_id` is nullable (NO_MATCH lines carry no product), so skip the nil IDs when building the batch key set.

```go
products, err := s.productRepo.GetByIDs(ctx, s.pool, accountID, ids) // map[uuid.UUID]domain.Product
if err != nil {
    return nil, err
}
for _, it := range items {
    if it.ProductID == nil {
        continue // NO_MATCH — flagged, not discarded.
    }
    p := products[*it.ProductID]
    // ...
}
```

### Parallelize independent external calls

Independent AI/embedding calls run via `golang.org/x/sync/errgroup`, then results are persisted sequentially (a `pgx.Tx` is not concurrency-safe). Pattern in `api-layering`.

## DTOs (HTTP contract)

- **Request** and **response** structs live in `internal/delivery/http/dto/`. Bind requests via `ShouldBindJSON`; validate with `binding:` tags (`required`, `min`, `max`, `email`, `oneof`, `dive`). `json` tags are **snake_case** — the Coti API wire convention.
- Map DTO ↔ domain **at the handler boundary** with a small mapper (`toQuoteVersionResponse(v domain.QuoteVersion) QuoteVersionResponse`). Never bind a request straight into a domain struct, and never serialize a domain struct straight to the wire.
- **Money and quantities are decimal strings on the wire — never floats, never centavo ints.** Accept them as `string` (validate `numeric`) and parse to a decimal server-side; serialize them from the decimal's `.String()`. A JSON number would lose `NUMERIC(14,2)` precision on the round-trip, so this is a hard rule.
- IDs are `uuid.UUID` (marshals as a canonical UUID string).
- One doc comment per struct naming its role and the route it serves. Give a field a comment only when the name doesn't already say it.

```go
// CreateProductRequest is the body for POST /v1/products. The account comes from
// the tenant context, never the body.
type CreateProductRequest struct {
    Code          *string `json:"code" binding:"omitempty,max=255"`
    CanonicalName string  `json:"canonical_name" binding:"required,min=1,max=255"`
    Unit          *string `json:"unit" binding:"omitempty,max=64"`
    Category      *string `json:"category" binding:"omitempty,max=255"`
}

// SetProductPriceRequest is the body for POST /v1/products/:id/prices. Money is a
// decimal STRING, parsed to a decimal server-side — never a float.
type SetProductPriceRequest struct {
    Price     string    `json:"price" binding:"required,numeric"`
    Currency  string    `json:"currency" binding:"required,len=3"`       // e.g. "ARS".
    MinPrice  *string   `json:"min_price" binding:"omitempty,numeric"`   // discount-engine floor.
    ValidFrom time.Time `json:"valid_from" binding:"required"`
}

// QuoteVersionResponse is returned by GET /v1/quotes/:id/versions/:n. total is the
// frozen decimal string: Σ item subtotals − Σ discounts.
type QuoteVersionResponse struct {
    ID            uuid.UUID `json:"id"`
    QuoteID       uuid.UUID `json:"quote_id"`
    VersionNumber int       `json:"version_number"`
    Total         string    `json:"total"`         // decimal string, never float.
    IsImmutable   bool      `json:"is_immutable"`
    CreatedAt     time.Time `json:"created_at"`
}
```

## Domain structs and enums (raw SQL — no ORM)

Domain structs in `internal/domain/` are plain Go — **no `gorm` tags, no `TableName()`, no ORM base struct.** They map to tables purely through the explicit column lists in repository SQL.

- **IDs:** `uuid.UUID` (`github.com/google/uuid`) or `pgtype.UUID` — never `int64`. Tenant keys are explicit fields where the table carries them: `AccountID` on account-scoped structs, `BranchID` on branch-scoped ones. Child tables that inherit tenancy through a parent FK carry only the parent ID (e.g. `quote_item.VersionID`).
- **Money and quantities:** a decimal type for every `NUMERIC(14,2)` column — `decimal.Decimal` (`github.com/shopspring/decimal`, register the pgx codec in `main`) or `pgtype.Numeric`. **Never `float64`, never `int64` centavos.** Nullable numerics use `decimal.NullDecimal` or `*decimal.Decimal`.
- **Timestamps:** `time.Time` for `TIMESTAMPTZ`. **`created_at` exists on every table**; **`updated_at` only on in-place-mutable tables** (`account`, `branch`, `app_user`, `product`, `combo`, `client`, `channel`, `rfq`, `quote`, `promotion`), maintained by a `set_updated_at()` trigger. **Append-only / immutable tables have no `updated_at`** — `quote_version`, `quote_item`, `rfq_status_change`, `quote_status_change`, `handler_decision`, `product_price`, `quote_message`, and the bridge tables — so do not put an `UpdatedAt` field on their structs. `quote_item` rows on a **non-frozen** version are still editable in place (the seller edits the draft); the service rejects any mutation whose parent version has `is_immutable = TRUE`. Use `*time.Time` for genuinely nullable timestamps (`archived_at`, `expires_at`, `valid_to`).
- **Nullable columns:** `*T` or a `pgtype` (e.g. `pgtype.Text`) — chosen per column, documented when not obvious. `quote_item.product_id` is nullable by design (NO_MATCH).
- **Embeddings:** `pgvector.Vector` for the `VECTOR(1536)` `product.embedding` column; the repository binds it directly.
- **Enums:** the schema defines **native PostgreSQL enum types**. Model each as a typed `string` plus a `const (...)` block whose values **exactly match the DB enum values** (UPPERCASE English). pgx scans and writes them as the native enum. Validate incoming values in the service (or a DTO `oneof` tag).

```go
// Product is a catalog item owned by an account, matched against RFQ lines.
type Product struct {
    ID            uuid.UUID
    AccountID     uuid.UUID           // catalog is account-scoped; availability/stock live in branch_product.
    Code          *string             // nullable.
    CanonicalName string
    Unit          *string             // nullable; free text (m2, kg, bolsa, ...).
    Category      *string             // nullable.
    Embedding     pgvector.Vector     // VECTOR(1536); semantic catalog search.
    IsActive      bool
    CreatedAt     time.Time
    UpdatedAt     time.Time           // product is mutable in place → has updated_at.
}

// QuoteVersion is an immutable, versioned snapshot of a quote. Append-only: no
// updated_at. total = Σ quote_item.subtotal − Σ quote_discount.amount.
type QuoteVersion struct {
    ID            uuid.UUID
    QuoteID       uuid.UUID
    AuthorID      uuid.UUID
    VersionNumber int
    Total         decimal.Decimal // NUMERIC(14,2).
    IsImmutable   bool            // draft = false; frozen = true.
    Comment       *string
    CreatedAt     time.Time
}

// QuoteStatus is the lifecycle state a quote carries once it exists. The full
// lifecycle is SPLIT across two entities: rfq.status (RECEIVED, GENERATED) covers
// the pre-quote phase; QuoteStatus covers QUOTED onward. quote.current_status is a
// backend-exclusive derived cache — never set by a human or the AI (see api-layering).
type QuoteStatus string

const (
    QuoteStatusQuoted          QuoteStatus = "QUOTED"
    QuoteStatusSent            QuoteStatus = "SENT"
    QuoteStatusChangeRequested QuoteStatus = "CHANGE_REQUESTED"
    QuoteStatusAccepted        QuoteStatus = "ACCEPTED"
    QuoteStatusRejected        QuoteStatus = "REJECTED"
)

// RFQStatus is the pre-quote lifecycle state, living on the rfq entity.
type RFQStatus string

const (
    RFQStatusReceived  RFQStatus = "RECEIVED"
    RFQStatusGenerated RFQStatus = "GENERATED"
)

// ItemMatchStatus is the catalog-match outcome for a quote line. NO_MATCH lines
// are flagged (product_id NULL), never discarded.
type ItemMatchStatus string

const (
    ItemMatchStatusMatched   ItemMatchStatus = "MATCHED"
    ItemMatchStatusAmbiguous ItemMatchStatus = "AMBIGUOUS"
    ItemMatchStatusNoMatch   ItemMatchStatus = "NO_MATCH"
)

// ChannelType is the channel a client contact / RFQ arrived through.
type ChannelType string

const (
    ChannelTypeWhatsApp ChannelType = "WHATSAPP"
    ChannelTypeEmail    ChannelType = "EMAIL"
    ChannelTypeWebApp   ChannelType = "WEBAPP"
    ChannelTypeCounter  ChannelType = "COUNTER"
    ChannelTypePhone    ChannelType = "PHONE"
)

// UserRole is the role of an app_user within an account.
type UserRole string

const (
    UserRoleAdmin  UserRole = "ADMIN"
    UserRoleSeller UserRole = "SELLER"
)
```

Discount-engine enums follow the same shape and are worth encoding verbatim, since they are the deterministic engine's vocabulary:

```go
// PromotionConditionType is the closed set of promotion condition kinds. ITEM_SET
// (not COMBO — that name is the catalog combo entity) covers a set of lines.
type PromotionConditionType string

const (
    PromotionConditionPerItem        PromotionConditionType = "PER_ITEM"
    PromotionConditionQuantityTiered PromotionConditionType = "QUANTITY_TIERED"
    PromotionConditionItemSet        PromotionConditionType = "ITEM_SET"
    PromotionConditionOnTotal        PromotionConditionType = "ON_TOTAL"
)

// PromotionActionType is how a promotion changes price. SPECIAL_PRICE is only
// valid for PER_ITEM / QUANTITY_TIERED.
type PromotionActionType string

const (
    PromotionActionPercentage   PromotionActionType = "PERCENTAGE"
    PromotionActionFixedAmount  PromotionActionType = "FIXED_AMOUNT"
    PromotionActionSpecialPrice PromotionActionType = "SPECIAL_PRICE"
)

// DiscountScope is the reach of an applied discount, derived from the rule's type
// and persisted on quote_discount.
type DiscountScope string

const (
    DiscountScopeItem    DiscountScope = "ITEM"
    DiscountScopeItemSet DiscountScope = "ITEM_SET"
    DiscountScopeTotal   DiscountScope = "TOTAL"
)

// DiscountOrigin is where an applied discount came from. AI_ADAPTATION means the
// AI reshaped the quote to reach a promo — the engine still computed the amount.
type DiscountOrigin string

const (
    DiscountOriginAutomatic    DiscountOrigin = "AUTOMATIC"
    DiscountOriginAIAdaptation DiscountOrigin = "AI_ADAPTATION"
    DiscountOriginManualSeller DiscountOrigin = "MANUAL_SELLER"
)

// HandlerSellerDecision is what the seller did with an AI proposal (handler_decision,
// Nivel 1 log). Append-only.
type HandlerSellerDecision string

const (
    HandlerDecisionApprovedAsIs   HandlerSellerDecision = "APPROVED_AS_IS"
    HandlerDecisionEdited         HandlerSellerDecision = "EDITED"
    HandlerDecisionRejected       HandlerSellerDecision = "REJECTED"
    HandlerDecisionManualOverride HandlerSellerDecision = "MANUAL_OVERRIDE"
)
```

The remaining native enums follow the same pattern — encode them the same way when you touch their features: `product_alternative_type` (EQUIVALENT, PREMIUM, ECONOMY), `quote_item_alternative_type` (PRODUCT, COMBO), `quote_item_alternative_origin` (AI, SELLER), `client_action_type` (ACCEPT, REJECT, REQUEST_CHANGE, COMMENT), `attachment_type` (IMAGE, PDF, SPREADSHEET, AUDIO, TEXT), `attachment_processing_status` (PENDING, PROCESSING, DONE, FAILED), `send_format` (WEBAPP_LINK, PDF, MESSAGE), `send_tracking_status` (PENDING, SENT, DELIVERED, VIEWED, FAILED), `notification_status` (PENDING, SENT, FAILED).

## Comments — Go doc comments

Use **Go doc comments** (`//` block) **directly above** the definition. For an exported symbol, start the comment with the symbol name (Go convention) and **end with a period.**

- **Handler:** 1–2 lines — what the endpoint does; optional returns / error status.
- **Service:** 1–2 lines — what it does; optional "Returns: …".
- **Repository:** 1–2 lines per method; one doc comment above the struct (`// ProductRepository owns persistence for catalog products.`) and one above the constructor (`// NewProductRepository builds a ProductRepository.`).
- **DTOs:** one line naming role + route — `// CreateProductRequest is the body for POST /v1/products.`, `// UpdateProductRequest is the body for PATCH /v1/products/:id. Partial update; only provided fields are written.`, `// ProductResponse is returned by list, get, create, and update.`
- **Domain structs / enums:** one line — `// Product is a catalog item owned by a branch...`, `// QuoteStatus is the lifecycle state a quote carries once it exists.`

**Only the essential ones.** Exported symbols carry a doc comment (Go convention) — keep it to **one line** that says what the caller needs. Everything else has to earn its place: the bar is _would a competent reader get this wrong without it?_ Comment a non-obvious **why**, a constraint that looks arbitrary, or a footgun.

Concrete limits, because "be brief" is too loose to bind:

- **One comment, one line.** Two if genuinely needed. A paragraph is not a comment — it is a `docs/technical/` section, and the comment points at it.
- **Never narrate rejected alternatives** ("uses `NOT EXISTS` rather than `ON CONFLICT` because…"). That belongs in the PR or a closed decision. Code says what it does, not what it does not do.
- **Never tell the bug's story** ("nobody noticed because the seed runs once").
- **Never restate the signature and never narrate the steps.**
- **Never describe how something used to be.** A versioned file reads as if it had always been this way.
- **When in doubt, leave it out.** A reviewer asking "why?" is cheaper than a file nobody reads.

The same bar applies to **SQL** — migrations, the reference schema, the seed — and to swaggo `@Description` blocks, where one line is the limit and changing one means regenerating `apps/api/docs/` in the same commit.

Exceptions to the end-with-a-period rule: inline comments on the same line as code, and short noun-phrase section labels (e.g. `// --- RFQ ---`).

## Verifying

`pnpm check:api` (`go build` + `go vet ./...`) must pass; golangci-lint (errcheck, govet, staticcheck, ineffassign, unused) runs in CI. See `testing` for tests, `commit` for the `type: imperative description` message, and `pr-format` / `agent-workflow` for the GitFlow branch and PR flow.
