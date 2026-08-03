# WhatsApp Cloud API

Coti receives WhatsApp Business messages through the Meta Cloud API webhook and persists
each accepted text message before the RFQ pipeline runs. The current implementation covers
inbound text messages; media payloads and outbound quote delivery are the next connector
steps.

## Routes

Meta calls the public webhook under:

```text
GET  /v1/public/webhooks/whatsapp
POST /v1/public/webhooks/whatsapp
```

`GET` is Meta's subscription challenge. It succeeds only when `hub.mode=subscribe` and
`hub.verify_token` matches `WHATSAPP_WEBHOOK_VERIFY_TOKEN`, then returns `hub.challenge` as
plain text.

`POST` accepts Cloud API deliveries. The API requires `WHATSAPP_APP_SECRET` and validates
`X-Hub-Signature-256` before parsing the body. If the signature is missing or invalid, the
request is rejected before it reaches the ingest service.

## Environment

```text
WHATSAPP_WEBHOOK_VERIFY_TOKEN=
WHATSAPP_APP_SECRET=
```

The verify token is any private random value entered in both Coti and the Meta webhook
configuration. The app secret is the Meta App Secret used to validate delivery signatures.

## Channel mapping

The webhook payload gives Coti a `metadata.phone_number_id`. That value maps to a row in
`channel.identifier`:

```sql
INSERT INTO channel (account_id, branch_id, type, identifier, config)
VALUES (
  '<account-id>',
  '<branch-id>',
  'WHATSAPP',
  '<meta-phone-number-id>',
  '{"display_phone_number":"+54 9 11 1234-5678"}'::jsonb
);
```

Only active WhatsApp channels are accepted. If no channel matches the received
`phone_number_id`, the delivery is acknowledged but ignored; Meta should not retry because
the payload is well-formed but not configured for this tenant.

## Persistence flow

For every text message in the webhook payload:

1. Resolve the WhatsApp channel by `phone_number_id` on the owner connection, because the
   tenant is not known yet.
2. Open a tenant-scoped transaction for the channel's account.
3. Insert `inbound_channel_message` with the Meta `messages[].id` as `external_message_id`.
4. On duplicate `(channel_id, external_message_id)`, stop without creating another RFQ.
5. Insert `rfq` with `raw_text` set to the message body and `client_label` set from the
   WhatsApp profile/name and sender id.
6. Link `inbound_channel_message.rfq_id` to the created RFQ.

This keeps the original message before processing and makes webhook retries idempotent.

## Local testing

Use an HTTPS tunnel that forwards to the API, then register the public URL in Meta:

```text
https://<tunnel-host>/v1/public/webhooks/whatsapp
```

For a local signed POST, compute `X-Hub-Signature-256` as:

```text
sha256=<hex_hmac_sha256(raw_body, WHATSAPP_APP_SECRET)>
```

The test payload must include:

```json
{
  "entry": [
    {
      "changes": [
        {
          "field": "messages",
          "value": {
            "metadata": { "phone_number_id": "<meta-phone-number-id>" },
            "contacts": [{ "wa_id": "5491112345678", "profile": { "name": "Test Client" } }],
            "messages": [
              {
                "from": "5491112345678",
                "id": "wamid.unique",
                "timestamp": "1785614400",
                "type": "text",
                "text": { "body": "I need 20 cement bags" }
              }
            ]
          }
        }
      ]
    }
  ]
}
```

## Meta development-mode validation

Meta apps that have not been published can verify a webhook endpoint and send dashboard test
events, but they may not deliver real user-originated webhook data. The Meta dashboard shows
this as a warning in the webhook configuration screen: while the app is unpublished, webhook
events are limited to tests sent from the app dashboard.

Observed local validation on 2026-08-03:

1. The callback URL accepted Meta's `GET` subscription challenge with the configured verify
   token.
2. A dashboard `messages` test delivered a signed `POST`; signature validation succeeded and
   the sample text payload was parsed correctly.
3. A real WhatsApp message sent to the Meta test phone number did not reach the webhook while
   the app remained unpublished.

For end-to-end persistence, the local API must run against PostgreSQL with `pgvector`, the
`00008_inbound_channel_message.sql` migration must be applied, and a `WHATSAPP` channel row
must exist with `channel.identifier` set to the Meta `phone_number_id`.

## Outbound quote delivery

Outbound WhatsApp delivery should be wired when the quote-send service exists. It must use
the same `channel` row for routing and create the corresponding `quote_send` and
`notification` records. Inside WhatsApp's 24-hour customer-service window, Coti can send a
regular response message; outside that window it must send an approved Meta template.
