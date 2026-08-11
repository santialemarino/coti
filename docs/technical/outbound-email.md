# Outbound email

Every message the system sends goes through one port, and every delivery attempt leaves one
record. The transport is a startup decision; nothing above the port knows which provider is
behind it.

## The port and its adapters

`domain.Mailer` is the port — one method, `Send(ctx, EmailMessage)`. Adapters live in
`apps/api/internal/mail/`, and `cmd/api/main.go` is the only place one is bound.

| Provider  | Behaviour                                                                      |
| --------- | ------------------------------------------------------------------------------ |
| `console` | Writes the message to the log, body included, and reports success              |
| `smtp`    | Delivers over SMTP, with STARTTLS and authentication where the server has them |

`console` is the default. Its log line is deliberately the whole body: with no provider behind
it, that is the only place a recovery link can be read. It is also the reason
`AUTH_REQUIRE_VERIFIED_EMAIL` cannot be turned on beside it — a transport that writes to a log
cannot deliver the link it would then demand. Selecting `smtp` is what unblocks that flag.

Selecting `smtp` with a credential missing fails at startup, every blank key named in the same
pass rather than one restart per problem.

### What the SMTP adapter sends

One message, `multipart/alternative`, the plain-text part first — a reader that renders a single
alternative takes the last one it understands, so the order is what decides whether an HTML
client shows the HTML. Both parts are `charset=utf-8` and `quoted-printable`; the subject and the
two display names are RFC 2047 encoded words. None of that is decoration: the copy is Argentine
Spanish, and a transport that assumes ASCII is how "Confirmá tu dirección" arrives mangled.

The sender is `MAIL_FROM_NAME` + `MAIL_FROM_ADDRESS`. The credentials are **one account per
environment**, read from configuration when the adapter is constructed — the whole installation
sends as Coti, not each corralón from its own server.

**TLS is declared, not negotiated.** With `MAIL_SMTP_STARTTLS` on, a server that does not
advertise STARTTLS fails the send instead of being fallen back on, because a stripped
advertisement is otherwise indistinguishable from a server that never had it — and the message
and the password behind it would cross the network in the clear. Authentication then happens only
where the server asks for it, which is what lets the same adapter talk to a sandbox that wants no
credentials at all.

`MAIL_SMTP_TIMEOUT_SECONDS` bounds the dial and the conversation after it, and a cancelled
request context closes the connection under an exchange already in flight: `net/smtp` takes no
context and cannot be interrupted any other way.

### Reading mail in development

`docker-compose.yml` runs [Mailpit](https://mailpit.axllent.org/), which accepts everything and
delivers nothing. `pnpm dev:docker` starts it with the rest of the stack, and the dockerized API
already points at it. Against a host API (`pnpm dev`), start it on its own and switch the
provider:

```bash
docker compose up -d mailpit
# apps/api/.env — the MAIL_SMTP_* keys are already pointed at it
MAIL_PROVIDER=smtp
```

Messages land at **http://localhost:8025**. Mailpit speaks plain SMTP on 1025, so
`MAIL_SMTP_STARTTLS=false` is what talks to it — the one case that setting exists for.

## What the service does

`services.MailService.Send` is the whole flow:

1. Reads the account inside its tenant transaction, for the brand.
2. Renders the layout with that brand.
3. Hands the message to the transport — **outside** any transaction, because it is off-process.
4. Writes the `notification` row with the outcome: `SENT` plus a `sent_at`, or `FAILED` with
   `sent_at` left null.

**A delivery failure is recorded and does not fail the operation that caused it.** `Send`
returns the transport's error so a caller can react, and the callers whose own work must
survive a bounce log it and carry on — an administrator who triggers a password reset still
gets a 204 when the provider is down, and the notification row says what happened.

## Templates

One layout, in `internal/services/mail_templates.go`: a branded header, a heading, some
paragraphs and at most one call to action. A new kind of message is a caller filling in
`OutboundMail`, not a new template — which is what let address verification ship without
touching this file.

Both single-use links share one issuer, `internal/services/auth_link.go`: retire the
outstanding ones, mint one, mail it, and resolve a presented one. Password recovery and
address verification differ only in the token type, the lifetime, the route and the copy.

The brand comes from the account — `name`, `brand_logo_url`, `brand_color`, the same pair the
client webapp renders a quote with. **The colour is pattern-checked before it reaches the
stylesheet.** `html/template` blanks any value it cannot prove is a CSS token, so an unchecked
one renders as no colour at all rather than as the account's; anything that is not a hex
literal falls back to the default.

Every user-facing string lives in `internal/services/mail_copy.go`, in Argentine Spanish. It is
the API's counterpart to the web apps' `es-AR` catalog and the only Spanish in the backend — new
copy goes there rather than inline at the call site.

## The delivery record

`notification` carries `account_id`, an optional `user_id` / `client_id` / `quote_id`, an
`event`, a `medium`, a `status` and `sent_at`. It is append-only and account-scoped like every
other tenant table.

Today the events are `PASSWORD_RESET` and `EMAIL_VERIFICATION`, both over `EMAIL`. The quote
magic link and the follow-up messages land here as they are built.

## Nothing sends on its own

There is no scheduler and no background sender. Every message is the direct consequence of an
action a seller or the user themselves took, which is what keeps the AI pipeline from being able
to contact a client.

## Configuration

In `apps/api/.env.example`, defaults in `internal/config`:

| Key                         | Meaning                                                       |
| --------------------------- | ------------------------------------------------------------- |
| `MAIL_PROVIDER`             | `console` (default) or `smtp`                                 |
| `MAIL_FROM_ADDRESS`         | sender; required unless the provider is `console`             |
| `MAIL_FROM_NAME`            | display name on the sender                                    |
| `MAIL_SMTP_HOST`            | required when the provider is `smtp`                          |
| `MAIL_SMTP_PORT`            | defaults to 587                                               |
| `MAIL_SMTP_USERNAME`        | required when the provider is `smtp`                          |
| `MAIL_SMTP_PASSWORD`        | required when the provider is `smtp`                          |
| `MAIL_SMTP_STARTTLS`        | require STARTTLS; defaults to true, false only for a sandbox  |
| `MAIL_SMTP_TIMEOUT_SECONDS` | bounds the dial and the conversation; defaults to 10          |
| `WEB_BACKOFFICE_URL`        | base of the links the API mails; validated as an absolute URL |

An unknown provider, a missing sender or a missing credential is reported at startup alongside
every other configuration problem, each key named.
