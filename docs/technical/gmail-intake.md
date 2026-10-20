# Gmail intake

An administrator connects one Gmail or Google Workspace mailbox to each branch email channel in
branch settings. The Google account's primary address must match the branch email address. The
connection is only for incoming requests; outbound quote delivery uses the configured mail
transport and requires the seller's action.

## Google setup

Enable the Gmail API in a Google Cloud project, configure its OAuth consent screen, and create a web
application OAuth client. Register the public API callback URL exactly as
`GMAIL_OAUTH_REDIRECT_URL`, for example `https://api.example.com/v1/public/gmail/callback`.
For local testing Google supports `http://localhost:8000/v1/public/gmail/callback`.
For the DigitalOcean App Platform ingress in this repository, the public callback is
`https://<primary-domain>/api/v1/public/gmail/callback`: ingress removes `/api` before forwarding
the request to the API.
Set `GMAIL_OAUTH_CLIENT_ID`, `GMAIL_OAUTH_CLIENT_SECRET`, `GMAIL_OAUTH_REDIRECT_URL` and the
32-byte base64 `CHANNEL_CONFIG_ENCRYPTION_KEY` on the API. With all Gmail OAuth settings empty,
the connector is disabled. Keep the client secret and encryption key outside source control.

The connector requests `https://www.googleapis.com/auth/gmail.readonly`, a restricted Google scope.
Google's verification requirements and, where applicable, security assessment must be completed
before allowing external users to connect. See [Gmail API scope requirements](https://developers.google.com/workspace/gmail/api/auth/scopes)
and [Google OAuth for web applications](https://developers.google.com/identity/protocols/oauth2/web-server).
During testing, access depends on the consent screen's publishing status and authorized test users.
An external application in Google's Testing state receives refresh tokens that expire after seven
days for this scope; reconnect the mailbox from branch settings to continue the test. See Google's
[refresh token expiration rules](https://developers.google.com/identity/protocols/oauth2#expiration).

## Operation

The authorization uses a short-lived single-use state and PKCE. The refresh token is encrypted with
AES-256-GCM under `CHANNEL_CONFIG_ENCRYPTION_KEY`; credentials never reach the frontend and message
contents are not written to request logs. The branch address is checked again when authorization
finishes and each time an incoming message is persisted. Closing the channel or changing the
address stops the old mailbox from being routed to that branch.

The API polls connected mailboxes every `GMAIL_POLL_INTERVAL_SECONDS` (30 by default). It stores
the original RFC 5322 message, body and attachments before waking the existing attachment extraction
job. Unsupported or unreadable material is retained for seller review; the model never sends a
response. Each provider message is idempotent within its channel. A cursor is advanced only after
the whole page sequence is durable. When history expires, the connector lists inbox messages since
the connection and then resumes from a cursor captured before that listing, so concurrent arrivals
are not skipped. The same initial listing covers messages received during authorization.

Monitor the API log for `gmail mailbox synchronization failed`; a successful connection message
confirms the authorization completed, not that later synchronization is healthy. If authorization
expires or is revoked, reconnect the same branch mailbox. Its last durable cursor is retained so
synchronization can resume without reprocessing already recorded messages.

Every new inbox message is treated as a separate request, including unrelated mail and replies to
an existing conversation. Threading and request classification are not implemented; use a
dedicated intake mailbox and have a seller review its requests.

`GMAIL_REQUEST_TIMEOUT_SECONDS` limits each provider request. `GMAIL_MAX_RAW_BYTES`,
`GMAIL_MAX_JSON_BYTES` and `GMAIL_MAX_MIME_PARTS` bound message processing. A message larger than
the configured raw or response limit stops cursor advancement and remains in Gmail for a later
retry after the limit is adjusted. Individual attachments beyond `STORAGE_MAX_FILE_SIZE_BYTES`
remain stored for review but are not sent to extraction. The scheduled extraction job remains a
backstop if the API process stops between persistence and its wake-up.

In production, all API replicas need a shared durable object store and the same encryption key.
Deploy migrations before enabling the connector. Gmail authorization and live delivery require a
real Google account and project; automated tests use provider doubles.
