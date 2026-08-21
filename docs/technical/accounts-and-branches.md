# Accounts and branches

An account is one corralón and the tenant boundary every other table hangs off. A branch is an
operating location under it. Registration creates both, plus the first administrator, in one
call.

## Registration

`POST /v1/public/accounts` is the only write that runs with no tenant. There is no account
yet, so there is no scope for row level security to read — it therefore runs on the owner pool
through `AdminTx`, and it is the only caller that legitimately does.

Everything it creates lands in one transaction:

1. the `account`,
2. its first `branch`,
3. that branch's `MANUAL_ENTRY` channel,
4. the administrator, with the password hashed.

The response carries a token pair, so the caller has a session without a second round trip.

**The administrator's address must be free across every account**, because login resolves a
user by email alone and an address therefore has to identify exactly one row.
`uq_app_user_email_global` on `lower(email)` enforces that; registration also checks it inside
its own transaction so the caller gets a precise `409` instead of a bare constraint violation.

## The manual-entry channel is not optional

Every branch has one, created with the branch. `rfq.channel_id` is `NOT NULL`, and a counter,
phone or unintegrated-messaging order has no other channel to point at — so a branch without it
cannot take the most common order in the business. Both paths that create a branch open it in
the same transaction as the branch itself.

## Branches

| Route                            | Who                      |
| -------------------------------- | ------------------------ |
| `GET /v1/branches`               | any authenticated caller |
| `POST /v1/branches`              | admin                    |
| `PUT /v1/branches/{branchId}`    | admin                    |
| `DELETE /v1/branches/{branchId}` | admin                    |

Reading is not admin-only because the branch switcher needs the list before it can send
`X-Branch-Id`; the query already narrows a seller to their assignments.

**`GET` answers two different questions, and keeping them apart is load-bearing.** By default it
returns the branches the caller may _operate in_ — active, and assigned unless they are an admin —
which is what the switcher and `X-Branch-Id` are validated against. With `include_inactive` it
returns every branch the account _has_, closed ones included, which is for administering them: the
read takes no user id at all, and the service refuses it to a seller with a **403**. A closed branch
reaching the first list would let a session pin itself to a branch every subsequent request refuses.

`DELETE` deactivates rather than removes, so the quotes and prices that reference the branch
stay explainable. **Closing the account's last active branch is refused** — an account with no
branch cannot take an order. The count runs inside the caller's transaction, so two concurrent
closes cannot both pass the check.

**Reopening is a `PUT` with `is_active` back to true.** Since `PUT` replaces the record it also
carries the name and expiry, and the last-active guard does not apply — it only runs when the flag
is being turned off. A closed branch stays fetchable by id, which is what keeps a quote that came in
through it explainable, but it is absent from the switcher and refused by the branch-access check.

Omitting `default_expiry_days` on create takes `BRANCH_DEFAULT_EXPIRY_DAYS`. It lives on the
branch, not the account, because tolerance to inflation differs between locations.

## Channels

A channel is an intake route on one branch — the way an order gets in, and the way a quote goes
back out. Four types, the `channel_type` enum: `WHATSAPP`, `EMAIL`, `WEBAPP`, `MANUAL_ENTRY`.

| Route                             | Who                      |
| --------------------------------- | ------------------------ |
| `GET /v1/channels`                | any authenticated caller |
| `POST /v1/channels`               | admin                    |
| `PUT /v1/channels/{channelId}`    | admin                    |
| `DELETE /v1/channels/{channelId}` | admin                    |

Every route is branch-scoped and reads the branch from `X-Branch-Id`; a request without one is a 422. Reading is not admin-only because a text draft has to name the channel its order arrived
through — `rfq.channel_id` is `NOT NULL` — so every seller needs the list. `include_inactive`
returns the closed ones too, for administering them rather than taking an order through one, and is
refused to a seller with a **403**.

`DELETE` deactivates rather than removes: `rfq`, `quote_send` and `quote_message` all reference
`channel(id)`, so the orders that arrived through it stay explainable. **Closing a branch's
manual-entry channel is refused** — a counter, phone or unintegrated-messaging order has no other
route to point at — and the guard sits on the `is_active` flag as well, or `PUT` would be a way
around `DELETE`. Reopening is a `PUT` with `is_active` back to true.

The type cannot change, because the shape of the configuration depends on it.

### The identifier is a column, and stays one

A branch may hold more than one channel of the same type — two WhatsApp numbers, two mailboxes — so
`channel.identifier` carries the number or the mailbox — **it is the only place either is written** —
and uniqueness is `(branch_id, type, identifier)`. A unique constraint does not compare NULLs, so a partial index
(`uq_channel_branch_type_no_identifier`) holds the identifier-less case to one per type per branch.
Both answer **409**.

`WEBAPP` and `MANUAL_ENTRY` are one per branch, so an identifier on either is refused with
`CHANNEL_IDENTIFIER`. An `EMAIL` identifier must be a bare address, because for a mail channel it
_is_ the mailbox: a malformed one guarantees the connector fails, and a display-name form
(`Pedidos <a@b>`) beside the plain one would be two channels on one mailbox, since uniqueness is on
the column verbatim. A `WHATSAPP` number is left alone — there is no one format to hold it to, and
the provider will reject what it does not like. `WHATSAPP` and `EMAIL` may carry one or go without — the partial index allows
one of each without, which is what every channel created before this route looks like. A blank
identifier is normalized to absent: an empty string is not NULL and would slip past that index. A
control character anywhere in it is refused — trimming only reaches the ends, and an embedded newline
is not part of any number or mailbox.

**The identifier is never duplicated inside `config`.** It is a column, channel uniqueness rests on
it, and a second copy is a second thing to keep in step.

### The configuration has a declared shape per type

`channel.config` is `JSONB`, and what may be in it is a closed set of fields per type. An unknown
field is refused rather than stored: this is where credentials live, and a free-form object there
becomes a dump nobody can reason about. The shapes are Go structs in
`internal/domain/channel_config.go`; the API does not publish them.

| Type           | Field                  | Credential | Required |
| -------------- | ---------------------- | ---------- | -------- |
| `WHATSAPP`     | `phone_number_id`      | no         | yes      |
| `WHATSAPP`     | `business_account_id`  | no         | no       |
| `WHATSAPP`     | `access_token`         | **yes**    | yes      |
| `WHATSAPP`     | `webhook_verify_token` | **yes**    | no       |
| `EMAIL`        | `smtp_host`            | no         | yes      |
| `EMAIL`        | `smtp_port`            | no         | yes      |
| `EMAIL`        | `smtp_username`        | no         | yes      |
| `EMAIL`        | `smtp_password`        | **yes**    | yes      |
| `EMAIL`        | `smtp_starttls`        | no         | no       |
| `WEBAPP`       | —                      |            |          |
| `MANUAL_ENTRY` | —                      |            |          |

**Neither shape names the endpoint the credentials are for**, because that is the identifier column:
the WhatsApp fields are the provider's own references for the number, not the number, and a mail
channel carries only what sends from its mailbox. `smtp_starttls` is declared rather than negotiated,
the way `MAIL_SMTP_STARTTLS` is: a server that stops advertising STARTTLS fails the send instead of
quietly downgrading to plaintext. A readable field is bounded at 255 bytes and a credential at 4096;
the check runs on the plaintext, before sealing, and nothing revalidates a stored config.

Validation of the shape and of the identifier both run before the credentials are sealed, so a
malformed request is answered as a malformed request even on a deployment that could not have stored
it anyway.

The two rules meet in the other direction too: **a channel that holds a configuration holds its
identifier**, since credentials belong to one number or one mailbox. That is checked against what the
channel will end up holding, not against what the request sent — otherwise a `PUT` that omits the
identifier while the stored configuration stays would leave those credentials pointing at nothing.

**An absent configuration is valid for every type.** A channel exists before its credentials do, and
absent, `null` and `{}` all mean the same thing — stored as SQL `NULL`. So "is this channel
configured" is a different question from "is this channel valid", and only the second is validated
here. A configuration that is present but does not match the type is a **422** with
`CHANNEL_CONFIG_SHAPE`, at the moment the channel is saved rather than when the first message
arrives.

`PUT` **replaces the whole configuration** when one is sent, leaves the stored one alone when the
field is absent, and removes it on an explicit `null`. Absent cannot mean "remove" here, unlike
every other field on the route: the API returns no credential, so a form has nothing to send back
and editing an identifier would otherwise discard a token silently.

**The identifier does not get that exemption**, because a response does return it: `PUT` replaces
it, so **omitting it clears it** — the same rule as `address` on a branch and the logo on an
account. That matters more here than there, since `resolveWhatsAppChannel` uses it to tell one of a
branch's numbers from another, so a form editing a channel sends the identifier back every time.

### Credentials are encrypted at rest, and never come back out

Every credential field is sealed with **AES-256-GCM** under `CHANNEL_CONFIG_ENCRYPTION_KEY` before
it reaches the database, under a fresh nonce, and stored as `v1.<base64>` in the same JSON key. The
readable fields stay in the clear. What this closes is a database-only compromise — a leaked backup,
a replica, a dump, a config that reaches a log — which account isolation and row level security do
not touch, since both assume the query goes through the API.

The key is its own setting and is deliberately not derived from `AUTH_JWT_SECRET`: rotating the token
secret is routine, and it would make every stored credential unreadable. Left unset the API still
boots and still serves channels; storing a credential is the one thing refused, with **503** and
`NOT_CONFIGURED`, so a deployment that never set a key cannot end up keeping a provider token in the
clear. A key that is set but not 32 bytes of base64 is a startup error, so a typo cannot quietly turn
encryption off. Rotating it makes already-sealed credentials unreadable — the envelope carries the
generation it was written under, so re-sealing is possible, but nothing does it yet.

**No response carries a configuration, whole or partial.** `ChannelResponse` has no `config` field at
all; `is_configured` is the whole surface, and it reports only that something is stored. This is
enforced a layer lower than the DTO: **no read selects the column.** Every channel query returns
`config IS NOT NULL` instead, so a `domain.Channel` cannot hold a credential and therefore cannot
leak one into a response or a `%+v` in a log line. The first connector that needs to read one adds
its own read for it.

## Account record

`GET /v1/account` returns the caller's own account and is readable by any member, because
anything naming the corralón needs it; `PUT /v1/account` replaces it and is admin-only. The route
carries no account id — it writes whatever the session resolved.

**Replaces, so an omitted optional field is cleared.** That is how a corralón removes a logo it no
longer wants on its quotes. A name that is blank once trimmed is refused with 422.

The brand pair (`brand_logo_url`, `brand_color`) is what the client webapp renders a quote with, and
both carry a format because a malformed one breaks a screen the corralón cannot see: the logo must be
an absolute URL, and the colour hexadecimal with three, four, six or eight digits behind a hash
(`#C2410C`). Either one malformed is a 400. `settings/account` in the backoffice mirrors both shapes
exactly rather than narrowing them — a form stricter than the column cannot show an account its own
stored value back.

## Identity

`GET /v1/me` returns the caller's id, name, email, role, account and branch reach. The frontend
reads it instead of decoding the access token, so token structure stays a backend concern.

## Configuration

`BRANCH_DEFAULT_EXPIRY_DAYS` (default `7`) is how many days a quote from a newly opened branch
stays valid. `config.Load()` refuses a value of zero or less.

`CHANNEL_CONFIG_ENCRYPTION_KEY` (no default) is 32 bytes of base64 — `openssl rand -base64 32` — and
seals the credential fields of `channel.config`. Unset, storing a credential is refused; malformed,
`config.Load()` refuses to start.
