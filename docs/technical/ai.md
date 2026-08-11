# AI Providers

Coti binds AI-backed behavior through domain ports in `apps/api/internal/domain` and
provider adapters in `apps/api/internal/ai`. Services depend on the port, not the adapter.

## Configuration

The API starts with RFQ extraction disabled unless a provider is selected:

```dotenv
AI_PROVIDER=disabled
AI_ANTHROPIC_API_KEY=
AI_ANTHROPIC_BASE_URL=https://api.anthropic.com
AI_ANTHROPIC_VERSION=2023-06-01
AI_RFQ_EXTRACTOR_MODEL=claude-sonnet-4-6
AI_RFQ_EXTRACTOR_TIMEOUT_SECONDS=20
AI_RFQ_EXTRACTOR_MAX_TOKENS=1024
```

Set `AI_PROVIDER=anthropic` and provide `AI_ANTHROPIC_API_KEY` to enable the RFQ text
extractor. Startup validation rejects an Anthropic provider without a key, an invalid base
URL, non-positive timeout, or non-positive token budget.

## RFQ Text Extraction

`POST /v1/rfqs/text-drafts` accepts authenticated plain-text RFQs for an active branch. The
handler stores the original text through the RFQ service, asks the configured extractor for
complete material lines, and creates a quote `DRAFT` with a mutable v1 version. It does not
price, discount, match against the catalog, or contact the client.

The Anthropic adapter uses forced tool use with a strict schema. It extracts only material
lines whose quantity is explicit or directly computable from the message. Incomplete lines
remain recoverable through `rfq.raw_text`; clarification persistence is intentionally not
implemented until the product model for clarification requests is closed.
