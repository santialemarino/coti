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

Texts go in one request and come back index-aligned. The entries are placed by the index each one
carries, not by the order they arrive in — the provider makes no ordering promise, and trusting
arrival order attaches the wrong vector to the wrong product with nothing to notice it.

**Binding a `pgvector.Vector` as a query argument needs a codec pgx does not have yet.** The type
is the port's return value, but nothing persists one yet, so the codec is deliberately not
registered: it is a `SELECT` looking the type up in the database, and in `AfterConnect` it would
run on every connection the pool ever opens. The work belongs to whichever change writes the first
vector query — add `github.com/pgvector/pgvector-go/pgx` and call `RegisterTypes` from the pool's
`AfterConnect` in `apps/api/internal/repository/db.go`, alongside the decimal codec already there.

## Retries, timeouts and the usage log

One policy, shared by every adapter, in `apps/api/internal/ai/retry.go`.

- **Retried:** rate limits, provider-side faults, connections that never produced a response, and
  answers that miss the schema. The wait doubles from `AI_RETRY_BACKOFF_SECONDS`.
- **Not retried:** a request the provider rejected, and a refusal. Repeating either only spends the
  allowance. Only the adapter knows which of its failures are transient, so it marks them and the
  shared loop reads the mark.
- **Timeouts are per attempt**, not per chain: `AI_LLM_TIMEOUT_SECONDS` and its two siblings cap one
  call, and `AI_MAX_ATTEMPTS` bounds how many there are.
- **The provider SDK's own retries are turned off.** Left on they would silently multiply every
  configured attempt — three configured attempts became nine real requests — and the attempt count
  in the log would be a third of the truth.

Every call is logged once, on success and on failure alike, with provider, model, operation,
attempt count, elapsed time and token counts. That is the pilot's cost measurement: a call that
failed after three attempts still cost something, so it is recorded too. Transcription reports no
token count, and logs zero rather than a guess.

When the attempts run out, the caller gets `domain.ErrAIUnavailable`, tagged `AI_UNAVAILABLE`,
which the handler layer answers as a `503`. A caller's own mistake — no schema, an empty recording,
a document type the provider does not read — is `ErrInvalidInput` instead, raised before any round
trip, so a bug of ours is never reported as a provider outage.

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
