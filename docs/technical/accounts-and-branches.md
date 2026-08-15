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
4. the administrator, with the password hashed,
5. the account's resumable onboarding record.

The response carries a token pair, so the caller has a session without a second round trip.
Onboarding starts after email verification rather than extending this transaction into a longer
registration wizard. See [onboarding.md](onboarding.md).

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
