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

## Catalog management

Administrators manage account-level products at `/settings/catalog`. The screen lists active and
inactive products with server-side search and pagination, and supports creating, replacing editable
attributes, deactivating, reactivating, and assigning a primary image. Deactivation is always soft:
historical quotes and prices keep their product reference.

The form reads the same database-backed family and subgroup taxonomy as the spreadsheet flow.
`GET /v1/product-taxonomy` exposes it to administrators, and the API still validates the selected
identifiers when a product is written.

### Product images

`POST /v1/products/{productId}/image` accepts one PNG, JPEG, or WebP image, verifies the detected
bytes against the declared content type, and replaces the product's `image_id` with a newly generated
identifier. The previous object can no longer be reached from the product. Images use the shared
object-storage port and this account-first key:

```
accounts/<account_id>/products/<product_id>/<image_id>
```

`GET /v1/public/product-images/{accountId}/{productId}/{imageId}` serves the current immutable image
inline. The random image id makes replacements cache-safe; the backoffice resolves the returned path
against `API_URL`.

### Bulk spreadsheet editing

Administrators can create and edit the catalog through a reviewed spreadsheet flow that upserts
account-level products and branch-scoped availability and prices:

1. `GET /v1/products/export` downloads a Spanish XLSX already populated with the account's coded
   products and the selected branch's current prices. It contains `Catálogo` and `Instrucciones`
   sheets, plus a hidden `Listas` sheet populated from the database-backed product taxonomy. Family,
   subgroup, and active-state cells use controlled values.
2. `POST /v1/products/import/preview` accepts `.xlsx` or `.csv`, validates every row, and
   writes nothing. The required columns are `codigo`, `nombre`, `unidad`, `familia`, and
   `activo`, and a `precio` for every new code. An existing product without a branch price may keep
   that cell empty while its catalog attributes are edited.
3. `POST /v1/products/import/confirm` revalidates the reviewed rows and atomically upserts each valid
   account-level product and its selected-branch availability. An existing code updates the product;
   a new code creates it. A changed price closes the current validity period and creates a new one,
   while an unchanged price creates no duplicate history row. Invalid rows are skipped.

`descripcion`, `subgrupo`, and `precio_minimo` are optional. `activo` accepts `SI` or `NO`; `NO`
soft-deactivates the account product and its availability in the selected branch. The service
validates that a provided subgroup belongs to the selected family. Prices use ARS and remain decimal
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
  on `product` it is the name plus the description, on `product_synonym` it is the term. Both use
  their `GIN` index. A synonym search first selects documents sharing any requested term, then
  confirms that the complete synonym occurs inside the longer description. They read the
  `spanish_unaccent` text search configuration, a copy of
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
**Every candidate that has a vector carries its distance to the line**, whichever half found it,
so a missing distance means a product not yet embedded rather than one outside the nearest few.

## Matching

Matching turns the candidates a search offered into one decision per RFQ line: which product, how
confident, and whether the seller has to look. It resolves every line of a request in a single
search, which is what keeps the whole set to one embedding call and one transaction. It asks the
search for a pool of `CATALOG_SEARCH_TOP_K × CATALOG_SEARCH_OVER_FETCH_FACTOR` candidates, scores
every one, orders them by that score and keeps the best top K: the fused rank decides what reaches
the matcher, never which candidate leads.

Its caller is the plain-text RFQ pipeline — see [rfq-pipeline.md](rfq-pipeline.md), which also
describes what a line looks like when matching cannot answer at all.

### Confidence: what the name accounts for, and how close the vector sits

A candidate's confidence, on `0..1`, is a blend of two readings that mean something on their own:

- **Coverage** — how much of the client's line the product's text accounts for. The line and the
  product's name, description and matched synonyms are tokenized the same way: accents folded,
  plural and gender endings dropped, figures split from the letters around them (`8mm`, `15x15x6`,
  `q188`), a decimal comma read as a point, a whole number joined to a fraction kept as one figure
  (`1-1/2` never meets `1/2`), and a unit bound to the figure it follows (`4mm` never meets `4L`).
  Every line token earns the best credit a product token not already spent gives it: `1` for the
  same word or the same value (`3` meets `3.00`), `0.9` for the same phonetic key (`ladriyo`,
  `sement`, `ierro`), `0.8` for one letter off on words of five or more, `0.75` for an
  abbreviation of four letters or more (`pret`, `durlo`). A figure weighs `1.5`, a word `1` and a
  lone unit `0.4`: the spec is what tells two products of one family apart, and it is exactly what
  an embedding blurs. A figure opening the line is the count (`10 bolsas de cemento`), and it is
  dropped with the packaging word after it.
- **Similarity** — cosine similarity mapped onto `0..1` between
  `CATALOG_MATCH_SIMILARITY_FLOOR_PERCENT` (what an unrelated pair of catalog texts reaches) and
  `CATALOG_MATCH_SIMILARITY_CEILING_PERCENT` (what a near-verbatim one does). Raw cosine from the
  embedding model lives in a narrow band — correct pairs measured `0.55` to `0.90` — and reading
  it as a probability is what made every correct match look weak. The band belongs to the model,
  so it is re-measured when `AI_EMBEDDINGS_MODEL` changes.

The confidence is `CATALOG_MATCH_COVERAGE_WEIGHT_PERCENT × coverage + the rest × similarity`, less
`0.05 ×` the share of the product's figures the line never asked for — and that only when the line
names a figure at all. A product with no vector yet is read on its coverage alone rather than half
of it. A seller-taught phrase (`quote_correction_memory`) leads whatever the text suggests, at
`1 − its distance`. Every figure is **rounded to four decimals before it is compared**:
`quote_item.confidence_score` is `NUMERIC(5,4)`, so the persisted number is the one the decision
was taken on.

`ts_rank` does not enter the score: it moves with term frequency and document length and means
nothing across queries. Coverage is the lexical signal that does.

### The decision

| Situation                                           | `match_status` | `product_id` | `confidence_score` |
| --------------------------------------------------- | -------------- | ------------ | ------------------ |
| No candidate at all                                 | `NO_MATCH`     | NULL         | `0.0000`           |
| Leader below `CATALOG_MATCH_MIN_CONFIDENCE_PERCENT` | `NO_MATCH`     | NULL         | the leader's       |
| Above the floor with a rival, as defined below      | `AMBIGUOUS`    | the leader   | the leader's       |
| Above the floor with no rival                       | `MATCHED`      | the leader   | the leader's       |

A **rival** is a candidate within `CATALOG_MATCH_AMBIGUITY_MARGIN_PERCENT` of the leader, or one
within three margins that covers the line as fully as the leader and carries no spec the line did
not ask for beyond the leader's. The second clause is what makes `piedra partida` against three
kinds of crushed stone a choice for the seller: the line never said which, and a vector a few
points closer is not the client choosing. The third keeps `PVC CUPLA RED 110X100`, a reducer, from
contesting `PVC CUPLA 110` for `cupla pvc 110`. Two seller-taught answers for one phrase are always
a rival.

Two parts of that are deliberate. **A rejected line keeps its best candidate's score**, because
`0.55` and `0.00` are different problems for whoever reviews the unmatched items. And **an
`AMBIGUOUS` line keeps the leading product**, so the seller confirms or replaces one proposal
rather than searching the catalog from scratch; `match_status` is what says it is unconfirmed. Only
`NO_MATCH` clears the product, which is the shape the domain asks for: **a line nothing matched is
flagged and stays in the quote, never dropped.** Choosing a product on a flagged line
(`PATCH /v1/quotes/{id}/items/{itemId}` with a `product_id`) resolves it: `MATCHED`, and no score,
since the matcher's reading described another product.

Every line comes back, in the order it went in, and the candidates ride along with it — **each one
carrying the confidence the matcher read it at**, not only the leader's. The seller picks another
from them, and the unmatched-items report shows what was considered and what each option was worth.
Which of them are persisted, and how, is in
[rfq-pipeline.md](rfq-pipeline.md#what-a-flagged-line-offers).

### The review: trade knowledge the catalog text does not carry

Some lines no text comparison settles: `placas de yeso` is a Durlock board, a green one resists
moisture, `cinta aisladora` is the insulating tape the catalog calls `CINTA AISLANTE`. The lines
the text left flagged — `AMBIGUOUS`, or `NO_MATCH` with a candidate at or above
`CATALOG_MATCH_REVIEW_FLOOR_PERCENT` — go to the bound language model, at most
`CATALOG_MATCH_REVIEW_MAX_LINES` per order, ten to a call and the calls side by side. A line whose
leader is seller-taught is not sent, and `0` turns the review off.

It is schema-forced like every call in [ai-providers.md](ai-providers.md#schema-forced-generation):
the model sees the client's
words and the candidates the matcher kept, each under a code, and the schema's enum is exactly
those codes, so it cannot name a product it was not shown. It answers every line with a one-line
reason and one verdict — `ONE` (the line names exactly this product), `SEVERAL` (these fit and the
line does not say which) or `NONE` (no candidate is what the line asks for) — and is told the
candidates are a shortlist, so "the only one offered" is not evidence. The backend then decides
what the verdict is worth:

| Verdict                                      | Becomes                                       |
| -------------------------------------------- | --------------------------------------------- |
| `ONE` on a candidate at or above the floor   | `MATCHED` on it                               |
| `ONE` between the review floor and the floor | `AMBIGUOUS`, that candidate leading           |
| `SEVERAL`                                    | `AMBIGUOUS`, the fitting candidates first     |
| `NONE`                                       | `NO_MATCH`, the candidates and the score kept |
| Anything that does not hold together         | The line exactly as the text had it           |

So **the model's knowledge alone never marks a line decided** — the catalog text has to back a
`MATCHED` on its own floor — and `NONE` only ever makes a line more cautious. The score kept is the
text's own reading of the chosen candidate. A review that fails, times out or answers for the wrong
number of lines is logged and changes nothing: matching never fails an order over it.

### What the review screen shows

Each quote line carries `confidence_level` beside the score: `HIGH` for a `MATCHED` line at or
above `CATALOG_MATCH_HIGH_CONFIDENCE_PERCENT`, `MEDIUM` for a `MATCHED` line below it or any
`AMBIGUOUS` one, `LOW` for `NO_MATCH`, and null for a line with no score — matching did not run,
or a person chose the product. The backoffice renders the level rather than cut-offs of its own,
so moving the calibration moves the screen with it.

### Calibration

Every default above is the calibration a labeled benchmark over a real 879-product corralón
catalog settled on: 154 tuning queries and 45 held-out ones, each labeled with the statuses and the
products a seller would accept. On it the text alone decides 92% of the tuning set and 93% of the
held-out one correctly, the review brings them to 94–97% and 98%, and **no configuration in the
chosen region matches a line to a wrong product with confidence** — a margin under 5 or a floor
under 55 is where those start. Move the settings against the pilot's catalog, not by feel. If
matching disappoints, the places to look are `product_synonym` and the similarity band, **not** the
embedding model.

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
| `CATALOG_MATCH_MIN_CONFIDENCE_PERCENT`     | 55      | Confidence below which a line is flagged `NO_MATCH`            |
| `CATALOG_MATCH_AMBIGUITY_MARGIN_PERCENT`   | 5       | Lead over the runner-up that makes a line `MATCHED`            |
| `CATALOG_MATCH_COVERAGE_WEIGHT_PERCENT`    | 75      | Share of the confidence coverage carries                       |
| `CATALOG_MATCH_SIMILARITY_FLOOR_PERCENT`   | 25      | Cosine similarity read as no resemblance                       |
| `CATALOG_MATCH_SIMILARITY_CEILING_PERCENT` | 90      | Cosine similarity read as a near-verbatim match                |
| `CATALOG_MATCH_HIGH_CONFIDENCE_PERCENT`    | 80      | Score a `MATCHED` line clears to read `HIGH`                   |
| `CATALOG_MATCH_REVIEW_FLOOR_PERCENT`       | 40      | Candidate score a flagged line needs to go to review           |
| `CATALOG_MATCH_REVIEW_MAX_LINES`           | 30      | Flagged lines one order sends to review; `0` turns it off      |

## API specification

All catalog handlers are annotated and appear in the generated spec. How it is generated,
served and verified: [api-specification.md](api-specification.md).
