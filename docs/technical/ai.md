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
RFQ service validates the channel and stores the original text as `RECEIVED` before calling
the configured extractor. A provider failure therefore leaves a recoverable RFQ instead of
discarding the input. Complete extraction then creates the quote `DRAFT` and mutable v1 in a
second transaction. It does not price, discount, match against the catalog, or contact the
client.

The request's `channel_id` must identify an active channel owned by the authenticated account
and the branch selected through `X-Branch-Id`. Channel validation and RFQ persistence share
the first transaction and run before the external extractor call. Once received, processing
does not depend on the channel remaining active. `GET /v1/channels` lists the active channels
available for the selected branch; the list response intentionally excludes the provider
configuration stored in `channel.config`.

The Anthropic adapter uses forced tool use with a strict schema. It returns complete material
lines plus proposed clarification questions. Missing or unclear quantity, unit, presentation,
or product description is blocking: the model must not invent a default. The proposal is stored
in `rfq_clarification` with status `PROPOSED`; the RFQ remains `RECEIVED`, and no partial quote
is created. The response carries `quote: null`, `version: null`, and the reviewable
`clarifications`. A later outbound-channel flow lets the seller approve or edit a question;
the AI never sends it.

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
client label until client resolution is implemented. The quote stays unassigned because the
authenticated developer calling the mock is not the seller who claimed an inbound RFQ.

This endpoint does not emulate Meta webhook signatures, delivery events, or outbound messages,
and it is not mounted when `ENV=production`. It still uses the configured RFQ extractor: with
`AI_PROVIDER=disabled` it returns `422` after persisting the RFQ as `RECEIVED`; with an enabled
provider it follows the regular RFQ-to-quote draft or clarification flow.

OpenAI embedding and OCR adapters are not part of this text-extraction endpoint yet. Their API
key becomes required when the catalog-matching and attachment-ingestion tickets bind those
ports in the composition root.
