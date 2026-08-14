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
RFQ service validates the source, asks the configured extractor for complete material lines,
and atomically stores the original text with a quote `DRAFT` and mutable v1 version. It does
not price, discount, match against the catalog, or contact the client.

The request's `channel_id` must identify an active channel owned by the authenticated account
and the branch selected through `X-Branch-Id`. Channel validation runs before the external
extractor call. `GET /v1/channels` lists the active channels available for that selected branch;
the channel is checked again inside the persistence transaction in case it was deactivated
during extraction. The list response intentionally excludes the provider configuration stored
in `channel.config`.

The Anthropic adapter uses forced tool use with a strict schema. It extracts only material
lines whose quantity is explicit or directly computable from the message. After a successful
draft, incomplete lines remain recoverable through `rfq.raw_text`; clarification persistence
is intentionally not implemented until the product model for clarification requests is closed.

## Development WhatsApp Intake

Outside production, `POST /v1/dev/whatsapp/messages` simulates one inbound WhatsApp text
message while reusing the production RFQ service. The route is authenticated and requires an
active `X-Branch-Id` header.

```json
{
  "from": "+5491155551234",
  "profile_name": "Juan Perez",
  "text": "Necesito 20 bolsas de cemento y 2 m3 de arena fina"
}
```

The service selects the branch's only active WhatsApp channel. When a branch has multiple
active WhatsApp channels, send the chosen channel as `channel_id`; an inactive channel or one
from another branch is rejected. The sender name and number are retained as the draft's loose
client label until client resolution is implemented.

This endpoint does not emulate Meta webhook signatures, delivery events, or outbound messages,
and it is not mounted when `ENV=production`. It still uses the configured RFQ extractor: with
`AI_PROVIDER=disabled` it returns `422` and persists no draft; with an enabled provider it follows
the regular RFQ-to-quote draft flow.
