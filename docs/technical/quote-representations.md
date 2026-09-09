# Quote representations

The backend generates one immutable bundle per approved quote version: a canonical JSON
snapshot, a commercial message template and a branded PDF. No frontend is included here.

## Approval and generation

`POST /v1/quotes/{quoteId}/representations` requires authentication and `X-Branch-Id`.
It takes no body or idempotency header. It returns 201 on creation and 200 on replay.
Only an unarchived `QUOTED` quote can generate a new bundle; `SENT` permits an existing
bundle only. Other lifecycle states return 409.

The service locks the quote and current version, validates all lines, approved alternatives,
currency and persisted totals, and freezes the version in one tenant transaction. Invalid
sources return 422 with `QUOTE_REPRESENTATION_INVALID` and an `issues` list, without freezing.
Seller item changes and alternative approvals reject immutable versions.

`PATCH /v1/quotes/{quoteId}/items/{itemId}/alternatives/{alternativeId}` accepts
`{"approved_by_seller": true}` or `false` while the current `QUOTED` version is mutable.
Only priced candidates can be approved. Unapproved alternatives never enter the public bundle.
Alternative product prices are snapshotted in a batch during materials acceptance; candidates
with a different currency stay unpriced. Mixed currencies among quote lines are rejected.

PDF rendering, logo retrieval and storage run outside transactions. A session advisory lock
serializes generation for the account and quote across instances. Rendering or upload failure
returns 503, leaves the quote `QUOTED` and frozen, and permits retry. No client is contacted.
The representation row is inserted only after upload succeeds. A database failure after upload
may leave an unreferenced object; retry uses the same key and may overwrite that object.

## Snapshot and storage

`quote_representation` is unique on `(account_id, version_id)`. Composite foreign keys bind
the account, branch, quote and version; RLS enforces the account boundary. The application
role can insert and read representations, but cannot update or delete them.

The PDF key is `accounts/{account}/quotes/{quote}/versions/{version}/quote.pdf`.
The row stores its content type, size and SHA-256 alongside schema version 1, creation time,
logo fallback indicator, canonical JSON and message template. Existing bundles are never
rebuilt from live catalog data. Money and quantity fields are decimal strings.

Names, codes and institutional identity are read when the bundle is generated. Quantities,
prices, discounts and totals come exclusively from persisted version snapshots. The payload
contains supplier identity, branch, customer name, reference, version, approval timestamp,
currency, lines, approved alternatives, discounts and total. It excludes contact details,
internal comments, AI signals, price floors and unapproved alternatives.

Migration 00019 assigns existing quotes account-local numbers ordered by creation time and
ID. New numbers use a transactional account counter, so rolled-back creations consume none.
References render as `COT-000001`. Legacy versions receive currency `ARS`; legacy immutable
versions use their creation timestamp as the unavailable historical freeze timestamp.

## Public access and delivery

Generation itself creates no public token, sends nothing and triggers no quality evaluation.
Delivery ensures the bundle before contacting either provider and checks its version again
inside preparation. `quote_send.format` remains `WEBAPP_LINK`. WhatsApp receives the stored
message with its channel-specific URL; email receives the snapshot reference, total and URL.
No PDF is automatically attached. Existing delivery idempotency, independent channels and
post-commit quality evaluation are retained.

`GET /v1/public/quote-sends/{token}` accepts only completed successful sends. Unknown, pending
and failed tokens return 404. Account resolution uses the owner pool; content reads use RLS.
Active responses contain `status`, `expires_at`, `quote`, `message` and `pdf_url`; expired
responses contain only `status` and `expires_at`, with HTTP 200. Responses use `no-store`.
Reads neither mark the send viewed nor run evaluation.

PDF URLs use the existing ObjectStorage signer, bounded by its configured lifetime and the
remaining delivery validity (rounded down to seconds). Already downloaded PDFs cannot be
revoked; the document directs the reader to the public link for current validity. It embeds
neither a token nor an absolute expiry. Authenticated generation returns a signed PDF URL and
a message preview containing exactly one `{{public_url}}` marker, replaced only at delivery.

## Branding and rendering

The native Go renderer uses A4 pages and embedded Go Unicode fonts. Brand color is an accent,
not a text background. Tables repeat their headings and long rows continue across pages.
The logo preserves its aspect ratio; missing or rejected logos fall back to supplier text.

Logo requests require HTTPS on port 443 without credentials, proxies or cookies. Every dial
validates resolved addresses and connects to a validated IP; redirects are revalidated.
Only decoded PNG/JPEG images within both byte and pixel limits are accepted. Configuration:

| Variable                           | Default  |
| ---------------------------------- | -------- |
| `QUOTE_LOGO_FETCH_TIMEOUT_SECONDS` | 3        |
| `QUOTE_LOGO_MAX_SIZE_BYTES`        | 2097152  |
| `QUOTE_LOGO_MAX_PIXELS`            | 12000000 |
| `QUOTE_LOGO_MAX_REDIRECTS`         | 3        |

## Verification

Run `pnpm check`, `pnpm test:api`, and the PostgreSQL integration suite with both test database
URLs configured. Representation and delivery integration tests use real persistence and local
ObjectStorage with the native PDF renderer. To inspect pagination, set
`TEST_QUOTE_PDF_OUTPUT_DIR` to an existing temporary directory and run
`go test ./internal/pdf` from `apps/api`; render the resulting PDF with Poppler.
