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

**The administrator's address must be free across every account.** `uq_app_user_email` is
`UNIQUE (account_id, email)` — per account — but login resolves a user by email alone, so two
accounts sharing an address would make the resulting session ambiguous. Registration therefore
refuses an address already in use anywhere and answers `409`.

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

`DELETE` deactivates rather than removes, so the quotes and prices that reference the branch
stay explainable. **Closing the account's last active branch is refused** — an account with no
branch cannot take an order. The count runs inside the caller's transaction, so two concurrent
closes cannot both pass the check.

Omitting `default_expiry_days` on create takes `BRANCH_DEFAULT_EXPIRY_DAYS`. It lives on the
branch, not the account, because tolerance to inflation differs between locations.

## Account record

`GET /v1/account` returns the caller's own account; `PUT /v1/account` replaces it and is
admin-only. The brand pair (`brand_logo_url`, `brand_color`) is what the client webapp renders
a quote with.

## Identity

`GET /v1/me` returns the caller's id, name, email, role, account and branch reach. The frontend
reads it instead of decoding the access token, so token structure stays a backend concern.

## Configuration

`BRANCH_DEFAULT_EXPIRY_DAYS` (default `7`) is how many days a quote from a newly opened branch
stays valid. `config.Load()` refuses a value of zero or less.
