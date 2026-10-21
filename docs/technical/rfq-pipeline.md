# The plain-text RFQ pipeline

What happens between a client's informal message and a quote a seller can review. One text goes in;
one `quote` in `DRAFT`, its first unfrozen `quote_version`, and one `quote_item` per material come
out, each line carrying the product it matched, how confident that is, why its quantity is what it
is, and — when the line needs the seller's eye — the candidates it was decided against.

That draft carries no prices. The seller accepting its materials is a second transition, and it is
what freezes them — the last section here.

The AI provider layer is in [ai-providers.md](ai-providers.md), the search and matching behind the
product decision in [catalog.md](catalog.md). This document is the flow that consumes both. The
files an order arrives with are stored beside it and are not read here — see
[file-storage.md](file-storage.md).

## Endpoints

| Method   | Path                               | What it does                                                           |
| -------- | ---------------------------------- | ---------------------------------------------------------------------- |
| `POST`   | `/v1/rfqs/text-drafts`             | Runs an order the seller pasted or typed through the pipeline          |
| `POST`   | `/v1/rfqs/file-drafts`             | Runs an order that arrived as a file through the same pipeline         |
| `GET`    | `/v1/rfqs`                         | The dashboard queue; `?include_archived=true` keeps archived quotes in |
| `GET`    | `/v1/rfqs/{id}`                    | Returns the detail with quote state history and delivery tracking      |
| `GET`    | `/v1/channels`                     | The active intake channels of the selected branch                      |
| `POST`   | `/v1/dev/whatsapp/messages`        | Simulates one inbound WhatsApp message. Not registered in production   |
| `POST`   | `/v1/quotes/{id}/accept-materials` | Prices the draft's lines and moves the quote to `QUOTED`               |
| `POST`   | `/v1/quotes/{id}/transition`       | Records an offline acceptance or rejection reported by the seller      |
| `POST`   | `/v1/quotes/{id}/reactivate`       | Reopens an accepted or rejected quote for resend or editing            |
| `POST`   | `/v1/quotes/{id}/payment`          | Marks an accepted sale paid («Cobrado»), with who and when             |
| `DELETE` | `/v1/quotes/{id}/payment`          | Takes the paid mark back                                               |
| `POST`   | `/v1/quotes/{id}/invoice-waiver`   | Marks an accepted sale as needing no invoice («No requiere factura»)   |
| `DELETE` | `/v1/quotes/{id}/invoice-waiver`   | Puts the sale back to waiting for its invoice                          |
| `GET`    | `/v1/quotes/awaiting-invoice`      | One page of the accepted sales still waiting for an invoice            |

The three that reach a model share **their own rate-limit allowance**, `RATE_LIMIT_AI_MAX` — the global one would let a single seller spend 300 generations a minute, and this is the first surface in the product billed per call. Valorization reaches no provider and spends nothing, so it stays on the global allowance.

The authenticated endpoints are branch-scoped and read the branch from `X-Branch-Id`. `channel_id` is required on a
draft, which is why the channel listing exists: `rfq.channel_id` is `NOT NULL`, and a caller
has to name the route the order arrived through rather than have one guessed for it. How a channel
is configured, and where its provider credentials live, is in
[accounts-and-branches.md](accounts-and-branches.md#channels).

## An order that arrived as a file

`POST /v1/rfqs/file-drafts` is a multipart request: the file on the `file` part, and the same
envelope the text route takes beside it (`channel_id`, `client_id`, `client_label`, `work_type`,
plus an optional `note` the seller adds for the model). It answers with the same reviewable draft.

What the format decides is only **how the order reaches the model**:

| Kind                                       | How it is read                                |
| ------------------------------------------ | --------------------------------------------- |
| Image (`jpeg`, `png`, `webp`, `heic`)      | Straight to the model as an image block       |
| PDF                                        | Straight to the model as a document block     |
| Spreadsheet (`xlsx`, `csv`)                | Flattened to tab-separated rows, then as text |
| Audio (`mp3`, `m4a`, `ogg`, `wav`, `webm`) | Transcribed, then as text                     |
| Plain text                                 | As text                                       |

A photo and a PDF go to the model **as they are**, because a materials list is laid out and
flattening it first throws away the columns the model reads it by. A recording and a spreadsheet
have no layout a model can use, so they become text first — the transcriber and the spreadsheet
reader respectively. The accepted set is `domain.AttachmentFormatFor`, the same one the attachment
upload enforces, so a file the pipeline would refuse cannot be stored either.

A spreadsheet's parser is chosen by its **bytes**, never its name or type: a ZIP signature is read
as `.xlsx`, anything else as CSV. That matters because Windows labels a `.csv` with the legacy
Excel type `application/vnd.ms-excel`, which is accepted and stored as `.csv`. A real legacy `.xls`
workbook (the OLE2 signature) is refused on upload with `LEGACY_EXCEL_FILE`, so the seller is asked
to save it as `.xlsx`; the Go readers evaluated for it failed on real Excel files and grew without
bound on malformed ones.

The order is kept before it is read, and the file before the model runs: a provider outage then
leaves the seller the order the client actually sent, on an RFQ they can work by hand. The
attachment row keeps whatever text the file yielded in `extracted_text` and closes at `DONE`; an
image and a PDF yield none, so the RFQ's `raw_text` names the file instead of inventing content
for it.

`RFQ_MAX_ITEMS` bounds what the model may return; a spreadsheet is bounded on the way in too, by
`RFQ_MAX_SPREADSHEET_ROWS` (default 500), so a price list uploaded by mistake is refused before it
is paid for. Both doors into the engine apply that limit over the same reader, so a sheet is an
order or a catalog by its own size and never by whether it arrived inline or through the sweep.
The file's own size is capped by `STORAGE_MAX_FILE_SIZE_BYTES`, the same limit the attachment
upload uses.

The development route resolves the branch's WhatsApp channel and then calls the same pipeline as a
production connector. It is a different way in, not a second pipeline — a copy would drift from the
path it is meant to rehearse. Production connector requirements are documented in
[accounts-and-branches.md](accounts-and-branches.md#connector-integration-contract). The development
route is registered only when `ENV` is not `production`.

## The order of operations, and why

```
store the order  →  extract  →  match  →  store the draft
   (transaction)    (provider)  (provider + its own transaction)   (transaction)
```

**The order is stored before anything reads it.** A model that fails, refuses or times out then
leaves a recoverable `rfq` instead of losing what the client wrote; the system never discards the
original input. That is why this is two transactions rather than one.

**Neither provider call happens inside a transaction.** Both are slow and fail on their own
timeline, and a `pgx.Tx` held open across one holds a pool connection with it.

**The draft is written as a single unit** — the `quote`, its `quote_version` number 1, its
`quote_item` rows, the current-version pointer, and the two status histories. A half-written
transition would leave an order whose quote exists with no lines, and nothing downstream could tell
that from an order for nothing.

**The writes do not run under the pipeline's deadline.** The deadline below bounds the two provider
calls; the persist that follows runs on the request's own context. Sharing one deadline would throw
away an extraction that was already paid for, at the last step.

## Quantities: the schema's escape value

The extraction schema forces a closed enum, `quantity_source`, on every line:

| Value        | Meaning                                                                                   |
| ------------ | ----------------------------------------------------------------------------------------- |
| `EXPLICIT`   | The client stated the number                                                              |
| `DERIVED`    | It follows from what they wrote — three pallets of fifty bags is a hundred and fifty bags |
| `UNRESOLVED` | The message carries no number that could be defended to a seller                          |

`UNRESOLVED` is the escape value, and it is the whole point of the enum: without a valid way to say
"I cannot tell how many", a model asked for a number produces one. A line that comes back
`UNRESOLVED` is **still written to the quote**, with `quantity` at zero and
`quote_item.quantity_rationale` saying which datum is missing. The material is what the client asked
for, and dropping the line to avoid an awkward zero loses it.

The service does not trust the pair. `EXPLICIT` and `DERIVED` must carry a positive quantity, and
`UNRESOLVED` must carry zero — a line that contradicts itself is refused, and the adapter zeroes an
unresolved quantity before the service ever sees it. So a number the model was told not to send
cannot reach a quote through either layer.

**Quantities are never inferred from a plan.** `rfq.work_type` is context for the seller's
recommendations, not an input: computing bags of cement from a surface in square metres is a
materials calculation, and that is outside the product.

### `quantity_rationale` is required on every line

Not only on the derived ones. The seller has to understand the number without reopening the
message — that is the same need the input-beside-the-interpretation view serves — so an explicit
quantity says so ("el cliente pidió 10 bolsas") and an unresolved one says what is missing. It is
model-written text a seller reads, so it is written in Argentine Spanish; everything else in the
schema is English.

## An order that names no material

The extractor reads nothing a supplier sells — the message was a greeting, or a question about
opening hours. Then **no quote is created and the `rfq` moves to `FAILED`**, and the route answers
`201` with the order alone: the text was stored, which is the part that matters, and the seller
works it by hand. A later run would read the same nothing, so there is nothing to wait for.

`GENERATED` means the engine produced materials, so an order that produced none has not reached
it, and a `quote` with no lines would be indistinguishable from an order for nothing. The response
carries `quote` and `version` as `null` for exactly this case.

## What a failed match does

Matching is asked for a decision per line and may not be able to give one — the embedding provider
is `disabled` or down, so no line can be embedded to search with. **Every line then stays
`NO_MATCH` with no product and a null `confidence_score`, and the draft is still written.** Losing
an extraction over a match would discard what the client asked for, and a flagged line is the state
the seller resolves anyway.

A null score is deliberately not a zero. Three states are distinguishable, and the unmatched-items
report needs them apart:

| `confidence_score` | Meaning                                       |
| ------------------ | --------------------------------------------- |
| `NULL`             | Nothing scored this line — matching never ran |
| `0.0000`           | Matching ran and no candidate came close      |
| `0.5500`           | Matching ran and rejected a near miss         |

The same holds for a set of decisions whose length does not match the lines: pairing a line with
another line's product is a wrong match nothing downstream could notice, so the whole set is
discarded rather than indexed into.

## What a flagged line offers

A `match_status` and a number tell the seller a line needs their eye. They do not tell them what to
do about it. So the candidates the matcher weighed are kept, as `quote_item_alternative` rows with
`origin = 'AI'` and `type = 'PRODUCT'`, and they come back attached to the line on both the draft
and the priced response.

| Line status | What it offers                        | Why                                                             |
| ----------- | ------------------------------------- | --------------------------------------------------------------- |
| `MATCHED`   | Nothing, unless the review decided it | The text decided it; a line the review settled keeps its offers |
| `AMBIGUOUS` | Every candidate but the one it kept   | It kept the leader, so the offers are the products it might be  |
| `NO_MATCH`  | Every candidate                       | It points at nothing, so the closest near miss is the first one |

**A candidate that scored zero is dropped from both**, which is the one exception to that table.

**`rank` is the candidate's place in the matcher's ranking, not a renumbering.** An `AMBIGUOUS`
line's offers therefore start at two: rank one is the product on the line. The ranking is the
matcher's own order by confidence — or the review's, when it settled the line — so the dropped zeros
are always its tail. `confidence_score` is what the candidate scored, on
`quote_item.confidence_score`'s own scale, so a 59% near miss reads differently from a 12% long
shot. Neither figure exists anywhere else on the row — `created_at` is the transaction's timestamp,
shared by every row of one insert — which is why the table carries both columns.

A zero means the product shares no word with the line and its vector sits under the calibration
band: the search reached it only because the pool is wider than what resembles the line. Offering
those would bury the near miss under everything the account sells.

**`price_snapshot` stays empty.** Nothing is priced when matching runs, and the price a seller would
freeze is the one in force when they choose. Freezing prices belongs to valorization.

**The catalog identity is joined, not frozen.** `code`, `canonical_name` and `unit` are read from
`product` as it stands, the same as the product the line itself matched — a bare id would be barely
better than a bare flag.

**Each line names its own candidates before either is written.** The service chooses the
`quote_item.id`, so a candidate references the line directly. Deriving the pairing from the insert's
row order would rest on an order Postgres does not promise, and a line offering another line's
products is a wrong answer nothing downstream could notice. `CreateItems` refuses a line that
carries no id: left unset, every line would insert the all-zeros uuid.

The candidates are written in the same transaction as the lines, so a draft never exists with lines
flagged and the candidates they were flagged against missing.

## The marks belong to the version

`quote_item` rows belong to one `quote_version`, and `quote_item_alternative` rows belong to one
`quote_item`. A later version therefore has lines of its own, and resolving a flagged line in it
leaves the earlier version's marks and candidates exactly as they were. That is what makes the
report auditable, and it is also the metric: how many lines arrived unmatched in version one, and
how many had to be fixed by hand. `uq_quote_version_draft` allows one unfrozen version per quote,
so a second version means the first was frozen.

## Settling a flagged line

Accepting the materials keeps a line the catalog left `AMBIGUOUS` or `NO_MATCH` — it is flagged,
never dropped — but a quote is only sent with every line on a confirmed product with a price; the
send otherwise answers `422 QUOTE_ITEMS_UNRESOLVED`. The seller settles a line on any editable quote,
priced ones included: `PATCH /v1/quotes/:quoteId/items/:itemId` with `confirm_match: true` marks the
product the line carries `MATCHED` and keeps its price, and choosing another product on a `QUOTED`
quote prices it at the branch's current price.

## The original AI proposal is preserved separately

The first commercial version stays editable because it is the seller's working draft. That makes
it unsuitable as the baseline for measuring whether the proposal was correct: editing it would
erase the evidence being compared.

Every generated draft therefore writes an append-only `quote_ai_generation` in the same
transaction as version 1. It records the provider, model, prompt version, forced-schema version,
and token usage. Its `quote_ai_generation_item` rows copy the proposed description, quantity,
unit, quantity source and rationale, selected product, match status, and score in client order.
They do not reference the live `quote_item` row, so later item replacement or removal cannot alter
or invalidate the baseline.

The request role can only `SELECT` and `INSERT` these tables. It cannot update or delete them. A
draft is not considered generated if this evidence cannot be written: the proposal, its editable
version, and both status histories share one transaction.

The comparison is not exposed as a confidence percentage. Relevant seller changes become
account-local correction memories: item additions, removals, quantity and unit changes teach
future extraction; corrected product selections teach catalog matching. Pricing, discounts,
comments, and descriptions do not become learning evidence.

Each memory starts as `PENDING` so the seller's correction is durable before an embedding provider
is called. Successful vectorization changes it to `READY`; the `quote-correction-learning`
scheduled job retries pending rows. Sending remains complete even when learning is temporarily
unavailable. Retrieval is local to the account, uses a fixed 80 percent cosine-similarity floor,
and retains at most 1,000 patterns per account with automatic low-value eviction.
An eligible catalog memory outranks generic lexical and product-vector search, while the branch
availability filter still applies. Two eligible memories pointing to different products produce
`AMBIGUOUS`; seller evidence is never resolved by an arbitrary tie-break.

### Integration contract for the future send flow

`QuoteQualityService.EvaluateFinalQuote` is the internal hook that closes one outcome. The future
send service constructs it with `repository.NewQuoteQualityRepository()`, enables
`WithCorrectionLearning` using `QuoteCorrectionService`, and calls it after its
send transaction has committed the version with `is_immutable = true` and a `quote_send` carrying
`sent_at` plus a successful tracking state. It must pass the quote id and the exact version id the
seller sent. The hook is idempotent, and the durable send row lets a failed attempt be retried even
after the quote advances beyond `SENT`.

There is deliberately no route or simulated delivery for this hook. Until sending exists, only
the integration suite invokes it by staging the frozen version, status transition, and durable
send record.

The `whole-quote-v1` evaluator produces one strict binary label. It requires the same billable
items — product, quantity, and unit, independent of order — every final line resolved to
`MATCHED`, every line priced, each subtotal equal to quantity times unit price, and the version
total equal to subtotals less unsuppressed discounts. Description and rationale edits are
editorial and do not change the label. Every failed condition is appended to
`quote_quality_difference`; changing these rules requires a new evaluator version rather than
rewriting old labels.

## Accepting the materials is what prices the quote

The draft the pipeline above produces carries no prices, and that is deliberate. Two transitions,
not one:

| Transition               | What happens                                                                                    |
| ------------------------ | ----------------------------------------------------------------------------------------------- |
| `RECEIVED` → `GENERATED` | The quote is born at `DRAFT` with version 1 and one line per material. No prices                |
| `DRAFT` → `QUOTED`       | The seller accepts the materials; each line freezes its price and floor and the total is summed |

The route is named for the seller's action rather than the calculation behind it, because the state
machine's trigger for this transition is "Aceptar materiales" — the pricing is what that causes,
not what the seller asks for.

**Each line freezes two values, and freezes them together.** `quote_item.unit_price_snapshot` is
the price in force at the quote's branch; `quote_item.min_price_snapshot` is the floor a discount
may not cross. They are captured as a pair because the discount evaluator reads only frozen values:
re-evaluating one version with the same lines always gives the same total, whatever the account has
done to its price list since. `quote_item.subtotal` is quantity × the frozen unit price, rounded to
two decimals.

**A null floor means no floor, never a floor of zero.** `product_price.min_price` is nullable and
most accounts never set one. Read as zero, a later discount could take the line to nothing.

**`quote_version.total` is Σ subtotals − Σ discounts.** Accepting the materials also runs the
discount sweep, which earns the promotions the priced lines qualify for; see
[discount-engine.md](discount-engine.md).

### The two lines that stay unvalued

Both keep all three values null, stay in the quote, and add nothing to the total:

- **A line with no product** (`match_status = NO_MATCH`) has nothing to price. Dropping it and
  pricing it at zero are both ways of misreporting what the client asked for.
- **A line whose product the branch cannot price** — never priced there, the last period ended, the
  product was deactivated, or the branch no longer carries it. One such product does not block the
  seller from quoting the other nineteen lines, so the transition goes through and the service logs
  a warning naming the products. A quote can therefore reach `QUOTED` with a gap in it, so the
  response names those lines: **`pricing_unavailable`**.

`pricing_unavailable` is a signal of its own rather than a fourth `match_status`, because it answers
a different question. The catalog decided such a line correctly — it stays `MATCHED` — and a line
can be matched and unpriceable at once, which one column cannot hold. It is **null until the
materials are accepted**: on a draft nothing has been valued, and `false` there would say the line
is fine when nobody has looked. A line with no product is already flagged `NO_MATCH` and is not
reported again.

"In force" is one definition, shared by every query that reads a current price: the newest period
that has started and has not ended, on a product that is active and that the branch still carries.
A price row outliving the product it belongs to is why the last two conditions are there — nothing
deletes `product_price` when a product is withdrawn.

### What this transition does not do

- **It does not freeze the version.** `is_immutable` stays `false`. `QUOTED` and a frozen version
  are correlated but different things — the seller still edits the draft, and freezing belongs to
  sending it.
- **It does not touch `rfq.status`,** which reached `GENERATED` when the draft was born.
- **It does not price an already-valued version twice.** An unarchived `DRAFT` or
  `CHANGE_REQUESTED` quote with a mutable current version may be valued. Other states answer
  `409` with `QUOTE_NOT_DRAFT`, or `QUOTE_ARCHIVED` on an archived one. A repeated request
  cannot quietly re-value a quote at today's prices.
- **No model is involved, not even to suggest an amount.** The arithmetic is deterministic and it
  is the backend's.

The status write carries the status the caller read in its own predicate, which is what makes the
transition atomic: two callers who both read `DRAFT` cannot both write `QUOTED` and append a history
row each, the second recording a previous status the quote had already left. The branch is in that
predicate too, and in the version read — a quote id arrives from the request, and row level security
guards only the account boundary.

## Line order is the client's order

`quote_item` has no ordinal column, so the order rows come back in is the order they were inserted.
`CreateItems` carries a `position` in its JSON payload and orders by it: `jsonb_to_recordset`
promises no order of its own, and `WITH ORDINALITY` is rejected alongside a column definition list.

Reading them back leans on the same thing. `CreateItems` steps `created_at` one microsecond per
position, so every line of a batch carries its own instant, and `ListItems` orders by `created_at`
and then `id`. An update rewrites a row and moves it physically, so an order that rested on how
Postgres happens to return ties would not survive a price change or a swapped product; this one
does. Lines written before the step share an instant and fall back to `id`.
The alignment that matters for correctness — which decision belongs to which line — is done in
memory before the insert, so this is about what the seller reads, not about matching.

## Bounds

Three settings, all in `apps/api/.env.example`:

- **`RFQ_MAX_TEXT_CHARACTERS`** (20000) caps the order handed to the model. `rfq.raw_text` is
  `TEXT`, so without it a pasted document goes to the provider whole.
- **`RFQ_MAX_ITEMS`** (200) caps the lines one order may produce. Matching runs one query per line,
  so a spreadsheet pasted as text turns one request into hundreds. The number is stated in the
  prompt so the model aims under it and enforced in the service, which is what makes it real — an
  order past the cap is **refused, not truncated**, because keeping the first two hundred lines of a
  three-hundred-line list reads as a complete quote and is not one. A list that long is a
  spreadsheet, and spreadsheets have their own ingest path.
- **`RFQ_PIPELINE_TIMEOUT_SECONDS`** (165) bounds reading, extraction and matching **where nothing
  is waiting on an HTTP response** — the scheduled attachment sweep. It is sized off what a
  generation actually costs: the answer is one forced-schema object of roughly seventy tokens per
  line, so a sixty-item order takes about two minutes to write and a budget under that refuses
  orders nothing is wrong with.
- **`RFQ_INLINE_PIPELINE_TIMEOUT_SECONDS`** (75) is the same pass **with a seller's request still
  open**, and it is a separate, shorter value because it answers to a deadline the sweep does not
  have: `SERVER_EDGE_TIMEOUT_SECONDS`. One shared number would either let a request outlive the edge
  or cut the sweep short for a limit that does not apply to it. **It is deliberately smaller than
  what a large order costs** — see below.
- **`RATE_LIMIT_AI_MAX`** (10) is the fourth, and it lives with the other allowances rather than
  here: it bounds calls per caller per window on the routes that reach a provider. Startup refuses
  a value above `RATE_LIMIT_GLOBAL_MAX`, which could never bite.

That last one exists because the AI timeouts are **per attempt**: `AI_LLM_TIMEOUT_SECONDS` times
`AI_MAX_ATTEMPTS` plus backoff is several times `SERVER_WRITE_TIMEOUT_SECONDS`. Left alone, the
server would cut the response off while it was being written, and the client would read a broken
connection rather than a model that ran out of time. Startup refuses an inline pipeline timeout at or
above the write budget, so the two cannot drift apart.

### The chain, and why the edge is the outer bound

`RFQ_INLINE_PIPELINE_TIMEOUT_SECONDS` (75) < `SERVER_WRITE_TIMEOUT_SECONDS` (90) <
`SERVER_EDGE_TIMEOUT_SECONDS` (100). Startup refuses any ordering but that one.

The edge is **not ours** — it is how long the platform in front waits before it gives up on us, and
nothing in this config can extend it. Whichever deadline fires first decides what the caller sees,
and the difference is not cosmetic: ours produces an answer, the platform's severs the connection
mid-handler. That cancellation takes `markRFQFailed` with it — it runs on the **request** context by
design, so that a pipeline timeout can still record the failure — which leaves the order at
`RECEIVED` and the seller watching a queue that says "in progress" over a pipeline that already
died. None of it reproduces locally, where nothing is in front of the server. Set the edge to `0`
when that is genuinely true.

### A large order outruns the inline budget, and is handed over rather than failed

**A 75-second inline budget cannot finish a large order** — by the sizing above a sixty-item order
needs roughly two minutes — so `CreateFileDraft` hands it to the sweep instead of failing it. The
file is already stored by then, so the intake returns the attachment to the queue (`PENDING`, both
timestamps cleared) and answers with the order at **`RECEIVED`**, which is the truth: it is still
being worked. `attachment-extraction` reads it on its next firing with the longer budget and no
caller holding a connection open, and drafts the quote exactly as the inline path would have.

**Only an interruption is handed over.** A budget that ran out or a provider that was not there says
nothing about the file, and a later run may well succeed. A model that answered and found no
materials, or a failure about the file itself, still moves the order to `FAILED` — the bytes do not
change, so a later run reads the same nothing, and the order is the seller's to load by hand. A
hand-off that cannot itself be written falls back to `FAILED` for the same reason.

So the three outcomes a caller sees are `rfq.status`: **`GENERATED`** with a draft, **`RECEIVED`**
with none (the sweep has it), **`FAILED`** (yours to load). No new field, and nothing to poll that
the queue does not already show.

## Client delivery

`POST /v1/quotes/{quoteId}/sends` is the seller-owned transition from `QUOTED` to `SENT`.
`Idempotency-Key` is a UUID. The body selects at least one destination: an E.164 WhatsApp phone,
an email address, or both. It may override the branch validity with `expiry_days` from 1 through 365.
Each selected type requires exactly one active channel of that type in the quote's branch.
The send dialog defaults to the inbound channel and offers the retained sender as an editable
destination when available. Each tracking row's `operation_id` groups channels from the same send.
The service first ensures the immutable representation bundle, then creates one `PENDING` `quote_send` per selected
channel before calling a provider. WhatsApp and email run independently; at least one successful
channel commits the transition and all selected outcomes are retained. Each channel receives its
own opaque token and `/quotes/{token}` webapp URL.

Provider calls happen outside business transactions. A PostgreSQL advisory lock serializes the
same account, quote and idempotency key across API instances, while separate tenant transactions
prepare and confirm the operation. A replay with the same payload returns the stored result; a
different payload under the same key is a conflict. A new key is an explicit resend and opens a
new validity window without changing older tokens.

The seller-facing detail endpoint exposes the current view and the audit trail together:
`rfq_status_history` comes from `rfq_status_change`, `quote_status_history` comes from
`quote_status_change`, and `deliveries` lists the `quote_send` attempts for the quote. Delivery
rows include the channel, destination, format, `tracking_status`, `public_url`, `sent_at`,
`expires_at`, and `created_at`, including failed attempts so the backoffice can explain why a
quote is still not visible to the client. `public_url` mirrors the exact link the webapp serves
for the send's `public_token`, so the backoffice can offer the client the address he actually
received without ever minting a token of its own.

After the successful confirmation commits, `QuoteQualityEvaluator.EvaluateFinalQuote` compares
the original AI proposal with the frozen version. Evaluation or embedding failures never change
the delivery response. The `quote-quality-evaluation` scheduled job finds missing evaluations
from durable successful sends; the existing `quote-correction-learning` job continues retrying
pending embeddings.

`GET /v1/public/quote-sends/{token}` first resolves only the owning account through the owner
pool, then verifies the completed send under an RLS-scoped transaction. Active tokens expose
the frozen quote, message and a short-lived PDF URL; expired tokens expose only `EXPIRED`
and `expires_at`. See [quote representations](quote-representations.md). The WhatsApp Cloud API
adapter uses the selected branch channel's encrypted Meta credentials. The console mailer is never
treated as a successful client delivery.

### Customer change requests

An active send can receive one public answer. `REQUEST_CHANGE` requires a message of at most
512 characters. Under one tenant transaction, the service locks the quote, records the
`client_action`, links a `CLIENT` `quote_message` to it, copies the frozen version's items and
alternatives into a mutable v2, values the copied lines and alternatives at the quote branch's
current catalog prices, and moves `SENT` to `CHANGE_REQUESTED`. Every copy of a sent version —
this one, the handler's, a reactivation's and a discard's — records on each line the line it was
copied from (`quote_item.source_item_id`), carries over the seller's discount decisions (typed
discounts, and promotions the seller suppressed, each on the copied lines) and runs the sweep, so
the draft starts from exactly the money the customer saw. Products with no current branch price
remain unpriced for seller review. The old send and public link remain pinned to v1. A stale send for
another version cannot change the quote.

No model runs on that request. The same transaction opens the quote's message window with no wait
and closes it, so the message, and any WhatsApp reply still in an open window, lands in a `CLOSED`
batch that the conversation queue picks up on its next pass. The customer sees their request
acknowledged at once; what the model proposes reaches only the seller's draft.

### Conversation batch handler

`BatchChangeHandler` is the `ConversationBatchHandler` the queue runs for every claimed batch, from
the API's ticker or the `conversation-batches` job
([whatsapp-cloud-api.md](whatsapp-cloud-api.md#closing-and-processing-batches)). It works in three
steps, each tenant transaction taking the quote's row lock before the batch's:

1. **Prepare.** It confirms the claim still holds (`status = PROCESSING` at the claim's
   `processing_started_at`) and runs the state×intention matrix (`domain.ValidateHandlerRun`): only
   an unarchived `SENT` or `CHANGE_REQUESTED` quote is handled. A `SENT` quote moves to
   `CHANGE_REQUESTED` with no user on the `quote_status_change` row, and the frozen version is cloned
   into the single draft when none is open; a `CHANGE_REQUESTED` quote keeps the draft it has. It
   reads the batch's `CLIENT` messages in order and the `CONVERSATION_CONTEXT_MESSAGES` newest
   messages of any author outside the batch, then commits, so the move holds whatever the model does.
2. **Propose.** Outside any transaction the forced-schema change handler receives the batch's
   messages, the context, the **draft's** lines with their opaque IDs, and the promotions in force at
   the quote's branch. Its instructions are a fixed text sent first, so the provider caches that
   prefix; everything variable rides in the input. It answers with a `level`:
   - `ATOMIC` — concrete operations: `ADD_ITEM`, `REMOVE_ITEM`, `REPLACE_ITEM`, `UPDATE_QUANTITY`,
     or `ADD_DISCOUNT` naming one of the promotions it was given (never an amount).
   - `COMPLEX` — a goal from a closed set, with no operations: `LOWER_TOTAL_PRICE` or
     `IMPROVE_QUALITY`, plus `intent_item_ids`, the lines the customer tied it to (empty means the
     whole quote).
   - `NO_MATCH` — the escape, which the adapter turns into one `REQUIRES_SELLER` operation.

   The backend checks that the level agrees with what the answer carries, that every line and
   promotion id is one it sent, and the item limit, then runs the matrix
   (`domain.ValidateHandlerIntent`: `ATOMIC`, `ADD_DISCOUNT` or `COMPLEX`), and matches added or
   replaced materials against the branch catalog.

3. **Apply.** Under the lock again the claim, the matrix and the draft are re-checked: a quote
   archived or closed meanwhile fails the batch at once (`domain.ErrBatchUnprocessable`), and a draft
   the seller sent, or a targeted line the seller removed, returns it to the queue for a fresh
   proposal. The operations are written in place, the discount sweep re-settles the draft's
   discounts and total, and `handler_decision` records the batch (`message_batch_id`), the input,
   the interpretation, the proposal and the state the quote was in at the claim. A later batch edits
   the same draft.

A `COMPLEX` goal runs deterministic recipes in the apply transaction, over the named lines or every
line with a product:

| Strategy                    | Runs for            | Each line in scope becomes                                                      |
| --------------------------- | ------------------- | ------------------------------------------------------------------------------- |
| `FIND_CHEAPER_ALTERNATIVES` | `LOWER_TOTAL_PRICE` | its cheapest `ECONOMY` alternative the branch prices below its own unit price   |
| `ADAPT_TO_REACH_DISCOUNT`   | `LOWER_TOTAL_PRICE` | the same line at the next threshold of a promotion it targets, if that is lower |
| `IMPROVE_QUALITY`           | `IMPROVE_QUALITY`   | its cheapest `PREMIUM` alternative the branch prices                            |

Only an active alternative sold in the base product's unit counts, and ties go to its name:
`product_alternative` has no rank. The swaps are written as `REPLACE_ITEM` operations and priced at
once at the branch price the recipe chose them by; quantities never change. `IMPROVE_QUALITY` has
no cap.

`ADAPT_TO_REACH_DISCOUNT` never adds a product and never lowers a quantity. A line's threshold is the
least quantity above its own at which an in-force `PER_ITEM` promotion (a condition's
`min_quantity`) or `QUANTITY_TIERED` one (the first tier, or the next one) targets the line's
product, family or subgroup and earns a figure the line does not earn already. `ITEM_SET` and
`ON_TOTAL` rules are never adapted to, and a promotion the seller suppressed on the draft is
skipped. The lines are walked in draft order and each keeps the raise whose engine total, evaluated
in memory on the swept state, is lowest — only if it is lower than what the walk had reached. The
raises are written as `UPDATE_QUANTITY` operations, the draft is swept again, and the strategy
stands only if that total ends strictly below the swept baseline. An engine application the raises
earned (one on a raised line that was not live with the same figure before) is stored with origin
`AI_ADAPTATION`.

`LOWER_TOTAL_PRICE` runs its two strategies "first that works": the first one is the customer's
wording (the model's optional `lower_price_strategy`, valid only with `COMPLEX` +
`LOWER_TOTAL_PRICE`), else the account's `preferred_lower_price_strategy` (an admin setting on
`PUT /v1/account`, default `FIND_CHEAPER_ALTERNATIVES`). A strategy with no lower total rolls its
transaction back and the other runs on the untouched draft; when neither delivers, the no-match
lists both in `strategies_tried`, in run order.

An `ADD_DISCOUNT` needs nothing when the promotion is already applied and restores it when the
seller suppressed it. A `PER_ITEM` or `QUANTITY_TIERED` promotion the quote does not earn is reached:
every line it targets below its threshold is raised to the least quantity that earns it, kept only
where the engine then applies that promotion on the line, and the total may rise — the customer
asked for that promotion, and the seller reviews it. A line the same proposal requantified keeps the
customer's quantity. The run records `ADAPT_TO_REACH_DISCOUNT` and the raises; an `ITEM_SET` or
`ON_TOTAL` promotion, or one no raise earns, is refused with `PROMOTION_NOT_EARNED`.

An `AI_ADAPTATION` application belongs to the engine like an `AUTOMATIC` one: the sweep recomputes
its amount, keeps it (and its origin) while it is still earned and drops it when it is not, and the
seller suppresses or restores it the same way. A draft cloned from a version carries it as a plain
`AUTOMATIC` promotion, suppressed or not.

The model never sets money: a requantified line keeps its unit price, a new or replaced line waits
unpriced for the seller to accept materials (a recipe's swap is the exception, priced by the
backend), and every discount comes from the sweep. It never answers the customer and never infers
an acceptance or a rejection: "dale, acepto" is its `NO_MATCH` escape. That escape, a proposal the
backend refuses, and a deployment with no language model bound (`AI_LLM_PROVIDER=disabled`; the
model is never called) record a `NO_MATCH` decision with origin `DIRECT_INPUT` and its reason
(`REQUIRES_SELLER`, `PROPOSAL_REFUSED`, `AI_DISABLED`). A recipe with no result, or a promotion the
quote does not earn, rolls the whole write back — atomic operations riding beside it included — and
records a `NO_MATCH` with origin `PIPELINE_EXHAUSTED` and reason `PIPELINE_EXHAUSTED` or
`PROMOTION_NOT_EARNED`. Every no-match finishes the batch `PROCESSED`, so nothing loops.
`handler_decision.ai_proposal` stores
`{outcome, no_match_origin, no_match_reason, interpretation, intent, strategies_tried,
preferred_strategy, adaptation, operations}` — `adaptation` holding each raised line (from, to, the
promotion it reached) and the engine totals before and after, which the detail's handler note and
the conversation panel return to the seller —`,
and `draft_before`/`draft_after`the draft's lines and seller-controlled discounts (typed or
suppressed) around the change, without prices. A provider failure is retried like any other error,
up to`CONVERSATION_MAX_ATTEMPTS`; a batch that fails records `failure_reason`: `UNPROCESSABLE`for
a refusal no retry can fix,`ATTEMPTS_EXHAUSTED` for one that ran out of attempts.

#### The seller's decision

Each `handler_decision` row is completed when the seller decides, which happens in one of two ways:

- **Sending the draft** settles every undecided row in the transaction that freezes it. An applied
  run is `APPROVED_AS_IS` when nothing the seller controls changed after it — no gap between its
  `draft_after` and the next run's `draft_before`, nor between the last run and the draft as frozen —
  and `EDITED` otherwise; repricing is not an edit, since prices are not in the snapshot. A `NO_MATCH`
  run is `MANUAL_OVERRIDE`. `user_id` and `decided_at` name the sender.
- **`POST /v1/quotes/:quoteId/discard-ai-changes`** resets the draft in place to a copy of the
  version the customer last received, with its discount decisions, repriced and swept like a fresh
  draft, and records every
  pending applied run `REJECTED`. The quote stays `CHANGE_REQUESTED`; a draft with no pending applied
  run answers `409 NO_AI_CHANGES`. Pending no-match runs stay for the send to settle.

`POST /v1/quotes/:quoteId/message-batches/:batchId/retry` returns an `ATTEMPTS_EXHAUSTED` batch to
`CLOSED` with a fresh count, keeping its place by `closed_at`, while the handler may still act on the
quote; anything else answers `409 BATCH_NOT_RETRYABLE`.

The RFQ detail's `changes_requested` compares the draft with the version the customer received:
`lines` pairs each sent line with the draft line copied from it (by `source_item_id`, never by
position) as `UNCHANGED`, `MODIFIED` or `REMOVED`, then the lines the draft added as `ADDED`;
`discounts` pairs a promotion with the same promotion — a line one on the sent line its draft line was copied from — and anything else by name, each side carrying its `origin`; `original_total` and `requested_total` close it.
`handler` carries the latest decision on the draft, and `client_messages` every message the customer
sent since the version they answered was frozen. The backoffice shows them as one card: the
customer's words, the reading and its outcome, and only the changed rows. `conversation` carries the whole
thread: each batch with its messages, status, `failure_reason`, `retryable`, and the handler's run
with the seller's decision; `pending` for a window still open; `seller_messages`, the quote's sends;
and `can_discard_ai_changes`. It is absent until the customer has written.

Each successful send is recorded as a `SELLER` `quote_message` on the send's channel, authored by
the sender, in the transaction that commits the send. Its body is the representation's message with
the public link left as the `{{public_url}}` placeholder, so the token lives only on `quote_send`. A
seller message never opens or extends a window; the handler reads it as context.

### Closing and reactivating a quote

An active public link records `ACCEPT`, `REJECT`, or `REQUEST_CHANGE` as a `client_action`. The
first two move `SENT` to `ACCEPTED` or `REJECTED`; the seller can record the same closing states
through `POST /v1/quotes/{quoteId}/transition` when the answer arrived outside Coti. Customer
actions carry no internal user id, while seller actions credit the authenticated user in
`quote_status_change`.

`POST /v1/quotes/{quoteId}/reactivate` is the only way out of either terminal state and always
operates on the same quote:

It refuses a sale that still stands as closed: **409** `QUOTE_ALREADY_INVOICED` while it has an
invoice in flight or issued and not cancelled by a credit note, and **409** `QUOTE_ALREADY_PAID`
while it is marked paid — the seller takes the mark back first. Reactivating clears «No requiere
factura» in the same transaction: a reopened sale is a new sale to close.

- `RESEND` moves the quote back to `QUOTED` and retains the current frozen version unchanged.
- `EDIT` copies the frozen version into the next mutable version, resets alternative approvals,
  values products and alternatives at the branch's current prices, records the seller as author,
  and moves the quote to `CHANGE_REQUESTED`. The seller then reviews the draft and accepts its
  materials before sending it.

The quote row is locked and the version copy, current-version pointer, status update, and
`quote_status_change` append share one tenant transaction. A quote lifecycle event is written only
to `quote_status_change`; `rfq_status_change` remains the RFQ ingestion ledger. The backoffice
tracking view combines both histories chronologically without duplicating one event across tables.
Archiving remains an orthogonal flag: it can hide a quote in any lifecycle state, including
`ACCEPTED` and `REJECTED`, and restoring it preserves that status.

### Closing a sale: paid and «No requiere factura»

Closing an accepted sale adds **facts, not statuses**: `quote.paid_at` / `paid_by_user_id` record
«Cobrado» and `invoice_waived_at` / `invoice_waived_by_user_id` record «No requiere factura», each
timestamp paired with its user by a CHECK and the user tied to the quote's account by a composite
foreign key. Neither touches `current_status`, `quote_status_change`, the state×intention matrix or
anything the conversation handler reads. Every write moves `quote.updated_at`, which is what the
inactivity sweep measures.

- `POST`/`DELETE /v1/quotes/{id}/payment` marks or unmarks the sale paid, by the caller, now. A
  repeat of the state the quote is already in changes nothing, so a second mark keeps the first
  author and time.
- `POST`/`DELETE /v1/quotes/{id}/invoice-waiver` takes the sale out of the invoicing wait or puts it
  back. Setting it is refused with **409** `ACCOUNT_INVOICING_OFF` while the account does not invoice
  with Coti, and with **409** `QUOTE_ALREADY_INVOICED` while the sale has a pending invoice or an
  issued one no credit note cancelled.
- All four answer **409** `QUOTE_NOT_ACCEPTED` on a quote that is not accepted and
  `QUOTE_ARCHIVED` on an archived one, and return the quote.

**Paying never gates invoicing, and Coti never invoices on its own.** «Marcar cobrado» in the
backoffice offers «Facturar también» with the invoice preview (type, receiver, total) in the same
confirmation, ticked by default when the account's `invoice_on_payment` is on and shown only while
invoicing is on and the sale has no live invoice; a preview that reports gaps disables it and names
them. The paid mark is committed first and the invoice issued after it, as two requests: an
invoice ARCA refuses leaves the sale paid, and the invoice card shows the refusal.

**An issued invoice ends the waiver.** The transaction that records ARCA's authorization of an
invoice — on the first answer or on reconciling a pending one — clears «No requiere factura» on the
sale. A refused or released attempt leaves it as the seller set it. Reconciling adopts a pending
invoice in the environment it was requested in, so one from the other environment still clears the
waiver but does not count as invoiced here, and the sale reopens waiting for its invoice.

**A complete sale is derived, never stored.** `GET /v1/rfqs` and `GET /v1/rfqs/{id}` carry, per
order, `paid_at`, `paid_by` (the user's name), `invoice_waived_at`, `invoice_waived_by`, `invoiced`
— an issued invoice no credit note cancelled, in this server's ARCA environment or in production —
`invoice_pending` — a voucher still waiting for ARCA, so the queue offers no second one — and
`sale_complete`: accepted, paid, and invoiced or waived, or the account does not invoice with
Coti — never while any voucher of the sale still waits for ARCA, in either environment. `domain.SaleComplete` is the one place that rule lives; a screen reads the answer rather than
re-deriving fiscal state.

### Sales awaiting an invoice («Por facturar»)

`GET /v1/quotes/awaiting-invoice?page=N` lists the accepted, unarchived sales that nothing stands
for yet: no issued invoice a credit note did not cancel (in this server's environment or in
production), no voucher waiting for ARCA in any environment, and no «No requiere factura». A
refused invoice, or one a credit note cancelled, leaves the sale on the list. While the account does
not invoice with Coti the list is empty. **Paid sales come first, then the longest accepted**,
dated by the latest `ACCEPTED` entry in `quote_status_change`: a paid sale is the one most ready to
invoice, and among the rest the oldest has waited longest.

It reads the same reach as the queue — the active branch, or every branch the caller reaches when
`X-Branch-Id` is absent, and for a seller only the sales they own or nobody claimed — so the list
never offers a sale its reader cannot open. Each row carries the quote and order ids, the number,
the branch, the client (or the order's loose label), the total, the acceptance date and `paid_at`.
It pages like the invoice history: `{items, total, page, page_size}`, 25 rows a page, a page
outside 1–100000 refused with **422**. One count and one page query, the invoice facts read by
lateral `EXISTS` subqueries rather than per row.

The backoffice shows it as the «Por facturar» view of Administración, beside «Comprobantes», only
while the account invoices (`?view=pending`; any other value, or the switch off, is the history).
Each row offers «Facturar», through the same confirmation as the queue and the invoice card, and
«No requiere factura», which takes the row off the list at once and reads the page again.

### Archiving a quote

`quote.archived_at` is a flag beside the state machine, never a state. `POST
/v1/quotes/{quoteId}/archive` sets it on a quote in any status and `POST .../unarchive` clears it;
neither writes `quote_status_change`. The queue leaves archived quotes out unless
`?include_archived=true` asks for them, and `idx_quote_expires` skips them. The backoffice reads the
queue without them and asks for them only while the table's «Archivado» filter is on.

**Every edit on an archived quote answers `409 QUOTE_ARCHIVED`** until it is unarchived: lines and
discounts, accepting materials, approving an alternative, a transition, reactivation, sending,
generating the PDF, the client association, issuing the invoice, and marking the sale paid or as
needing no invoice (either way). The guard is checked before the
status, so the refusal names the archive rather than a status the seller cannot see a reason for.
Reads stay open, and so does recovering an invoice already in flight at ARCA (the backoffice shows
the invoice card read-only and offers neither action until the quote is unarchived).

A seller may archive a `SENT` quote whose link is still live. The public side then stops offering
it: `GET /v1/public/quote-sends/{token}` answers `status: UNAVAILABLE` with no quote body, and an
answer through the link is refused with `QUOTE_ARCHIVED`, which the webapp words as an offer no
longer available. A customer who already answered keeps seeing their answer.

Quotes also archive themselves: the `quote-auto-archive` job puts away the ones nobody has moved in
`QUOTE_AUTO_ARCHIVE_INACTIVE_DAYS` — a sent one only once its offer has lapsed — and the complete
sales and rejected quotes quiet for `QUOTE_AUTO_ARCHIVE_CLOSED_DAYS`. See [scheduled-jobs.md](scheduled-jobs.md#quote-auto-archive).

## Where the code lives

| Piece                       | File                                                         |
| --------------------------- | ------------------------------------------------------------ |
| Ports and types             | `internal/domain/rfq.go`                                     |
| Prompt and forced schema    | `internal/ai/rfq_extractor.go`                               |
| The flow and its invariants | `internal/services/rfq_service.go`                           |
| Valorization                | `internal/services/quote_{service,helpers}.go`               |
| Channel discovery           | `internal/services/channel_service.go`                       |
| Candidates per line         | `internal/services/rfq_service.go` (`alternativesFromMatch`) |
| SQL                         | `internal/repository/{rfq,quote,channel}_repo.go`            |
| Routes and DTOs             | `internal/delivery/http/{handler,dto}/{rfq,quote}_*.go`      |
| Client delivery             | `internal/services/quote_delivery_service.go`                |
| Delivery persistence        | `internal/repository/quote_send_repo.go`                     |
| Change request persistence  | `internal/repository/quote_message_repo.go`                  |
| Evaluation retry            | `internal/services/quote_quality_job.go`                     |

`RFQExtractor` is a **feature port**: its adapter owns the prompt and the schema and reaches the
model through `StructuredGenerator`, so it names no provider and works behind whichever one is
bound. The schema carries no `minLength`, `maxLength` or `maxItems` — structured outputs do not
enforce those, and stating them would read as a guarantee the service is the one making.

## QA surfaces

The automated RFQ suites never call a live model. `pnpm test:rfq` uses fixed provider doubles for
fast service and handler checks; `pnpm test:rfq:integration` uses the same deterministic answers
over the real router, PostgreSQL, pgvector search, tenant context, and quote persistence. This keeps
CI repeatable while still proving that a mocked WhatsApp message reaches a reviewable `DRAFT`.

`pnpm eval:rfq` is the opt-in model evaluation. It logs into a running development API, posts the
cases from `scripts/fixtures/rfq-eval-cases.json` to `/v1/dev/whatsapp/messages`, and compares only
observable contract fields: RFQ and quote status, line count, quantities, units, rationales, match
status, and pricing when requested. It prints one `PASS` or `FAIL` per case and stores both the
machine-readable JSON and a self-contained interactive HTML dashboard under the ignored
`.artifacts/rfq-eval/` directory. The dashboard exposes the case description, declared
expectations, source definition, extracted lines, and complete HTTP responses. `--verbose` also
prints every response; `--price` runs the deterministic material-acceptance transition after every
draft. `pnpm report:rfq` rebuilds the HTML from the latest JSON without contacting a provider.

The runner reads its connection settings from `RFQ_EVAL_*` variables and has defaults for the
development seed. A bearer token can be supplied as `RFQ_EVAL_TOKEN`; otherwise it logs in with
`RFQ_EVAL_EMAIL` and `RFQ_EVAL_PASSWORD`. Run `pnpm eval:rfq --help` for the full option list.

### Live trace and debugging

`pnpm debug:rfq` runs the complete live suite with `--trace` against the development API on port
`8001`; `pnpm debug:rfq:case` limits it to `explicit-quantity` for breakpoint work. The trace
separates API readiness, authentication, WhatsApp ingestion, RFQ persistence, model extraction,
catalog matching, draft persistence, deterministic pricing, and expected-response assertions. A
failed assertion is assigned to the stage whose observable contract diverged. Each HTTP response
stores its `X-Request-Id`, so the report can be correlated with the API request log.

`pnpm serve:rfq` serves an interactive QA Lab at http://localhost:4173 using only the Node standard
library. Its fixed registry exposes unit surfaces for extraction, RFQ orchestration, matching,
pricing and the HTTP contract; a PostgreSQL-backed integration surface; and live WhatsApp custom
or suite evaluations. The browser can select only those registered commands and same-origin POSTs
are enforced, so the local server is not an arbitrary command runner.

Before a run, the server performs a surface-specific preflight and rejects blocked requests. Unit
surfaces require Go and their registered source files. The integration surface additionally
requires pnpm, both test database URLs, a reachable PostgreSQL instance, and the pgvector
migration; its button invokes `pnpm test:rfq:integration` directly. Live surfaces require a healthy
API, PostgreSQL, pgvector, and both provider keys. Key values never leave the server.

Custom WhatsApp cases are validated and stored under ignored `.artifacts/rfq-eval/` data. They can
declare expected RFQ/quote status, line count, first-line description, quantity, unit and match
status, and can be deleted without removing reports from previous runs. A live run selects one
active branch loaded through the evaluator's authenticated user,
displays its providers, and requires an explicit confirmation before the server starts it. The
selected id is passed as `--branch` and recorded with the run. Deterministic surfaces state that
they consume no provider. The Lab polls each run for
stdout/stderr, status and its eventual detailed report link. Deterministic Go runs use
`go test -json`, which the Lab converts into expandable rows with a human-readable description,
status, duration, related test function, and captured output. `/latest` opens the newest report,
whose **Trazabilidad** tab shows setup and case events, durations, failure details and request ids.
Serving, browsing or rebuilding a report never contacts an AI provider.

The committed VS Code launch configuration provides **RFQ START HERE: API + ALL TESTS** for the
complete dashboard and **RFQ DEBUG: API + ONE CASE** for focused breakpoint work. Their test-only
child configurations are hidden because they require an API that is already running. After the Go
extension is installed, reload the VS Code window once so its debug adapter is registered. Each
compound starts the API under Delve and runs the evaluator after its health endpoint becomes
available, allowing Go breakpoints inside extraction, matching, and persistence while the client
timeline remains visible.
