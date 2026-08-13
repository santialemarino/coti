# AI providers

Every call to an external model goes through one of three ports. Which provider answers is a
startup decision; nothing above a port knows, and no service imports a provider package. This is
the layer the RFQ engine sits on: item extraction, catalog matching, the vector index and
multi-format ingest all reach their models through it.

## The three ports

They live in `apps/api/internal/domain/ai.go`, so services and adapters both depend on them
without a cycle. Adapters live under `apps/api/internal/ai/`, one subpackage per provider, and
`cmd/api/main.go` is the only place any of them is bound.

| Port                  | Method                        | What it does                                      |
| --------------------- | ----------------------------- | ------------------------------------------------- |
| `StructuredGenerator` | `Generate(ctx, request, out)` | Asks a language model for one schema-valid answer |
| `Embedder`            | `Embed(ctx, texts)`           | Turns text into catalog-width vectors             |
| `Transcriber`         | `Transcribe(ctx, audio)`      | Turns a recording into text                       |

**Each capability is selected on its own**, because no provider covers all three: Anthropic
exposes no embedding or speech-to-text API, which is the entire reason a second provider exists.
An environment can run the language model with the transcriber off.

| Capability                  | Setting                     | Providers               |
| --------------------------- | --------------------------- | ----------------------- |
| Language model, vision, PDF | `AI_LLM_PROVIDER`           | `disabled`, `anthropic` |
| Embeddings                  | `AI_EMBEDDINGS_PROVIDER`    | `disabled`, `openai`    |
| Transcription               | `AI_TRANSCRIPTION_PROVIDER` | `disabled`, `openai`    |

Adding a fourth provider is a new subpackage plus one `case` in the composition root. The retry
policy, the usage log and the error wrapping are provider-agnostic and already shared, so nothing
in `domain`, `services` or another adapter changes.

### Every capability arrives disabled

The default for all three is `disabled`, and the stand-in adapters refuse every call with
`domain.ErrAIUnavailable`. That is what lets a fresh checkout boot with no keys: the failure lands
on the call that needed a model, as a controlled error the caller handles, instead of on startup.
Turning a capability on makes its key required — a blank one fails the boot with the key named,
and every problem in the same pass rather than one restart per typo.

The stand-ins refuse rather than answer on purpose. A stub returning invented line items would be
the one thing this layer exists to prevent.

## Schema-forced generation

`GenerationRequest` carries the prompt in two halves, and the split is load-bearing:

- **`Instructions`** is the stable half — the action catalog, the rules, the pipeline. It is sent
  as the system prompt and marked for caching, so a repeat call is charged in full only for what
  follows it. The marker is a no-op below the provider's minimum cacheable prefix (~512 tokens),
  which a short instruction block will not reach — the saving arrives once the catalog and rules
  are real, not on a one-line prompt.
- **`Input`** is the variable half, as a list of blocks: text, an image, or a document. One port
  therefore covers a typed message, a photographed materials list and an attached PDF, because the
  same model reads all three — no separate OCR service.

`Schema` is the JSON Schema the answer has to satisfy, sent as `output_config.format`. The model
is structurally unable to return an action outside the schema's closed enums, which is why the
catalog carries an explicit escape value: "I cannot resolve this" has to be a valid answer rather
than a reason to invent one.

**A non-conforming answer is rejected and asked for again, never repaired.** The adapter decodes
into the caller's own type with unknown fields refused, inside the retry loop — so a schema that
drifts away from the struct behind it fails loudly instead of silently dropping a field. A
truncated answer (`max_tokens`) is retried too. A refusal is not: the same prompt would be refused
again.

**The output is zeroed before each attempt, and that is load-bearing.** `encoding/json` assigns
fields as it walks, so a rejected answer leaves its values in the caller's struct; the next attempt
would then decode over the top and hand back a mix of the two — a quantity from the answer that was
thrown away, which is exactly the invented number this layer exists to prevent.

`AI_LLM_MAX_TOKENS` is a ceiling, not a default: a request may ask for less but never for more, so
the env key is the operational limit it claims to be.

**Reasoning depth replaces temperature.** The model no longer accepts `temperature`, `top_p` or
`top_k` — sending any of them is rejected outright. `AI_LLM_EFFORT` is the knob that took over,
and it defaults to `low` for the same reason the closed decision asked for a low temperature:
extraction and classification are mapping work, not open-ended writing. Note that
`AI_LLM_MAX_TOKENS` caps the answer **and** the model's own reasoning together, so a tight budget
shows up as a truncated answer rather than a terse one.

## Embeddings and the catalog width

`domain.EmbeddingDimension` is 1536 because `product.embedding` is `VECTOR(1536)`. The adapter
asks the provider for that width explicitly and **refuses any other**, so a model swap that
changes it fails at the call instead of writing vectors the column cannot hold. Changing the
constant means an `ALTER` on the column and re-embedding the whole catalog.

Texts are sent in batches of `AI_EMBEDDINGS_BATCH_SIZE` and come back index-aligned with the input.

**Batching is not an optimisation.** A catalog import hands over thousands of descriptions at once,
and one request carrying all of them exceeds the provider's per-request limits and is rejected
wholesale — losing every vector in the batch, with nothing to tell the caller where the cap was. A
batch that fails fails the whole call, rather than returning a short list a caller would read as
complete.

**Within a batch, entries are placed by the index each one carries**, not by the order they arrive
in: the provider makes no ordering promise, and trusting arrival order attaches the wrong vector to
the wrong product with nothing to notice it. Those indexes are per request, so the adapter maps each
batch back onto its slice of the caller's list.

**Binding a `pgvector.Vector` as a query argument needs a codec**, registered from the pool's
`AfterConnect` in `apps/api/internal/repository/db.go` alongside the decimal one. It costs a
`SELECT` per connection, which is what looks the type's oid up in the database, so it has to be
per connection rather than once at startup: the pool opens them on its own schedule.

What is done with the vectors — the catalog backfill, the hybrid search and the index it reads —
is in `catalog.md`.

## Retries, timeouts and the usage log

One policy, shared by every adapter, in `apps/api/internal/ai/retry.go`.

- **Retried:** rate limits, provider-side faults, connections that never produced a response, and
  answers that miss the schema. The wait doubles from `AI_RETRY_BACKOFF_SECONDS` up to
  `AI_MAX_BACKOFF_SECONDS` — without a ceiling the doubling is unbounded, and eight attempts alone
  would reach a two-minute wait.
- **A provider that names its own window wins.** A `Retry-After` header is honoured in place of our
  ladder, because a real rate-limit window is measured in tens of seconds and three attempts at 1s
  and 2s would all land inside it and fail. Asked for longer than the ceiling, we stop instead of
  sitting it out: every attempt inside that window would fail and spend the allowance anyway.
- **Not retried:** a request the provider rejected, and a refusal. Repeating either only spends the
  allowance. Only the adapter knows which of its failures are transient, so it marks them and the
  shared loop reads the mark; `ai.RetryableStatus` is the single definition of which statuses count,
  so adding one is one edit rather than one per provider.
- **Timeouts are per attempt**, not per chain: `AI_LLM_TIMEOUT_SECONDS` and its two siblings cap one
  call, and `AI_MAX_ATTEMPTS` bounds how many there are. **The whole chain can therefore run far
  longer than `SERVER_WRITE_TIMEOUT_SECONDS` allows a response** — 60s × 3 for one generation
  against a 30s server budget. The first AI-backed route cannot simply wait for a call inline: it
  belongs off the request path, or that budget has to be raised deliberately for it.

Every call is logged once, on success and on failure alike, with provider, model, operation,
attempt count, elapsed time and token counts. **The counts are summed over every attempt**, because
each attempt submitted the prompt and was charged for it — a call that succeeds on its third try
cost three prompts, and one that failed after three still cost something. Both cache figures are
recorded separately: writing the instruction prefix costs more than an ordinary input token and
reading it costs a fraction, and neither is included in the input count, so total input is the
three added together. Transcription reports no token count, and logs zero rather than a guess.

Failures are told apart by whether retrying could ever help:

| Failure                                                    | Caller sees                          | Status |
| ---------------------------------------------------------- | ------------------------------------ | ------ |
| No provider bound, an outage, a rate limit, attempts spent | `domain.ErrAIUnavailable`            | 503    |
| The provider rejected the request, or refused the prompt   | a plain error, the detail in the log | 500    |
| A caller's own mistake, caught before any round trip       | `domain.ErrInvalidInput`             | 422    |
| The caller cancelled                                       | `context.Canceled`                   | —      |

The middle row is the one worth naming: a malformed schema, a wrong model or a bad key is **our**
fault, and reporting it as an outage would point monitoring at a healthy provider and invite a
client to retry a request that can never succeed.

## What this layer does not decide

The invariants stay above the ports, in the services:

- **The AI never writes to production.** The chain is AI proposes → backend validates → seller
  approves. These ports return proposals; nothing here persists anything.
- **The AI never calculates money.** The deterministic discount engine does.
- **Every proposal is validated against the state×intention matrix** before it materializes.

## Configuration

Every key, with its default and what it selects, is documented in `apps/api/.env.example` under
`--- AI providers ---`. Defaults live in `apps/api/internal/config`, and `config.Load` validates
the lot at startup.
