# Backoffice session and API layer

How the backoffice holds a session and how it talks to the API. The API side of the same story
is in [authentication.md](authentication.md).

## The token is opaque to the frontend

The backoffice **forwards** the access token and never inspects it for authorization. It does
not hold `AUTH_JWT_SECRET` and cannot verify a signature, deliberately: the key is symmetric,
so a second service holding it could mint a token for any account — the exact cross-tenant
exposure the row level security policies exist to prevent.

That leaves a clean split:

| Question                        | Answered by                                          |
| ------------------------------- | ---------------------------------------------------- |
| Is there a token at all?        | `middleware.ts`, from the cookie                     |
| Has it expired?                 | `middleware.ts`, from the unverified `exp` claim     |
| Is the session behind it valid? | the API, via `GET /v1/me` and on every other request |
| May this caller do this?        | the API, always                                      |

Reading `exp` without verifying it is safe because of what it is used for: the worst a forged
value buys is an unnecessary refresh. Nothing is authorized on it.

## Where the session lives

Two httpOnly cookies, `coti_access_token` and `coti_refresh_token`. httpOnly is the point —
client code cannot read either, so no script on the page can lift the token. They are session
cookies today; the API's `remember_me` path has no UI yet, so there is nothing to persist for.

- **`lib/auth/tokens.ts`** — edge-safe primitives: the cookie names and options, the expiry
  read, and the three calls to `/v1/public/auth/{login,refresh}` and `/v1/auth/logout`. It
  talks to the API with its own `fetch` because the authenticated client reads the session, so
  a session that read the client would be a cycle. It may not import `next/headers` or
  `server-only`: `middleware.ts` imports it, and middleware runs on the edge.
- **`lib/auth/session.ts`** — server-only. `getSession()` asks `GET /v1/me`, so its answer
  accounts for what a cookie cannot: a bumped session epoch, a deactivated user, a revoked
  token. `startSession` / `clearSession` / `endSession` own the cookie writes.

## The gate

`middleware.ts` runs on everything but static assets and decides reachability:

| Situation                                           | Result                                                     |
| --------------------------------------------------- | ---------------------------------------------------------- |
| Public route, no usable token                       | through                                                    |
| Login / forgot / reset, token present and unexpired | redirected home                                            |
| Protected route, token unexpired                    | through                                                    |
| Protected route, token expired, refresh token held  | renewed, then through                                      |
| Protected route, refresh rejected                   | cookies cleared, redirected to login with `?next=`         |
| Protected route, API unreachable                    | through — the cookies survive and the next request retries |

**Middleware is the only place a session is renewed.** Next allows a cookie write from a server
action, a route handler or middleware, and only middleware runs before the page renders — so
renewing here is what lets a server component read a live token without ever handling expiry.
The renewed pair is written onto the request as well as the response, so the render this
request triggers already sees it.

`AUTH_REFRESH_SKEW_SECONDS` (60) is how early a token counts as expiring, so a request cannot
start with one that dies mid-flight. Several requests arriving inside that window each try to
refresh; the API's `AUTH_REFRESH_REUSE_GRACE_SECONDS` window is what keeps those from reading
as token theft.

## Two gates, because one cannot see everything

`app/(protected)/layout.tsx` calls `getSession()` and redirects when it comes back null. That
is not a duplicate of the middleware check — middleware knows only that a token exists and has
not expired, while this asks the API whether the session is still good.

It redirects to **`/session-ended`**, a route handler, rather than straight to the login screen.
A layout cannot write cookies, so redirecting with the dead cookies still set would have
middleware bounce the caller back to a page that rejects them, forever. The route handler
clears the cookies and then sends them to login. `/session-ended` is public and, unlike the
other public routes, is exempt from the signed-in bounce for the same reason.

## `?next=` is resolved, not string-matched

The post-login destination is resolved against a throwaway origin and rejected unless the origin
is unchanged. A `startsWith('/')` check is not enough: the URL parser reads a backslash as a
slash, so `/\evil.com` passes it and then navigates off-site.

## The API client

`lib/api/client.ts` is the single point the backoffice talks to the API through — base URL,
bearer, `X-Branch-Id`, and the error vocabulary, decided once instead of per screen.

- `apiRequest<T>()` returns the parsed body or throws an `ApiError`. `apiFetch()` is the escape
  hatch for a caller that needs the raw response, like a download reading
  `Content-Disposition`.
- Emptiness is read off the **body**, not the status: the API answers 204 for a completed write
  and 202 for an accepted one, and neither carries a body.
- Every status maps to one `ApiErrorCode` (`badRequest`, `unauthenticated`, `forbidden`,
  `notFound`, `conflict`, `unprocessable`, `rateLimited`, `unreachable`, `unexpected`), so no
  screen branches on a raw status. The API's `{error, detail}` text is kept for the log and
  never rendered; the interface owns its own message per code, in `translations/es.json`.
- The client carries **transport only**. Turning the API's snake_case JSON into camelCase is
  each `lib/api/<feature>` module's job, with explicit raw types and a mapper per entity.
  Request bodies, query params and headers stay snake_case: that is the wire contract.

`app/error.tsx` is the recoverable state for anything a screen did not catch, so an unexpected
response leaves the user with a retry rather than a broken page.

## Forms

Server actions plus `useActionState`, with zod validating in the action. There is no
react-hook-form yet: the field-level UX it buys — inline per-field errors, dirty tracking — is
design-system work, and these screens are placeholders. Errors surface form-level today.

React resets a form once its action resolves, so an action that fails hands the submitted email
back for the field to re-seed itself. Passwords deliberately are not handed back.

## Configuration

`apps/backoffice/.env.example`: `NEXT_PUBLIC_API_URL` and `AUTH_REFRESH_SKEW_SECONDS`.
`AUTH_JWT_SECRET` is **not** among them, by the reasoning at the top of this file.
