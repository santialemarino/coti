# WhatsApp Cloud API

Coti connects each branch's `WHATSAPP` channel to Meta's Cloud API. An inbound text message creates
an unassigned, reviewable RFQ draft; sending a seller-approved quote delivers its public webapp
link through the same branch channel. The AI never sends a message or approves a quote.

For how a teammate receives least-privilege Meta access, registers a personal test phone, and
tests this shared development integration, see [Meta App development access and phone testing](meta-app-development-testing.md).

## Configuration

The API needs one app-level secret and every active branch channel needs its own Meta number
configuration. Keep all values in the deployment secret store or the ignored `apps/api/.env` file;
do not place them in source control, a browser client, or a log.

| Setting                | Location         | Purpose                                                  |
| ---------------------- | ---------------- | -------------------------------------------------------- |
| `WHATSAPP_APP_SECRET`  | API environment  | Validates Meta's `X-Hub-Signature-256` header.           |
| `phone_number_id`      | `channel.config` | Routes inbound events and selects Meta's sending number. |
| `access_token`         | `channel.config` | Authorizes one outbound Meta API call.                   |
| `webhook_verify_token` | `channel.config` | Matches Meta's callback verification request.            |

`channel.config` is submitted through the admin channel API and is encrypted with
`CHANNEL_CONFIG_ENCRYPTION_KEY` before it reaches PostgreSQL. It never appears in API responses.
The channel's `identifier` is the human-facing number; Meta's `phone_number_id` is not a phone
number and belongs only in the configuration.

For a development test number, configure the active `WHATSAPP` channel with its number as
`identifier` and this JSON shape, substituting local secret values:

```json
{
  "phone_number_id": "META_PHONE_NUMBER_ID",
  "business_account_id": "META_WABA_ID",
  "access_token": "META_ACCESS_TOKEN",
  "webhook_verify_token": "LOCAL_RANDOM_VERIFY_TOKEN"
}
```

The application uses `WHATSAPP_GRAPH_API_VERSION` (default `v25.0`) and
`WHATSAPP_REQUEST_TIMEOUT_SECONDS` for outbound requests. Update the version deliberately when
Meta retires the configured Graph API version.

Each new branch already has an unconfigured `WHATSAPP` channel. An authenticated administrator
finds it with `GET /v1/channels`, then configures it with
`PUT /v1/channels/{channelId}` and the selected branch's `X-Branch-Id` header. Include the
human-readable number in E.164 form as `identifier` whenever a configuration is saved:

```json
{
  "identifier": "+15551234567",
  "is_active": true,
  "config": {
    "phone_number_id": "META_PHONE_NUMBER_ID",
    "business_account_id": "META_WABA_ID",
    "access_token": "META_ACCESS_TOKEN",
    "webhook_verify_token": "LOCAL_RANDOM_VERIFY_TOKEN"
  }
}
```

There is no channel-settings screen in the backoffice yet; this guarded administrator API is the
configuration surface until that UI is built. Its response confirms only `is_configured`, never the
credentials.

## Meta callback

Set Meta's callback URL to:

```text
https://<api-host>/v1/webhooks/whatsapp
```

The `GET` callback verification accepts the configured `webhook_verify_token`; the `POST` endpoint
requires an HMAC-SHA256 signature made with `WHATSAPP_APP_SECRET`. Subscribe the Meta app to the
`messages` webhook field. A deployment with no app secret returns `503` for inbound deliveries
instead of accepting an unsigned message.

The callback is public but not tenant-authenticated. It resolves `metadata.phone_number_id` on the
owner pool, then opens the tenant transaction for the exact account, branch, and channel. An
unconfigured destination is acknowledged without creating tenant data.

Only inbound text messages are materialized in this release. Their Meta message ID is retained in
`inbound_channel_message` with a unique `(channel_id, external_message_id)` constraint, a reserved
RFQ ID, the original provider payload, and sender metadata. A retry resumes that RFQ; it cannot
create a second one. Media and delivery-status events are acknowledged but do not create RFQs.

When a seller opens the send dialog for an RFQ that entered through WhatsApp, Coti reads that
retained sender and pre-fills it as the editable WhatsApp recipient. Meta supplies it as a
digits-only `wa_id`, which Coti exposes to the UI in E.164 form. This is channel-delivery context,
not a client write: an inbound message neither creates nor updates a `client` record.

## Outbound quote delivery

`POST /v1/quotes/{quoteId}/sends` already creates a durable `quote_send` before contacting a
provider. For the selected branch's active WhatsApp channel, Coti opens that channel's encrypted
credentials only in memory, posts a text message with the public quote link to Meta, and stores
Meta's message ID as `provider_reference`. If Meta rejects the call, the individual delivery is
recorded as `FAILED`; an email copy, when selected, still runs independently.

Use a dedicated test recipient added in Meta while the app is in development. A production number
requires Meta's production registration and the usual business verification; the integration code
does not make an unregistered number sendable.

Meta's development recipient allow-list can reject an Argentine inbound `wa_id` (`549...`) even
when it accepts the same test recipient without the mobile prefix (`54...`). In development only,
when Meta responds with the specific recipient-not-allowed code `131030`, Coti retries once with
that alternate representation. The inbound default and production sends retain the canonical
`+549...` value; the fallback is not enabled in production.
