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

Administrators can load an initial catalog through a reviewed spreadsheet flow that creates
account-level products and branch-scoped availability and prices:

1. `GET /v1/products/export` downloads a Spanish XLSX with `Catálogo` and `Instrucciones`
   sheets, plus a hidden `Listas` sheet populated from the database-backed product taxonomy.
   Family and subgroup cells use dropdowns sourced from that hidden sheet.
2. `POST /v1/products/import/preview` accepts `.xlsx` or `.csv`, validates every row, and
   writes nothing. The required columns are `codigo`, `nombre`, `unidad`, `familia`, and
   `precio`.
3. `POST /v1/products/import/confirm` revalidates the reviewed rows and atomically creates
   each valid account-level product, its active availability at the selected branch, and
   its first branch price. Invalid or already-existing codes are reported and skipped.

`descripcion`, `subgrupo`, and `precio_minimo` are optional. The service validates that a
provided subgroup belongs to the selected family. Initial prices use ARS and remain decimal
strings throughout the HTTP contract; currency and price conditions are not spreadsheet
columns. Every route requires an administrator and an active `X-Branch-Id`; the account
always comes from the authenticated tenant.

Catalog declares its columns and workbook sheets on the shared contract documented in
[Shared spreadsheet layer](spreadsheets.md); it does not own CSV, XLSX, ZIP, or cell serialization.

## Hybrid search

Product matching resolves an RFQ line against the catalog through two halves at once, and both
are needed: the semantic half generalizes past wording the catalog never used, the lexical half
carries the exact trade vocabulary that a vector model has no way to know.

- **The lexical half** is Postgres full-text search over a `search_document` generated column —
  on `product` it is the name plus the description, on `product_synonym` it is the term. Both
  are `GIN` indexed. They read the `spanish_unaccent` text search configuration, a copy of
  `spanish` with `unaccent` in front of the stemmer: informal request text drops accents
  constantly, and under the stock configuration "hormigon" would never reach "hormigón".
- **The semantic half** orders `product.embedding` by cosine distance (`<=>`).
- **Both halves are account-scoped in the query**, and the result is joined against
  `branch_product` so a search can only ever offer what the active branch carries. A search with
  no active branch is refused rather than answered account-wide.

**Recognition quality is not a model-choice problem.** Trade terms — a `telagoma` for a membrane,
a `pastina` for a grout — are what the synonym table and the lexical half are for. Reaching for a
larger embedding model instead solves nothing, and the escape hatch if recognition really does
disappoint is in [ai-providers.md](ai-providers.md): a wider model can be truncated back to 1536
dimensions and the catalog re-embedded into the same column, with no migration.

### Merging the two halves, and the trim

Each half is ranked on its own and the two are merged by **reciprocal rank fusion**: a candidate
contributes `1 / (CATALOG_SEARCH_RRF_K + its rank)` from every half that found it. Ranks are what
make the halves comparable at all — a cosine distance and a `ts_rank` share no scale — and a
candidate both halves found therefore outranks one only a single half saw.

**The service asks the database for more rows than the caller wants and trims the result.** An
approximate vector scan orders before the branch filter runs, so a request for twenty candidates
can come back with six once the products the branch does not stock are dropped. The first fetch
is `top K × CATALOG_SEARCH_OVER_FETCH_FACTOR`, and it widens until the limit is met, a wider fetch
stops returning anything new — which is what a branch carrying fewer than K matches looks like — or
`CATALOG_SEARCH_MAX_FETCH` is reached. **An empty round is not a stopping condition**: the nearest
vectors in the account can all be stock this branch does not carry, which is precisely the case
widening exists for. Asking for K usable candidates therefore returns K whenever the branch has
them within the ceiling.

The search returns candidates and their evidence, and decides nothing: which of them counts as a
match, which line is `AMBIGUOUS`, and which is flagged `NO_MATCH` belongs to the matching service.

## Matching

Matching turns the candidates a search offered into one decision per RFQ line: which product, how
confident, and whether the seller has to look. It resolves every line of a request in a single
search, which is what keeps the whole set to one embedding call and one transaction.

### The fused score is a ranking, not a confidence

Reciprocal rank fusion answers "which candidate first", and its figure maxes at
`2 / (CATALOG_SEARCH_RRF_K + 1)` — about `0.033` at the default. Persisting it would put every
line under any threshold worth setting. Confidence is derived instead from figures that mean
something on their own scale:

- **Cosine similarity**, `1 - distance`, clamped to `0..1`. A candidate the vector half never
  scored — a synonym hit on a product carrying no embedding — has no similarity to read and takes
  `CATALOG_MATCH_LEXICAL_CONFIDENCE_PERCENT` instead. It has to sit above the floor, or a trade
  term loaded as a synonym could never resolve to its product.
- **The margin** over the runner-up, on the same scale. This is what separates a decided line from
  a choice: two cements at `0.91` and `0.90` are not a confident match.

Two consequences of that shape are deliberate rather than oversights. **`ts_rank` never enters the
score**, because it is not comparable across queries — it moves with term frequency and document
length, so a flat configured worth is more honest than a number that looks precise and is not. Which
means two candidates reached only by the lexical half tie at exactly that worth, and the line comes
back `AMBIGUOUS` however much better one text match was. And **a candidate both halves found scores
no higher than one the vector half found alone at the same distance**: the agreement between the
halves already decided which candidate leads, and counting it again in the confidence would count it
twice. Confidence measures the winner; the ranking measures the agreement.

The leading candidate is the one the **search** ranked first, never a re-ranking. Matching decides
status; ranking is the search's, and the margin can therefore come out **negative** when the two
halves disagree about which product a line is — which is an ambiguous line, and needs no special
case.

Every figure is carried as a decimal and **rounded to four decimals before it is compared**, not
on the way to the database. `quote_item.confidence_score` is `NUMERIC(5,4)`, so the persisted
number is then exactly the one the decision was taken on.

### The decision

| Situation                                                                        | `match_status` | `product_id` | `confidence_score` |
| -------------------------------------------------------------------------------- | -------------- | ------------ | ------------------ |
| No candidate at all                                                              | `NO_MATCH`     | NULL         | `0.0000`           |
| Leader below `CATALOG_MATCH_MIN_CONFIDENCE_PERCENT`                              | `NO_MATCH`     | NULL         | the leader's       |
| Above the floor, margin at or above the ambiguity margin (or a single candidate) | `MATCHED`      | the leader   | the leader's       |
| Above the floor, margin below it                                                 | `AMBIGUOUS`    | the leader   | the leader's       |

Two parts of that are deliberate. **A rejected line keeps its best candidate's score**, because
`0.55` and `0.00` are different problems for whoever reviews the unmatched items. And **an
`AMBIGUOUS` line keeps the leading product**, so the seller confirms or replaces one proposal
rather than searching the catalog from scratch; `match_status` is what says it is unconfirmed. Only
`NO_MATCH` clears the product, which is the shape the domain asks for: **a line nothing matched is
flagged and stays in the quote, never dropped.**

Every line comes back, in the order it went in, and the candidates ride along with it — the seller
picks another from them, and the unmatched-items report shows what was considered.

### Calibration

The three settings are the whole knob, and they exist to be moved against a real catalog rather
than guessed here. If matching disappoints, the place to look is `product_synonym` and the relative
weight of the two halves — **not** the embedding model.

`CATALOG_SEARCH_TOP_K` is bound to this: below two there is no runner-up, so every line above the
floor would read as decided and `AMBIGUOUS` could never happen. Configuration refuses it at boot
rather than letting the quality drop silently.

## Embedding the catalog

Vectors are written by a command, never by a request:

```bash
go run ./cmd/catalog-embed --account <uuid> [--refresh-all]   # from apps/api
pnpm db:vector-index [--lists <n>]                            # from the repo root
```

`catalog-embed` opens the restricted pool alone, so the backfill cannot reach past the account it
was given. It refuses an account that does not exist — under row level security a mistyped id
otherwise reads as a catalog with nothing left to embed. It pages through the account's catalog by
product id, embedding each page outside
any transaction and writing it back in a short one. **It is a command because the work does not
fit a request:** a catalog is thousands of texts, and the AI timeouts are per attempt rather than
per chain, so one page can outlast any HTTP response budget. A run that fails halfway keeps the
pages before it, and a re-run resumes.

By default it takes only what needs it — no vector, or `embedding_updated_at` older than the
row's `updated_at`, which is how an edited product comes back around. `--refresh-all` re-embeds
everything, which is what a change of embedding model needs. It requires
`AI_EMBEDDINGS_PROVIDER=openai` and a key, and refuses up front without them.

**The vector index is created afterwards, and deliberately not by a migration.** Built on an
empty table an approximate index is degenerate — it has no data to partition, and it does not
improve later on its own. `pnpm db:vector-index` builds it once the catalog is embedded, with
`lists` sized to the rows that carry a vector (pgvector's own guidance: `rows/1000`, or
`sqrt(rows)` past a million), and `--lists` overrides that. The drop and the build run in one
transaction, so a build that is interrupted or runs out of memory leaves the working index in place
rather than none. It runs as the owner role and rebuilds
the index from scratch, so it is the command to re-run after the catalog grows an order of
magnitude. The build holds a write lock on `product` for its duration.

`CATALOG_SEARCH_IVFFLAT_PROBES` is the query-side companion: the database visits one partition
per scan by default, which recalls too little of the catalog to survive the branch filter.

## Configuration

| Variable                                   | Default | What for                                                       |
| ------------------------------------------ | ------- | -------------------------------------------------------------- |
| `CATALOG_DEFAULT_PAGE_SIZE`                | 50      | Page size when `limit` is omitted                              |
| `CATALOG_MAX_PAGE_SIZE`                    | 200     | Cap on `limit`, so nobody asks for everything                  |
| `CATALOG_IMPORT_MAX_BYTES`                 | 5242880 | Maximum catalog spreadsheet upload size                        |
| `CATALOG_SEARCH_TOP_K`                     | 10      | Candidates per line when the caller names no limit; at least 2 |
| `CATALOG_SEARCH_OVER_FETCH_FACTOR`         | 4       | Multiplier on the rows each half is asked for                  |
| `CATALOG_SEARCH_MAX_FETCH`                 | 2000    | Widest one round of the widening may ask for                   |
| `CATALOG_SEARCH_IVFFLAT_PROBES`            | 10      | Index partitions one approximate scan visits                   |
| `CATALOG_SEARCH_RRF_K`                     | 60      | Constant in the rank fusion merging the two halves             |
| `CATALOG_EMBEDDING_BATCH_SIZE`             | 200     | Products the backfill reads and writes per round               |
| `CATALOG_MATCH_MIN_CONFIDENCE_PERCENT`     | 60      | Similarity below which a line is flagged `NO_MATCH`            |
| `CATALOG_MATCH_AMBIGUITY_MARGIN_PERCENT`   | 5       | Lead over the runner-up that makes a line `MATCHED`            |
| `CATALOG_MATCH_LEXICAL_CONFIDENCE_PERCENT` | 75      | Worth of a candidate only the lexical half scored              |

## API specification

All catalog handlers are annotated and appear in the generated spec. How it is generated,
served and verified: [api-specification.md](api-specification.md).
