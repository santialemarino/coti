# Authentication

A short access token plus a single-use rotating refresh token. The API is bearer-based: the
frontend is what stores the access token in an httpOnly cookie.

## Endpoints

| Method   | Route                     | Auth                                     |
| -------- | ------------------------- | ---------------------------------------- |
| `POST`   | `/v1/public/auth/login`   | no                                       |
| `POST`   | `/v1/public/auth/refresh` | no (the refresh token is the credential) |
| `POST`   | `/v1/auth/logout`         | yes                                      |
| `GET`    | `/v1/branches`            | yes                                      |
| `GET`    | `/v1/users`               | yes, admin                               |
| `POST`   | `/v1/users`               | yes, admin                               |
| `GET`    | `/v1/users/:userId`       | yes, admin                               |
| `PUT`    | `/v1/users/:userId`       | yes, admin                               |
| `DELETE` | `/v1/users/:userId`       | yes, admin                               |

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

## Middleware order

1. `Authenticate` runs on all of `/v1`: it verifies the token signature and delegates the
   rest to `AuthService.ResolveTenant`, which confirms what the signature cannot (the user
   exists, is active, the epoch is current, the requested branch is one they may use) in a
   single transaction. A request **without** an authorization header passes through
   unauthenticated rather than being rejected, so a public route can still see who is calling
   when they happen to be logged in.
2. `RequireTenant` guards the authenticated group and returns 401 when there is no tenant.
3. `RequireAdmin` goes after `RequireTenant` on admin routes.

## User administration

`/v1/users` is the one admin-only group, guarded by `RequireAdmin` after `RequireTenant`. A
seller reaching any of it gets **403**.

- **The account comes from the session**, never the body — there is no account field on the
  wire, so an admin cannot create a user anywhere but their own account.
- **An admin sets the initial password.** There is no invitation flow, so this is the only way
  a user gets credentials. It must clear `AUTH_PASSWORD_MIN_LENGTH`.
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
environment, unlike the operational thresholds. `AUTH_PASSWORD_MIN_LENGTH` is the operational
one, and it floors at 8.

The two development seed users have the password `coti1234`. Development only.

## Configuration

All in `apps/api/.env.example`, with defaults in `internal/config`: `AUTH_JWT_SECRET`
(required, at least 32 characters), `AUTH_ACCESS_TTL_MINUTES`, `AUTH_REFRESH_TTL_HOURS`,
`AUTH_REFRESH_REMEMBER_DAYS`, `AUTH_REFRESH_REUSE_GRACE_SECONDS`,
`AUTH_MAX_FAILED_ATTEMPTS`, `AUTH_LOCKOUT_MINUTES`, `AUTH_PASSWORD_MIN_LENGTH`.

## Not built yet

Password recovery, user invitations and email verification are not implemented, and neither is
an admin-initiated password reset — a user who forgets their password has no path back today.

Account signup is not implemented either: an account's first admin has no route, so the seed is
the only path to one. Account bootstrap goes through `db.AdminTx()`, because the account does
not exist when it is created and there is no scope to set.
