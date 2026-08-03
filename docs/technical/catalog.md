# Catalog

The catalog belongs to the **account**: `product`, `product_synonym` and
`product_alternative` all hang off `account_id`. One product is one row per account, with one
embedding and one set of synonyms and alternatives. What varies per branch — availability,
stock and price — lives in `branch_product` and `product_price`.

That split cuts the routes in two: the account-level ones ignore `X-Branch-Id`, and the
per-branch ones need it to write.

## Endpoints

Account level:

| Method   | Route                                                   | What it does                             |
| -------- | ------------------------------------------------------- | ---------------------------------------- |
| `GET`    | `/v1/products`                                          | Paginated list, with search and category |
| `POST`   | `/v1/products`                                          | Create                                   |
| `GET`    | `/v1/products/{productId}`                              | One product                              |
| `PUT`    | `/v1/products/{productId}`                              | Replace                                  |
| `DELETE` | `/v1/products/{productId}`                              | Soft delete                              |
| `GET`    | `/v1/products/{productId}/synonyms`                     | The product's synonyms                   |
| `POST`   | `/v1/products/{productId}/synonyms`                     | Add a synonym                            |
| `DELETE` | `/v1/products/{productId}/synonyms/{synonymId}`         | Remove a synonym                         |
| `GET`    | `/v1/products/{productId}/alternatives`                 | Alternatives, in the requested direction |
| `POST`   | `/v1/products/{productId}/alternatives`                 | Define an alternative                    |
| `DELETE` | `/v1/products/{productId}/alternatives/{alternativeId}` | Remove an alternative                    |

Per branch:

| Method | Route                                   | What it does                          |
| ------ | --------------------------------------- | ------------------------------------- |
| `GET`  | `/v1/products/{productId}/availability` | Where it is sold, and with what stock |
| `PUT`  | `/v1/products/{productId}/availability` | Set availability and stock            |
| `GET`  | `/v1/products/{productId}/prices`       | Validity-period history               |
| `POST` | `/v1/products/{productId}/prices`       | Put a price in force                  |

All of them require a session (`RequireTenant`).

## One code per account, and why blank becomes NULL

`uq_product_account_code` is a **partial** unique index over `(account_id, code)` where
`code IS NOT NULL`. A repeated code inside the account returns **409**; the same string in
another account is fine.

The service normalizes: it trims whitespace and **turns blank into NULL**. That is necessary,
not cosmetic — two products with code `''` would collide against the index, while two
carrying `NULL` are exactly what an unnumbered catalog looks like.

## Replace and soft delete

`PUT` **replaces** the editable attributes: an omitted nullable field becomes NULL, so the
caller sends the product as it should end up. `is_active` is the exception — omitted leaves
it alone, so an edit form cannot accidentally revive a deactivated product. Reactivating is
explicit: `"is_active": true`.

`DELETE` is a **soft delete** (`is_active = FALSE`). The row survives because closed quote
items and the price history reference it; deleting it would rewrite history. Repeating the
call does not fail. The listing hides inactive items unless `include_inactive=true`.

## Synonyms

Trade vocabulary that improves lexical matching.

`source` records where the term came from, and is the native enum `product_synonym_source`:

| Value      | Who writes it                                       |
| ---------- | --------------------------------------------------- |
| `MANUAL`   | A person, from the backoffice. This is the default. |
| `LEARNED`  | The matching pipeline, proposing from real requests |
| `IMPORTED` | The bulk catalog import, and the seed               |

**The endpoint accepts only `MANUAL` and `LEARNED`.** `IMPORTED` is written by the bulk
import, which has its own path and no reason to be reachable from a request body. The
domain's set and a route's accepted set need not match.

A repeated term on the same product returns **409**, enforced by `uq_product_synonym_term`,
unique over `(account_id, product_id, lower(term))` — case-insensitive, because "Portland"
and "portland" are the same term to a matcher. The insert uses it as its `ON CONFLICT`
target, so two simultaneous requests carrying the same term cannot both pass.

## Alternatives

`product_alternative` links a base product to another that can stand in for it, typed
`EQUIVALENT`, `PREMIUM` or `ECONOMY`. `uq_product_alternative` allows one link per ordered
pair: repeating it returns **409**, and changing the type means deleting and recreating.

**Direction is a parameter, not two implementations.** `direction` picks which end of the
relation to read:

- `OUTGOING` (default) — what can be offered instead of this product. What the recommendation
  engine asks for.
- `INCOMING` — which products this one is an alternative to. What the upsell path asks for.

A product cannot be its own alternative (**422**). The link is deletable from either end, and
the route's `productId` has to be one of the two: a link between two other products is not
deletable through a third one.

## The check the database does not do

Before writing a synonym or an alternative, the service **reads the product inside the tenant
scope**. That is not redundant:

> Referential-integrity checks — foreign keys, uniqueness — **bypass row level security**.

So a foreign key to `product` happily accepts another account's product id, leaving a synonym
with our own `account_id` hanging off an invisible product. The policy only looks at the
`account_id` of the row being inserted, which is ours. The preceding `SELECT` is the only
thing that closes that hole, and tests pin it — one of them asserts the foreign key really
does allow it, so the day that changes, the test says so.

## Per-branch availability

`branch_product` says whether the branch sells the product and with how much stock. `PUT`
upserts against `uq_branch_product`, the schema's own `(branch_id, product_id)` uniqueness,
so the caller does not have to know whether it is the first time.

`stock` is a decimal string, and **absent is not zero**: NULL means the branch does not track
stock for that item, zero means it has none left. An unspecified `is_active` is `true`,
because the point of the call is normally that the branch does sell it; setting it to `false`
is how a branch stops offering something the account still catalogs.

## Per-branch prices, versioned by validity

**A price is never overwritten.** Setting a price opens a new validity period and closes the
previous one at the same instant: the old row's `valid_to` equals the new one's `valid_from`,
and both writes go in **one transaction** serialized on the product row, so neither a crash
nor two concurrent repricings can leave the product with two open periods or none.

That is what keeps a quote frozen last month explainable: the price that applied then is
still in the table.

- An unspecified `valid_from` means now.
- A `valid_from` **before** the open period's start is rejected (**422**): it would close a
  period before it opened, and rewrite which price applied at a moment already quoted.
- The first price for a product at a branch closes nothing.
- An unspecified `currency` is `ARS`.
- `min_price` is the **discount engine's floor**, so it cannot exceed the price it floors
  (**422**).
- `GET` returns the full history, closed periods included, grouped by branch and newest
  first.

Both writes need an active branch: without `X-Branch-Id` there is no correct target, and
guessing one would price the wrong branch (**422**, not a silent default). Reads do work
without the header, and then return every branch of the account — which is how an admin
compares them.

## Money travels as a decimal string

`price`, `min_price` and `stock` are `NUMERIC(14,2)` in the database, `decimal.Decimal` in
Go, and **decimal strings** in the JSON. Never floats: a JSON number would lose precision on
the round trip. The pgx codec is registered per connection in `AfterConnect`, because the
pool opens connections whenever it wants, replacements for dead ones included.

The service rejects what the column cannot store exactly:

- more than two decimals — Postgres would round the third away without saying so, and on
  money that is a defect, not a rounding preference;
- more than 12 integer digits — one extra typed digit has to be an actionable message, not a
  500;
- negatives.

## Pagination

`limit` and `offset`. Without `limit`, `CATALOG_DEFAULT_PAGE_SIZE`; above the cap,
`CATALOG_MAX_PAGE_SIZE`. `total` counts **every** row matching the filter, not the ones on
the page, and comes from a `count(*) OVER ()` in the same query: one round trip, and the
total cannot contradict the page it describes.

## Initial catalog import

Administrators can load an initial catalog through a reviewed, branch-scoped spreadsheet
flow:

1. `GET /v1/products/export` downloads a Spanish XLSX with a `Catálogo` sheet and
   a second `Instrucciones` sheet.
2. `POST /v1/products/import/preview` accepts `.xlsx` or `.csv`, validates every row, and
   writes nothing. The required columns are `codigo`, `descripcion`, `unidad`, and `precio`.
3. `POST /v1/products/import/confirm` revalidates the reviewed rows and atomically creates
   each valid account-level product, its active availability at the selected branch, and
   its first branch price. Invalid or already-existing codes are reported and skipped.

`nombre`, `categoria`, `precio_minimo`, `moneda`, and `condiciones` are optional. When
`nombre` is empty, `descripcion` becomes the canonical product name. Prices remain decimal
strings throughout the HTTP contract. Every route requires an administrator and an active
`X-Branch-Id`; the account always comes from the authenticated tenant.

## Configuration

| Variable                    | Default | What for                                      |
| --------------------------- | ------- | --------------------------------------------- |
| `CATALOG_DEFAULT_PAGE_SIZE` | 50      | Page size when `limit` is omitted             |
| `CATALOG_MAX_PAGE_SIZE`     | 200     | Cap on `limit`, so nobody asks for everything |
| `CATALOG_IMPORT_MAX_BYTES`  | 5242880 | Maximum catalog spreadsheet upload size       |

## API specification

All catalog handlers are annotated and appear in the generated spec. How it is generated,
served and verified: [api-specification.md](api-specification.md).
