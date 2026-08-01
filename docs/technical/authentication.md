# Authentication

A short access token plus a single-use rotating refresh token. The API is bearer-based: the
frontend is what stores the access token in an httpOnly cookie.

## Endpoints

| Method | Route                     | Auth                                     |
| ------ | ------------------------- | ---------------------------------------- |
| `POST` | `/v1/public/auth/login`   | no                                       |
| `POST` | `/v1/public/auth/refresh` | no (the refresh token is the credential) |
| `POST` | `/v1/auth/logout`         | yes                                      |

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
| Absent                                            | operates account-wide (what an admin does) |
| A branch of the account, user assigned (or admin) | lands in the context                       |
| An existing branch with no assignment             | **403**                                    |
| A branch of another account, or nonexistent       | **403**                                    |
| Not a UUID                                        | **400**                                    |

An inaccessible branch is a 403 and is **not** silently dropped: dropping it would leave the
caller reading the whole account while believing they are scoped to one branch.

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

## Passwords

bcrypt at the default cost. It is a cryptographic constant, so it is **not** configurable per
environment, unlike the operational thresholds.

The two development seed users have the password `coti1234`. Development only.

## Configuration

All in `apps/api/.env.example`, with defaults in `internal/config`: `AUTH_JWT_SECRET`
(required, at least 32 characters), `AUTH_ACCESS_TTL_MINUTES`, `AUTH_REFRESH_TTL_HOURS`,
`AUTH_REFRESH_REMEMBER_DAYS`, `AUTH_REFRESH_REUSE_GRACE_SECONDS`,
`AUTH_MAX_FAILED_ATTEMPTS`, `AUTH_LOCKOUT_MINUTES`.

## Not built yet

Password recovery, user invitations and email verification are not implemented. Neither is
account signup — the seed is the only path to a user, and account bootstrap goes through
`db.AdminTx()` because the account does not exist when it is created.
