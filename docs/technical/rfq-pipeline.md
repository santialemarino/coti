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

| Method | Path                               | What it does                                                         |
| ------ | ---------------------------------- | -------------------------------------------------------------------- |
| `POST` | `/v1/rfqs/text-drafts`             | Runs an order the seller pasted or typed through the pipeline        |
| `GET`  | `/v1/channels`                     | The active intake channels of the selected branch                    |
| `POST` | `/v1/dev/whatsapp/messages`        | Simulates one inbound WhatsApp message. Not registered in production |
| `POST` | `/v1/quotes/{id}/accept-materials` | Prices the draft's lines and moves the quote to `QUOTED`             |

The two that reach a model share **their own rate-limit allowance**, `RATE_LIMIT_AI_MAX` — the global one would let a single seller spend 300 generations a minute, and this is the first surface in the product billed per call. Valorization reaches no provider and spends nothing, so it stays on the global allowance.

All four are branch-scoped and read the branch from `X-Branch-Id`. `channel_id` is required on a
text draft, which is why the channel listing exists: `rfq.channel_id` is `NOT NULL`, and a caller
has to name the route the order arrived through rather than have one guessed for it. How a channel
is configured, and where its provider credentials live, is in
[accounts-and-branches.md](accounts-and-branches.md#channels).

The development route resolves the branch's WhatsApp channel and then calls the same service method
the production route does. It is a different way in, not a second pipeline — a copy would drift from
the path it is meant to rehearse. It is registered only when `ENV` is not `production`.

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
opening hours. Then **no quote is created and the `rfq` stays `RECEIVED`**, and the route answers
`201` with the order alone: the text was stored, which is the part that matters.

This is not an error case. `GENERATED` means the engine produced materials, so an order that
produced none has not reached it, and a `quote` with no lines would be indistinguishable from an
order for nothing. The response carries `quote` and `version` as `null` for exactly this case.

## What a failed match does

Matching is asked for a decision per line and may not be able to give one — the embedding provider
is `disabled`, the catalog has no vectors yet, the provider is down. **Every line then stays
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

| Line status | What it offers                      | Why                                                             |
| ----------- | ----------------------------------- | --------------------------------------------------------------- |
| `MATCHED`   | Nothing                             | The line is decided; there is nothing to choose between         |
| `AMBIGUOUS` | Every candidate but the one it kept | It kept the leader, so the offers are the products it might be  |
| `NO_MATCH`  | Every candidate                     | It points at nothing, so the closest near miss is the first one |

**A candidate that scored zero is dropped from both**, which is the one exception to that table.

**`rank` is the candidate's place in the matcher's ranking, not a renumbering.** An `AMBIGUOUS`
line's offers therefore start at two: rank one is the product on the line. Ranks can also skip,
because the candidates are ordered by the fused rank rather than by score, so a dropped zero can
sit between two offers. `confidence_score` is what the candidate scored, on
`quote_item.confidence_score`'s own scale, so a 59% near miss reads differently from a 12% long
shot. Neither figure exists anywhere else on the row — `created_at` is the transaction's timestamp,
shared by every row of one insert — which is why the table carries both columns.

A zero means no similarity at all: the search reached the product because the top-K is wider than
the catalog, or because it shares a word with the line. Offering those would bury the near miss
under everything the account sells.

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

**`quote_version.total` is Σ subtotals − Σ discounts.** The promotion sweep that produces the
second term is US-38, so today it is zero — part of the formula rather than missing from it.

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
- **It does not touch `rfq.status`,** which only has `RECEIVED` and `GENERATED`.
- **It does not price a second time.** Only an unarchived quote at `DRAFT` may be valued; anything
  else answers `409` with `QUOTE_NOT_DRAFT`, or `QUOTE_ARCHIVED` on an archived one. Re-pricing an
  already-valued version is an explicit act of the seller's, not the side effect of a repeated
  request — a double-clicked button must not quietly re-value a quote at today's prices.
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

Reading them back leans on the same thing. `ListItems` orders by `created_at`, and one batch shares
a single value of it, so that clause separates lines added later from the original ones and leaves
the batch itself to the order it was written in. Valorization updates every line, which rewrites
those rows — measured at 200 lines, the order survives it, but nothing in Postgres promises that.
An ordinal column is the fix if the order ever has to be guaranteed rather than observed, and
editing lines will want one anyway.
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
- **`RFQ_PIPELINE_TIMEOUT_SECONDS`** (25) bounds extraction and matching together.
- **`RATE_LIMIT_AI_MAX`** (10) is the fourth, and it lives with the other allowances rather than
  here: it bounds calls per caller per window on the routes that reach a provider. Startup refuses
  a value above `RATE_LIMIT_GLOBAL_MAX`, which could never bite.

That last one exists because the AI timeouts are **per attempt**: `AI_LLM_TIMEOUT_SECONDS` times
`AI_MAX_ATTEMPTS` plus backoff is several times `SERVER_WRITE_TIMEOUT_SECONDS`. Left alone, the
server would cut the response off while it was being written, and the client would read a broken
connection rather than a model that ran out of time. Startup refuses a pipeline timeout at or above
the write budget, so the two cannot drift apart.

This bounds the symptom. The wider answer is to move extraction off the request path, which is its
own piece of work: there is no queue in the API today, and `cmd/scheduled-job` runs work on the
platform's cron rather than on demand.

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
