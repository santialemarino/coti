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
client code cannot read either, so no script on the page can lift the token.

**"Mantener la sesión abierta" decides how long they last.** Unchecked, both are session cookies
that die with the browser and the API issues its short refresh TTL. Checked, `remember_me` goes
to the API — which is what unlocks its 30-day refresh window — a third cookie `coti_remember`
records the choice, and all three get a `maxAge` of `AUTH_REMEMBERED_SESSION_DAYS`. The flag has
to be recorded because middleware renews on the edge with no other way to know, and because
`change-password` re-issues the pair: without it a remembered session would quietly decay into
one that dies with the browser. The API remains what decides whether a refresh token is live, so
an over-long cookie costs nothing but a wasted round trip.

- **`lib/auth/tokens.ts`** — edge-safe primitives: the cookie names and options, the expiry
  read, and the three calls to `/v1/public/auth/{login,refresh}` and `/v1/auth/logout`. It
  talks to the API with its own `fetch` because the authenticated client reads the session, so
  a session that read the client would be a cycle. It may not import `next/headers` or
  `server-only`: `middleware.ts` imports it, and middleware runs on the edge.
- **`lib/auth/session.ts`** — server-only. `getSession()` asks `GET /v1/me`, so its answer
  accounts for what a cookie cannot: a bumped session epoch, a deactivated user, a revoked
  token. `startSession` / `clearSession` / `endSession` own the cookie writes.

`getSession()` and `getBranches()` are wrapped in React's `cache()`, so the shell, the section
layout and the page under them share one round trip instead of three.

**Every cookie reader here returns `undefined` or a real value, never `''`.** Next implements
`cookies().delete(name)` as a set to an empty string with an expiry in the past, so a read after a
delete in the same request still finds the entry — blank. Consumers that test truthiness cannot
tell the difference, but one falling back with `??` takes the blank as a choice and looks up a
value nobody set. Normalising at the reader is what keeps that from being each caller's problem.
Tests use the `cookieJar()` double from `@repo/vitest-config/cookies`, which reproduces the
delete-as-empty-value behaviour; a hand-rolled jar that drops the key is kinder than production
and hides exactly this.

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
start with one that dies mid-flight.

**A prefetch does not get to spend a refresh token.** `Link` prefetches would otherwise land
several renewals inside the same skew window, and the API reads a replayed refresh token past
its grace window as theft. Middleware lets a request carrying `next-router-prefetch` through
unrenewed: it renders nothing anyone is looking at. What survives that is genuine concurrency,
which the API's `AUTH_REFRESH_REUSE_GRACE_SECONDS` window absorbs.

## Two gates, because one cannot see everything

`app/(protected)/layout.tsx` calls `getSession()` and redirects when it comes back null. That
is not a duplicate of the middleware check — middleware knows only that a token exists and has
not expired, while this asks the API whether the session is still good.

It redirects to **`/session-ended`**, a route handler, rather than straight to the login screen.
A layout cannot write cookies, so redirecting with the dead cookies still set would have
middleware bounce the caller back to a page that rejects them, forever. The route handler
clears the cookies and then sends them to login. `/session-ended` is public and, unlike the
other public routes, is exempt from the signed-in bounce for the same reason.

## The active branch

A fourth cookie, `coti_branch`, holds the branch the caller is working in. `lib/auth/branch.ts`
owns it and the header switcher is its only writer; every authenticated call then inherits it as
`X-Branch-Id` without a screen deciding its own scope. Its options are the session's, so a
branch chosen in a plain session dies with the browser and `clearSession` drops it with the rest
— a choice that outlived its session would greet the next user on that machine.

Two rules make it safe, and both are the opposite of the obvious thing:

- **The cookie is never validated on read, and never discarded for looking wrong.** No branch
  header means account-wide for an admin, so dropping a suspicious value _widens_ their scope.
  It is forwarded as-is and the API — which checks it against the account and the caller's
  assignments on every request — answers 403. Validation happens once, on the **write**:
  `setActiveBranch` refuses a branch outside `GET /v1/branches`, so the cookie can never name
  one the caller never had.
- **`GET /v1/me` and `GET /v1/branches` opt out of the header** via `branchScoped: false` on the
  client. A stale cookie on identity would 403 the session itself and sign the caller out
  instead of failing one screen; a stale cookie on the branch list would 403 the very list
  needed to switch away from it, with no way back. Everything else inherits by default.

A caller reaching a single branch is shown no switcher: that branch is their whole reach and the
API scopes to it with or without the header. A screen that genuinely needs one branch named — the
price import — resolves the active branch, falling back to the sole reachable one.

`requireAdmin()` guards an admin-only page with **`notFound()`**, not a 403 screen, so a seller
who guesses the URL is not told the page is there. The API refuses them regardless; this only
decides what the refusal looks like.

## The browser's address is forwarded

Every call the backoffice makes is server-side, so the API would otherwise see one address for
every user and rate-limit them as one caller. The client's address travels in
`X-Forwarded-For` on both the API client and the session module's own fetches, and the API
counts one hop back to this server.

`WEB_TRUSTED_PROXY_HOPS` is how this app finds that address in its _own_ incoming request —
counted from the end whatever sits in front appends to, for the same reason the API does it.
Zero locally, where nothing is in front, and the API then falls back to its peer.

## The signup wizard

`/signup` is three steps — the corralón, its first branch, the administrator — on **one**
`react-hook-form`, because registration is a single request. Either the account, that branch, the
branch's manual-entry channel and the administrator all exist or none do, so a caller who
abandons the wizard has created nothing.

- **Each step gates on its own fields** (`form.trigger([...stepFields])`), never the whole form.
  Validating everything marks fields the caller has not reached and leaves messages on steps
  nobody is looking at.
- **The primary button submits on every step**, so Enter does what pressing it does; the handler
  decides whether that means "continue" or "create the account".
- **A rejection the API attaches to a field moves the wizard to that field's step.** Nothing ties
  the wizard's position to the form's state, so a `setError` on a field that is off screen reads
  as a button that did nothing — and stepping back while the request is open is enough to be
  somewhere else when the answer lands. `steps.ts` owns the field-to-step map, and a test pins
  every field of the schema to exactly one step.
- **A second submit cannot open a second account.** A disabled button stops a second click but not
  a second submit, so the handler refuses to re-enter while one is in flight.
- **A blank optional field is left out of the body rather than sent empty.** The API's optional
  fields are pointers with `omitempty`, which only skips a nil one — a pointer to `""` passes
  validation and reaches the column.
- **The step swap is atomic, and the entrance is a keyed CSS animation.** A step's stepper entry,
  its description and its button all live outside the box holding its fields, so an exit animation
  that outlives the state change puts one step's inputs under the next step's button — and a click
  there submits a step nobody has filled in. The `key` on the fields wrapper is what makes the
  remount, and therefore the replayed entrance, coincide with the state change.
- **Focus moves into the step that was just revealed**, onto whichever field carries an error and
  otherwise the first. Unmounting the outgoing step drops focus to the body, so without this
  tabbing restarts from the top of the page on every step and a screen reader is told nothing. It
  deliberately does not fire on the first render, which would skip the heading.

On 201 the answer carries a token pair, so the action opens a session and sends the caller to
`/verify-email`: signed in, with an address the API has not confirmed yet.

## Branch administration

`/settings/branches` is admin-gated by `requireAdmin()` and lists the account's branches, opens
one, edits one and closes one. Two things about it are decided by the API rather than by taste:

- **It lists active branches only, so it cannot reopen a closed one.** `GET /v1/branches` filters
  `is_active = TRUE` — the same predicate that keeps a closed branch out of the switcher and out of
  `IsAccessibleBy` — so a closed branch cannot be listed, and therefore cannot be selected to
  reopen. `PUT /v1/branches/:id` accepts `is_active`, so the capability exists on the API; what is
  missing is a way to see a closed branch at all.
- **Closing the active branch drops the selection with it.** The API refuses a branch that is not
  active, so a `coti_branch` cookie naming the branch just closed would answer 403 on every
  branch-scoped read afterwards, and the caller would be locked out of the app until they noticed
  the switcher. The action clears the cookie when the two ids match, and only on success.

The refusal to close the last active branch is a **422**, and it is the only 422 that route answers,
which is why it maps to its own message. On creating and editing, a 422 means this form and the
API's validation have drifted apart, so it reads as a generic validation problem instead.

Both writes revalidate `'/'` with `'layout'` rather than the route: a branch that opens or closes
changes the shell's switcher as much as the list, and the shell is not on this route's tree.

## The verification screen

`/verify-email?token=…` is the second route the API mails into, alongside `/reset-password`.
**Confirming is a button, not something that happens on load.** The link is single use, and a
mail client's scanner, a corporate link checker or a router prefetch all issue a GET — any of
which would burn the token before the person reading the mail ever clicked, and leave them
looking at "this link is not valid". A link that really is expired, used or unknown falls
through to a resend form, so the dead end is recoverable without going back to a mail client.

It is public but **not** signed-out-only: registration hands the caller a session, so the most
common way to reach it is already logged in. **That session is also what tells the two no-token
states apart** — signed in means they just registered and the mail is on its way, signed out means
the link they followed is broken. Both offer the resend form.

Login maps the API's 403 to its own message, which is the one rejection here that says why. It
is only reachable once the password matched, so it tells the caller nothing they could not
already establish — and they cannot get past it without being told.

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
- `X-Branch-Id` comes from the active branch by default. `branchScoped: false` opts a call out
  and `branchId` pins it to a specific branch — see "The active branch" above for why each
  exists.

`app/error.tsx` is the recoverable state for anything a screen did not catch, so an unexpected
response leaves the user with a retry rather than a broken page.

## Forms

**react-hook-form + zod, submitting to a server action.** The primitives are shared in
`@repo/ui` — `Form`, `FormField`, `FormItem`, `FormLabel`, `FormControl`, `FormDescription`,
`FormMessage`, `FormRootMessage` — because the webapp's public RFQ form needs the same field UX.

- **A schema is a factory taking the translator**, so a zod message is a catalog key the form
  resolves rather than Spanish baked into `form-schema.ts`. It defaults to identity, which is
  what lets the **server action re-validate with the same schema** and ignore the wording. The
  client's validation is a courtesy; the action never trusts it.
- **`noValidate` on every form**, which is only honest now that zod validates client-side: the
  browser's bubbles are suppressed and something real replaces them.
- **`FormControl` stamps `aria-invalid` and `aria-describedby`** onto the input it wraps, so the
  invalid styling and the screen-reader wiring are automatic instead of per call site.
- **The message row animates between `0fr` and `1fr`** and reserves no space when empty, so
  revealing an error never snaps the layout.
- **A required marker is `<FormLabel required>`**, never a hand-written asterisk.
- **A server-side rejection lands inline on its field** via `form.setError`, so it reads like a
  validation error — a wrong current password marks `currentPassword`. A rejection that belongs
  to no field goes to `root` and renders in `FormRootMessage`: login answers "invalid
  credentials" without saying which half was wrong, and the form must not invent a guess.

## Configuration

`apps/backoffice/.env.example`: `NEXT_PUBLIC_API_URL`, `AUTH_REFRESH_SKEW_SECONDS` and
`AUTH_REMEMBERED_SESSION_DAYS`. `AUTH_JWT_SECRET` is **not** among them, by the reasoning at the
top of this file.
