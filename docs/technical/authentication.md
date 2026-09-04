# Authentication

A short access token plus a single-use rotating refresh token. The API is bearer-based: the
frontend is what stores the access token in an httpOnly cookie. How the backoffice holds that
session, gates its routes and renews the token is in
[backoffice-session.md](backoffice-session.md).

## Endpoints

| Method   | Route                                 | Auth                                     |
| -------- | ------------------------------------- | ---------------------------------------- |
| `POST`   | `/v1/public/auth/login`               | no                                       |
| `POST`   | `/v1/public/auth/refresh`             | no (the refresh token is the credential) |
| `POST`   | `/v1/public/auth/forgot-password`     | no                                       |
| `POST`   | `/v1/public/auth/reset-password`      | no (the link is the credential)          |
| `POST`   | `/v1/public/auth/verify-email`        | no (the link is the credential)          |
| `POST`   | `/v1/public/auth/resend-verification` | no                                       |
| `POST`   | `/v1/auth/logout`                     | yes                                      |
| `POST`   | `/v1/auth/change-email`               | yes                                      |
| `POST`   | `/v1/auth/change-password`            | yes                                      |
| `GET`    | `/v1/branches`                        | yes                                      |
| `GET`    | `/v1/users`                           | yes, admin                               |
| `POST`   | `/v1/users`                           | yes, admin                               |
| `GET`    | `/v1/users/:userId`                   | yes, admin                               |
| `PUT`    | `/v1/users/:userId`                   | yes, admin                               |
| `DELETE` | `/v1/users/:userId`                   | yes, admin                               |
| `POST`   | `/v1/users/:userId/password-reset`    | yes, admin                               |

`login` and `refresh` return the same body: `access_token`, `access_expires_at`,
`refresh_token`, and a `user` with `id`, `account_id` and `role`. **The refresh token is
shown exactly once** — only its hash is stored.

## Access token

A JWT signed with HS256, and the algorithm is **pinned at verification**: a forged
`alg: none` header cannot downgrade the check.

Claims: `sub` (user), `account_id`, `role`, `session_epoch`, `iat`, `exp`.

**The active branch is not a claim.** A seller switches branch without logging in again, so
it travels in the `X-Branch-Id` header and is resolved per request.

**And it is validated.** It is the only thing standing between a caller and another branch of
their own account: branch isolation is application-layer by decision — the database policies
guard the account boundary, not the branch one — so a branch id taken from a request and
trusted propagates all the way down.

| Header                                            | Result                                     |
| ------------------------------------------------- | ------------------------------------------ |
| Absent, admin                                     | operates account-wide                      |
| Absent, seller                                    | confined to the branches they are assigned |
| A branch of the account, user assigned (or admin) | lands in the context                       |
| An existing branch with no assignment             | **403**                                    |
| A branch of another account, or nonexistent       | **403**                                    |
| Not a UUID                                        | **400**                                    |

An inaccessible branch is a 403 and is **not** silently dropped: dropping it would leave the
caller reading the whole account while believing they are scoped to one branch.

**Omitting the header does not widen a seller's reach.** With no branch selected, a seller's
per-branch reads narrow to the union of their `user_branch` rows, which `ResolveTenant` loads
into `Tenant.AllowedBranchIDs`; only an admin reads the whole account. `Tenant.BranchFilter()`
is the single place that decides this, and it **fails closed** — a seller with no assignments,
or whose assignments were never loaded, reads nothing rather than everything.

## Branch list

`GET /v1/branches` is what the branch switcher reads before it can send `X-Branch-Id`. An admin
gets every active branch of the account; a seller gets the ones they are assigned. It sits
behind `RequireTenant`, not `RequireAdmin` — a seller needs it to switch branch.

## Refresh token

32 bytes of entropy, stored only as a hex SHA-256. The raw value is not guessable, so a fast
hash is enough — unlike a password, which needs a slow one.

Every token belongs to a **family** (`family_id`). Refreshing consumes the presented token
and issues its successor in the same family, all in one transaction: a crash in the middle
cannot leave a session with no live token.

**Theft detection with a grace window:**

| Situation                                                       | Result                                             |
| --------------------------------------------------------------- | -------------------------------------------------- |
| A live token                                                    | rotates normally                                   |
| A consumed token, **inside** `AUTH_REFRESH_REUSE_GRACE_SECONDS` | a fresh rotation — two tabs racing, not theft      |
| A consumed token, **past** the window                           | the **whole family** is revoked and it answers 401 |
| A revoked, expired or unknown token                             | 401                                                |

Without the grace window, having two tabs open logs you out.

A rotation **preserves the original duration**: a "remember me" session does not decay into
the short TTL on its own.

## Immediate logout

`app_user.session_epoch` is incremented, and every access token carrying an older epoch is
rejected — with no blacklist. The presented refresh token's family is revoked too.

The refresh token in the body is **optional**: a client that lost it must still be able to
end the session. A token belonging to **another** user is ignored rather than revoked, so
logout cannot be used to end someone else's session.

The price of immediate logout: the middleware performs **one indexed primary-key read per
authenticated request** to compare the stored epoch (two when the request names a branch, in
the same transaction). That is deliberate.

## Login

1. Looks the user up by email on the **owner pool**. That is mandatory: at login the account
   is not known yet, so a tenant-scoped query would match no policy and **always fail**.
2. An unknown email, a wrong password and an inactive user all return **the same** 401. An
   email that does not exist still runs a bcrypt comparison against a dummy hash, so latency
   does not reveal which addresses are registered.
3. The `is_active` check runs **after** the password, so disabled accounts cannot be
   enumerated either.
4. A wrong password increments `failed_attempts`; reaching `AUTH_MAX_FAILED_ATTEMPTS` sets
   `locked_until`.
5. A locked account returns **429**, not 401 — that one is exposed on purpose: the client
   needs to tell "wrong password" from "stop trying for a while". While locked, the correct
   password also returns 429.

## A deactivated account cuts every way in

`account.is_active` is load-bearing, not decorative. Login, refresh and `ResolveTenant` all
read the user **and their account** in one join (`GetAuthSubjectByID` /
`GetAuthSubjectByEmailCrossAccount` → `domain.AuthSubject`), and every one of them asks
`IsUsable()` — both the user and the account are active — rather than `AppUser.IsActive`.

Reading it on every request is what makes it reach **tokens already issued**: a live access
token stops resolving the moment the corralón is deactivated, instead of lasting out its 15
minutes. It is also what makes reactivating a single write, with nothing else to undo — the
users sign in again and are back.

**The answer is the same as bad credentials.** A deactivated user, a deactivated account and
a wrong password are one 401 with one body. Someone without a session has no business
learning which of the three it was.

**The flag is written by a script pair, not an endpoint.** `user_role` is only `ADMIN` and
`SELLER`, so there is no actor inside the product a "deactivate this corralón" route would
belong to. Both commands run on the owner role (`DATABASE_ADMIN_URL`) because they are not
request-scoped:

```bash
pnpm db:account:deactivate --account <uuid>
pnpm db:account:activate   --account <uuid>
```

Deactivating also bumps `session_epoch` for every user of the account, in the same transaction.
That is **not** what stops the outstanding tokens — the flag above already does, on every
request. It buys one specific thing: **reopening the corralón does not resurrect the sessions
that were live when it closed.** A token minted before the closure carries the old epoch, so it
stays refused and the user logs in again. Activating therefore has no counterpart to undo. See
[database.md](database.md#deactivating-and-reactivating-an-account).

## Email verification

Registration mails a single-use confirmation link and records the send; confirming it stamps
`app_user.email_verified_at`. It rides the same machinery as password recovery —
`auth_token`, one shared issuer, the same hash-only storage and single-use redemption — and
differs only in the token type, the lifetime (`AUTH_EMAIL_VERIFICATION_TTL_HOURS`, 48) and
the route the link lands on.

- **Requiring it is a flag that starts off** (`AUTH_REQUIRE_VERIFIED_EMAIL`). `config.Load`
  **refuses to turn it on while `MAIL_PROVIDER` is `console`**: a transport that only writes to a
  log cannot deliver the link anyone would need, so enforcing it would lock every user out of the
  environment. That is also why `false` is the default rather than a soft start — under `console`
  it is the only value that boots. Under `smtp` the flag is free to be turned on — see
  [Outbound email](./outbound-email.md).
- **An admin-created user is verified on creation, in the same transaction.** No path mails
  them a link — only public registration does — so without this they would carry a null
  `email_verified_at` forever and the flag would lock them out of an account they were
  deliberately given access to. Trusting the admin's word is the right reading of the threat:
  verification exists to stop someone reserving an address they cannot read, which is a
  **public-registration** problem, and an admin works inside their own account and can squat
  nothing. Mailing a link instead would make a mistyped address a permanent lockout rather than
  a recoverable one that surfaces at password recovery.
- **Changing the address drops the confirmation.** The stamp proved one mailbox reachable and
  says nothing about the next, so both write paths null `email_verified_at`: `UserRepository.Update`
  whenever an admin edit actually changes the address — compared folded, like the unique index, so
  a change of case alone is not a change — and `UpdateEmail` unconditionally, because the
  self-service route below refuses an address that is not a change before it gets there. Without
  this an account could be pointed at a mailbox nobody proved while still reading as verified,
  which is the one thing this flag exists to prevent.
- **`GET /v1/me` reports `email_verified`**, which is what lets a screen tell "confirm your
  address" from "already done" instead of guessing.
- **Enforced on use, not at the door.** Login does not look at the flag: issuing a session is not
  using the product, and refusing at the door left whoever mistyped their address at signup with a
  session that expired and no screen to correct it from. Instead the flag rides `Tenant`, and
  `RequireVerifiedEmail` guards the authenticated group — see
  [Middleware order](#middleware-order) for what stays open.
- **Refresh does not look at it either.** Without renewal the session dies inside
  `AUTH_REFRESH_TTL_HOURS` and the lock-in returns through another door, so a renewed token is
  simply another unconfirmed one: the closed routes refuse it exactly as they refused its
  predecessor.
- **Confirming restores everything with no further step.** `ResolveTenant` reads the column on
  every request, so the write the link performs is the whole of it: no epoch bump, no token
  revocation, and the very next call with the same bearer is allowed through.
- **The caller is told why** — a 403 naming the reason, unlike every other rejection here. That is
  safe because it is only reachable with a live session: whoever sees it already authenticated, and
  they cannot get past it without being told what to do.
- **Confirming an already-verified address succeeds.** A user clicking the link twice has no
  way to tell the two clicks apart, so the second is not an error and does not move the
  timestamp.
- **`resend-verification` answers 202 for every address** — unregistered, already confirmed,
  deactivated — for the same reason `forgot-password` does.

**This does not close the address-squatting hole on its own.** Reserving someone else's address
is only prevented by _requiring_ verification, or by expiring unverified registrations, which
needs the scheduled-job runtime. The transport that requirement waited on now exists, so closing
it is the configuration change the flow was built for.

### Changing your own address

`POST /v1/auth/change-email` is what makes the requirement survivable: without it, correcting a
typo would go through user administration, which is on the closed side of the gate, and the person
this exists to rescue stays shut in.

- **The current password is the proof of identity**, compared outside the transaction the way
  `change-password` does it. A wrong one is a 401.
- **And the write carries that password's hash in its own predicate**, so a recovery link redeemed
  inside the ~100ms bcrypt window cannot have its change undone by the password it replaced —
  matching no row answers 401. `change-password` closes the same window the same way.
- **It does not end the caller's sessions**, unlike a password change. The credential has not
  moved, so revoking would deny an attacker nothing they could not redo by logging in with the
  password they already used to reach this route — and it would sign the caller out of their other
  devices in the middle of fixing a typo. The account is behind the 403 wall again either way,
  since the change drops the confirmation.
- **It retires any outstanding recovery link**, which is the one thing revocation would have
  covered: a `PASSWORD_RESET` token already mailed to the old address stays redeemable, and after
  the write that mailbox belongs to somebody else.
- **The old address is told.** A silent change is how a takeover goes unnoticed, and the previous
  mailbox is the only place it can surface, so it receives an `EMAIL_CHANGED` notification naming
  the address that replaced it. It carries no link, and it is sent **before** the confirmation link
  — that one can be asked for again from the confirmation screen and this one cannot. A failed
  delivery does not fail the change: the address has already moved and there is nothing to undo.
- **A conflict is a 409 with `EMAIL_TAKEN`, the caller's own address included.** The remedy is the
  same either way, and answering differently would report whether a stranger holds the address.
- It carries the **`mail` allowance**: it sends to an address the caller names, which is the
  surface `forgot-password` and `resend-verification` are bounded on for the same reason. It is
  also the only route that sends **two** messages, so it can spend twice the per-attempt
  `MAIL_SMTP_TIMEOUT_SECONDS` of the write budget.

## Rate limits

A global allowance over all of `/v1`, plus tighter ones on the surfaces a stranger can use to
flood the database or someone's mailbox, and on the ones a **provider bills per call**. It sits
**ahead of `Authenticate`**, so a flood is refused before it costs a query.

| Scope         | Setting                      | Routes                                                          |
| ------------- | ---------------------------- | --------------------------------------------------------------- |
| `global`      | `RATE_LIMIT_GLOBAL_MAX`      | all of `/v1`                                                    |
| `credentials` | `RATE_LIMIT_CREDENTIALS_MAX` | login, reset-password, verify-email                             |
| `signup`      | `RATE_LIMIT_SIGNUP_MAX`      | public account registration                                     |
| `mail`        | `RATE_LIMIT_MAIL_MAX`        | forgot-password, resend-verification, change-email, admin reset |
| `ai`          | `RATE_LIMIT_AI_MAX`          | the RFQ text draft and the development intake                   |

Refresh is deliberately left on the global allowance alone: the backoffice renews on a
schedule the user does not control, and a tighter limit there would log people out.

`ai` is the odd one out: it guards spend rather than load. The routes behind it each cost a
generation and an embedding, and the global allowance would let one authenticated seller run 300
of them a minute. Startup refuses a value above `RATE_LIMIT_GLOBAL_MAX`, which could never bite.

### A second counter, keyed on the mailbox

Every allowance above is keyed on the **caller**, which bounds what the API serves and not
what a mailbox receives: a sender coming from many addresses stays inside its own allowance on
each of them while one victim's inbox fills up. So `forgot-password` and `resend-verification`
carry a second counter keyed on the **target address**, and both have to pass before anything
is sent.

`change-email` deliberately carries only the caller-keyed one. Its per-address cap would have to
answer with the same 202-and-send-nothing the two public routes use, and this route reports whether
the write happened — so a silent refusal would tell the caller their address changed when it did
not. What bounds it instead is that a change writes the address to the caller's own row: reaching
the same mailbox twice means moving away from it and back, which costs two caller allowances.

| Setting                           | Key                    | Reach                             |
| --------------------------------- | ---------------------- | --------------------------------- |
| `RATE_LIMIT_MAIL_MAX`             | caller                 | one caller across the mail routes |
| `RATE_LIMIT_MAIL_PER_ADDRESS_MAX` | target address, hashed | one mailbox across every caller   |

- **Going over it still answers 202**, and simply does not send. A 429 here would be a perfect
  enumeration oracle — it would confirm both that the address is registered and that someone
  has been asking about it, which is precisely what the uniform answer exists to withhold.
- **The key is the hex SHA-256 of the normalised address**, so a dump of the counter store
  holds no mailbox in the clear. Normalising first is what makes case and padding one bucket
  instead of a fresh allowance per spelling.
- **The two routes share one bucket per address**, because what is being protected is the
  inbox and the inbox does not care which route filled it.
- It is **not** compared against `RATE_LIMIT_GLOBAL_MAX` the way the per-route allowances are.
  Those are unreachable above the global limit, since one caller has to spend both; this one is
  counted across callers, so it bites whatever the global limit is.
- The counter lives in the delivery layer, where the bound request already carries the address:
  the guard runs in the handler rather than in middleware, which would have to parse the body
  to find out who the message is for.

**The cost, stated plainly:** any per-target cap can be spent by someone other than the
target, so an attacker who knows an address can keep recovery mail from reaching it while they
keep spending. That is the same trade login lockout makes, and it is bounded from the other
side — filling one mailbox's window costs caller allowances too, so it cannot be done from one
address.

**The key is the authenticated user when the request carries a readable bearer, and the
client address when it does not.** Reading the token here only buys a stable thing to count
by — the signature is checked and nothing else, because session validation is
`ResolveTenant`'s job and this runs before it on purpose.

**The client address is counted back from the end proxies append to.** That is the only end a
client cannot forge: anything it writes into `X-Forwarded-For` itself ends up to the _left_ of
what the first trusted proxy wrote, so with `RATE_LIMIT_TRUSTED_PROXY_HOPS` intermediaries the
real caller is at `len(hops) - hops`. A chain shorter than configured means the request did not
arrive the way it was meant to, and the header is discarded for the peer address.

**`RATE_LIMIT_TRUSTED_PROXY_HOPS` defaults to 0**, which means the forwarding header is not
trusted at all — right while nothing sits in front of the API, and wrong the moment something
does. Setting it is part of putting a proxy in front, not an afterthought.

Counters are fixed windows held in memory, which is why the response can name an exact time to
retry rather than an estimate. That is enough for one instance; `middleware.Limiter` is the
seam a shared store takes over at. The 429 body names **no limit** — only
`retry_after_seconds`, mirrored in `Retry-After` — because which bucket was hit would tell a
caller probing the API how its allowances are laid out.

## Middleware order

1. `Authenticate` runs on all of `/v1`: it verifies the token signature and delegates the
   rest to `AuthService.ResolveTenant`, which confirms what the signature cannot (the user
   exists, is active, the epoch is current, the requested branch is one they may use) in a
   single transaction. A request **without** an authorization header passes through
   unauthenticated rather than being rejected, so a public route can still see who is calling
   when they happen to be logged in.
2. `RequireTenant` guards the authenticated group and returns 401 when there is no tenant.
3. `RequireVerifiedEmail` goes next, and answers **403** with `EMAIL_NOT_VERIFIED` when
   `AUTH_REQUIRE_VERIFIED_EMAIL` is on and the caller's address is unconfirmed. It takes the
   setting rather than being mounted conditionally, so the route tree is one shape whichever way
   the flag is set.
4. `RequireAdmin` goes after those on admin routes.

**Three routes sit above the confirmed-address gate**, because they are the only way out of being
unconfirmed and closing them is what would trap someone:

| Route                        | Why it stays open                            |
| ---------------------------- | -------------------------------------------- |
| `GET /v1/me`                 | the screen has to know whose address it is   |
| `POST /v1/auth/logout`       | ending a session is never worth blocking     |
| `POST /v1/auth/change-email` | correcting the address is the way out itself |

Everything else under `/v1` that resolves a tenant is closed. The exception is the signed file
link, which carries its whole authorization in the URL and resolves no tenant at all.

## User administration

`/v1/users` is the one admin-only group, guarded by `RequireAdmin` after `RequireTenant`. A
seller reaching any of it gets **403**.

- **The account comes from the session**, never the body — there is no account field on the
  wire, so an admin cannot create a user anywhere but their own account.
- **An admin sets the initial password.** There is no invitation flow, so this is the only way
  a user gets credentials. It clears the same policy as every other password.
- **An admin may create either role.** `ADMIN` and `SELLER` are both accepted.
- **A duplicate email is a 409**, raised by a constraint rather than a read-then-write. Two
  back it: `uq_app_user_email` per account, and `uq_app_user_email_global` on `lower(email)`
  across every account. The global one is what login depends on — it resolves a user by email
  alone, so an address has to identify exactly one row. Being functional, it also holds when a
  writer forgets to lowercase, which the per-account constraint does not.
- **`PUT` replaces** name, email, role and branch assignments. `is_active` omitted leaves the
  flag alone, so an edit form cannot silently revive a deactivated user.
- **`DELETE` deactivates**, keeping the row so the user's quotes keep an author.
- **Deactivating bumps `session_epoch`** in the same transaction, so the access tokens the user
  already holds stop working immediately instead of lasting until they expire.
- **An admin cannot deactivate themselves or change their own role.** Either would drop the
  last admin out of their own account, and there is no recovery path. Editing their own name
  and email stays allowed.
- **Branch assignments are written in the same transaction as the user**, and every branch id
  is checked to belong to the account first: a foreign key does not confine a child row to its
  account, because referential integrity bypasses row level security.
- **`password_hash` is never on the wire**, in any response.

## Passwords

bcrypt at the default cost. It is a cryptographic constant, so it is **not** configurable per
environment, unlike the operational thresholds.

**One policy, applied wherever a password is stored** — `domain.PasswordPolicy`, called by signup,
admin user creation, the self-service change and the recovery reset. A password must be at least
`AUTH_PASSWORD_MIN_LENGTH` characters (12 by default; the config refuses to be set below 8) and
carry an uppercase letter, a lowercase letter, a number and a symbol. The character rules are fixed
rather than configurable: they are a product decision, not an operational threshold an environment
tunes.

The cap is **72 bytes, not 72 characters**, because that is what bcrypt hashes — it refuses a longer
input outright, so the policy catches it as invalid input instead of letting the write fail. A
password of accented characters reaches that cap in fewer characters than an ASCII one.

**Logging in applies no policy at all.** The password is being compared, not chosen, so a rule
introduced after an account was created never locks that account out of its own login screen.

The two development seed users have the password `coti1234`. Development only — it predates the
policy, and only logging in accepts it.

## Password lifecycle

Three ways a password changes after the user exists.

| Path            | Route                                          | Credential presented |
| --------------- | ---------------------------------------------- | -------------------- |
| Change your own | `POST /v1/auth/change-password`                | the current password |
| Recover         | `POST /v1/public/auth/{forgot,reset}-password` | the mailed link      |
| Admin-triggered | `POST /v1/users/:userId/password-reset`        | the admin's session  |

**Changing a password ends every other session, and that takes two writes, not one.** Bumping
`session_epoch` kills the access tokens; on its own it leaves the refresh tokens alive, and one
of those would mint a fresh access token carrying the _new_ epoch — the session the change was
meant to end simply continues. So the credential write is always the same three statements in
one transaction: the new hash, the epoch bump, and revoking every live refresh token for the
user.

`change-password` therefore **returns a new token pair**: the caller's own session is among the
ones it just ended, so without a fresh pair a user would be logging themselves out.

**`forgot-password` always answers 202** — registered address or not, active user or not, mail
delivered or not. Anything else turns the response into a way to find out which addresses exist.

The **response** carries nothing; the **clock** still does. A registered address costs two
transactions and a transport call, an unregistered one costs a single read, and no dummy work
evens that out the way login's dummy bcrypt compare does. Bounding it is the rate-limit ticket's
job, not this route's.

**The link is a single-use, expiring grant**, stored in `auth_token`:

- 32 bytes of entropy, kept only as a hex SHA-256. The mailed value never touches the table.
- `expires_at` is `AUTH_PASSWORD_RESET_TTL_MINUTES` after it was minted.
- Requesting a new link retires the outstanding ones for that user and type.
- Redemption is `UPDATE ... WHERE consumed_at IS NULL` **first**, inside the transaction. That
  predicate is what makes it single-use under concurrency: two simultaneous redemptions of the
  same link, and the loser's update matches no row.
- Unknown, expired, already-redeemed and wrong-type tokens all answer **401** alike.
- The lookup runs on the owner pool. The bearer presents a link and nothing else, so the
  account is what the token reveals, not something the caller already knows.

**Admin-triggered reset sends the user the same link they would request themselves**, so the
administrator never learns or sets a password. It is confined to their own account by reading
the target _inside_ the account scope — a user of another account is simply absent, which is a 404. A deactivated user is a 422: there is no point mailing them a way back in.

The epoch bump lands where the password actually changes. For the recovery and admin paths that
is when the link is redeemed, not when it is sent — triggering a reset is not a request to log
someone out.

The link points at a **backoffice** route (`WEB_BACKOFFICE_URL` + `/reset-password?token=…`),
not an API one: the user clicks it in a mail client and has to land on a screen.

## Configuration

In `apps/api/.env.example`, with defaults in `internal/config`: `AUTH_JWT_SECRET`
(required, at least 32 characters), `AUTH_ACCESS_TTL_MINUTES`, `AUTH_REFRESH_TTL_HOURS`,
`AUTH_REFRESH_REMEMBER_DAYS`, `AUTH_REFRESH_REUSE_GRACE_SECONDS`,
`AUTH_MAX_FAILED_ATTEMPTS`, `AUTH_LOCKOUT_MINUTES`, `AUTH_PASSWORD_MIN_LENGTH`,
`AUTH_PASSWORD_RESET_TTL_MINUTES`, `AUTH_EMAIL_VERIFICATION_TTL_HOURS`,
`AUTH_REQUIRE_VERIFIED_EMAIL`, `WEB_BACKOFFICE_URL`, and the `RATE_LIMIT_*` group above
(including `RATE_LIMIT_MAIL_PER_ADDRESS_MAX`, default 3).

**`AUTH_JWT_SECRET` stays in the API.** It is a symmetric HMAC key, so a second service holding
it could mint a token for any account — the frontends forward the token and never verify it.
The backoffice's own settings are in
[backoffice-session.md](backoffice-session.md#configuration).

## Not built yet

User invitations are not implemented: an admin sets a user's initial password, which is what
lets US-05 close without them.

No endpoint writes `account.is_active`, by design — see above.

Rate-limit counters are held per process, so behind more than one instance the effective
allowance is the configured one times the number of instances. The seam is named in
[Rate limits](#rate-limits) above.
