# WhatsApp Cloud API

Coti connects each branch's `WHATSAPP` channel to Meta's Cloud API. An inbound text message either
continues the conversation of a live quote sent to that sender or creates an unassigned, reviewable
RFQ draft; sending a seller-approved quote delivers its public webapp
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
`inbound_channel_message` with a unique `(channel_id, external_message_id)` constraint, the original
provider payload, sender metadata, and exactly one target: a reserved RFQ ID or the
`quote_message` the text became (see [Conversation routing](#conversation-routing)). A retry keeps
the first delivery's target — it resumes that RFQ, or does nothing for a message already joined —
so it can create neither a second order nor a second message. Media and delivery-status events are
acknowledged but do not create RFQs.

## Conversation routing

Before a message becomes an order, `ConversationService.Route` checks whether its sender is
answering a quote. Within the branch the channel resolves to, a candidate is a quote whose latest
successful `quote_send` on **the same channel** went to the sender, where that send has not expired
and the quote is `SENT` or `CHANGE_REQUESTED` and not archived. `client.phone` plays no part: the
send's destination is the address the quote actually reached.

- **The sender's forms.** Meta's `wa_id` is digits only, and an Argentine mobile is written both
  with and without the 9 after `54`; `domain.InboundSenderDestinations` returns `+<wa_id>` plus the
  other Argentine form, and either matches a destination.
- **The newest send wins.** Several live quotes for one sender route to the one sent last; the
  stored `quote_message.other_live_quotes` counts the rest, so the seller can be told the message
  may belong to another quote.
- **Locks.** The candidate read is unlocked, so each candidate is locked (`SELECT … FOR UPDATE`, the
  same row lock the edit paths take first) and re-checked in recency order; one a seller accepted or
  archived meanwhile is skipped.
- **No match** creates the RFQ exactly as before.

A joined message is written in one tenant transaction with its reservation: the `CLIENT`
`quote_message`, its window, and the `inbound_channel_message` pointing at it. The router takes a
resolved route and a sender, not a WhatsApp payload, so an email inbox can reuse it once
`InboundSenderDestinations` learns that channel.

### Message windows

A `CLIENT` message opens the quote's `OPEN` `message_batch` or extends it, in one
`INSERT … ON CONFLICT` on `uq_message_batch_open`, so concurrent replies share one window:

```text
closes_at = min(now + CONVERSATION_WINDOW_RESET_SECONDS, opened_at + CONVERSATION_WINDOW_MAX_SECONDS)
```

`SELLER` and `SYSTEM` messages are context and never open or extend a window. Defaults are 120 s
and 600 s; startup refuses a cap that does not exceed the reset. `CONVERSATION_CONTEXT_MESSAGES`
(10) is how many earlier messages of any author travel with a batch as context. Routing never
changes `quote.current_status`.

### Closing and processing batches

A sweep closes the due windows and works the queue, across every account on the owner pool:

1. **Close.** Each page locks up to `CONVERSATION_SWEEP_BATCH_SIZE` quotes with a window due
   (`closes_at <= now`), skipping a quote another transaction holds, then in the same transaction
   moves their windows to `CLOSED` (`closed_at` from the database's clock, so instance skew cannot
   reorder the queue) and stamps every unbatched `CLIENT` message of the
   quote with the batch. The quote is locked before its window, the order routing follows too, so a
   message either lands before the close and joins the batch or after it and opens the next window.
2. **Reclaim.** A `PROCESSING` batch whose `processing_started_at` is older than
   `CONVERSATION_PROCESSING_TIMEOUT_SECONDS` plus one ticker interval lost its worker: it counts
   the attempt and returns to `CLOSED`, or `FAILED` once `attempts` reaches
   `CONVERSATION_MAX_ATTEMPTS`.
3. **Process.** One statement claims the oldest `CLOSED` batch that heads its quote's queue by
   `(closed_at, id)` on a quote with no `PROCESSING` batch (`FOR UPDATE SKIP LOCKED`), and
   `uq_message_batch_processing` refuses a second claim on one quote should two workers race (the
   loser moves on to the next batch). The
   claimed batch goes to the `ConversationBatchHandler` port with one attempt's timeout, then
   `PROCESSED`; an error counts an attempt and returns it to `CLOSED` (`last_error`), or `FAILED` at
   the limit, and the quote's next batch waits behind it until then. Each batch gets one attempt
   per pass, so retries are spaced by the cadence. A shutdown hands the claim back uncounted.

Every write that settles a claim matches its `processing_started_at`, so a worker that was reclaimed
and comes back late changes nothing; those writes outlive a shutdown but are bounded at 10 s. The
handler owns every quote write a batch causes, the `SENT` → `CHANGE_REQUESTED` move included, and
its transactions take `MessageBatchRepository.LockClaim` right after the quote's lock: a lost claim
is refused with a conflict, and a claim it holds cannot be reclaimed until it commits. Both binaries
bind the batch change handler ([rfq-pipeline.md](rfq-pipeline.md#conversation-batch-handler)). A
handler error wrapping `domain.ErrBatchUnprocessable` — the quote was archived or closed after its
window closed — spends the batch's last attempt at once and leaves it `FAILED` with the reason in
`last_error` and `failure_reason = UNPROCESSABLE`; one that ran out of attempts is
`ATTEMPTS_EXHAUSTED`, the only kind the seller can send back to the queue. The webapp's «Pedir cambios» enters the same queue as a batch closed in its own
transaction.

The API runs the sweep on an in-process ticker every `CONVERSATION_TICKER_SECONDS` (30), one pass at
a time, stopped and awaited on shutdown before the pools close. The `conversation-batches`
scheduled job runs the same sweep every 15 minutes as a backstop
([scheduled-jobs.md](scheduled-jobs.md)); the two may overlap safely.

When a seller opens the send dialog for an RFQ that entered through WhatsApp, Coti reads that
retained sender and pre-fills it as the editable WhatsApp recipient. Meta supplies it as a
digits-only `wa_id`, which Coti exposes to the UI in E.164 form. This is channel-delivery context,
not a client write: an inbound message neither creates nor updates a `client` record.

## Outbound quote delivery

`POST /v1/quotes/{quoteId}/sends` already creates a durable `quote_send` before contacting a
provider. For the selected branch's active WhatsApp channel, Coti opens that channel's encrypted
credentials only in memory, posts a text message with the public quote link to Meta, and stores
Meta's message ID as `provider_reference`. If Meta rejects the call, the individual delivery is
recorded as `FAILED`; email, when selected, still runs independently. The seller can also send
solely by email without selecting a WhatsApp destination.

Use a dedicated test recipient added in Meta while the app is in development. A production number
requires Meta's production registration and the usual business verification; the integration code
does not make an unregistered number sendable.

Meta's development recipient allow-list can reject an Argentine inbound `wa_id` (`549...`) even
when it accepts the same test recipient without the mobile prefix (`54...`). In development only,
when Meta responds with the specific recipient-not-allowed code `131030`, Coti retries once with
that alternate representation. The inbound default and production sends retain the canonical
`+549...` value; the fallback is not enabled in production.
