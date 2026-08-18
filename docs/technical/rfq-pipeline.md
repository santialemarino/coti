# The plain-text RFQ pipeline

What happens between a client's informal message and a quote a seller can review. One text goes in;
one `quote` in `DRAFT`, its first unfrozen `quote_version`, and one `quote_item` per material come
out, each line carrying the product it matched, how confident that is, and why its quantity is what
it is.

The AI provider layer is in [ai-providers.md](ai-providers.md), the search and matching behind the
product decision in [catalog.md](catalog.md). This document is the flow that consumes both.

## Endpoints

| Method | Path                        | What it does                                                         |
| ------ | --------------------------- | -------------------------------------------------------------------- |
| `POST` | `/v1/rfqs/text-drafts`      | Runs an order the seller pasted or typed through the pipeline        |
| `GET`  | `/v1/channels`              | The active intake channels of the selected branch                    |
| `POST` | `/v1/dev/whatsapp/messages` | Simulates one inbound WhatsApp message. Not registered in production |

All three are branch-scoped and read the branch from `X-Branch-Id`. `channel_id` is required on a
text draft, which is why the channel listing exists: `rfq.channel_id` is `NOT NULL`, and a caller
has to name the route the order arrived through rather than have one guessed for it.

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

## Line order is the client's order

`quote_item` has no ordinal column, so the order rows come back in is the order they were inserted.
`CreateItems` carries a `position` in its JSON payload and orders by it: `jsonb_to_recordset`
promises no order of its own, and `WITH ORDINALITY` is rejected alongside a column definition list.
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

That last one exists because the AI timeouts are **per attempt**: `AI_LLM_TIMEOUT_SECONDS` times
`AI_MAX_ATTEMPTS` plus backoff is several times `SERVER_WRITE_TIMEOUT_SECONDS`. Left alone, the
server would cut the response off while it was being written, and the client would read a broken
connection rather than a model that ran out of time. Startup refuses a pipeline timeout at or above
the write budget, so the two cannot drift apart.

This bounds the symptom. The wider answer is to move extraction off the request path, which is its
own piece of work: there is no queue in the API today, and `cmd/scheduled-job` runs work on the
platform's cron rather than on demand.

## Where the code lives

| Piece                       | File                                              |
| --------------------------- | ------------------------------------------------- |
| Ports and types             | `internal/domain/rfq.go`                          |
| Prompt and forced schema    | `internal/ai/rfq_extractor.go`                    |
| The flow and its invariants | `internal/services/rfq_service.go`                |
| Channel discovery           | `internal/services/channel_service.go`            |
| SQL                         | `internal/repository/{rfq,quote,channel}_repo.go` |
| Routes and DTOs             | `internal/delivery/http/{handler,dto}/rfq_*.go`   |

`RFQExtractor` is a **feature port**: its adapter owns the prompt and the schema and reaches the
model through `StructuredGenerator`, so it names no provider and works behind whichever one is
bound. The schema carries no `minLength`, `maxLength` or `maxItems` — structured outputs do not
enforce those, and stating them would read as a guarantee the service is the one making.
