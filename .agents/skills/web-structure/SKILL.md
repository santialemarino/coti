---
name: web-structure
description: Frontend app structure and where to create files (pages, components, lib, config) across Coti's two Next.js apps — backoffice (authenticated) and webapp (public). Use when adding routes, pages, or organizing code in apps/backoffice or apps/webapp.
---

# Web structure (Coti frontend)

Coti has **two** Next.js apps (this is the defining difference from a single-app
frontend). Decide which app you are in **first** — the rules below differ per app.

- **`apps/backoffice`** — Authenticated vendor & admin web app. Port 3000. Uses
  route groups: `(auth)` (no session) vs `(protected)` (session enforced). This
  is where reps triage RFQs, review AI-extracted quotes, manage the catalog, and
  switch sucursales.
- **`apps/webapp`** — Public customer-facing app. Port 3001. **No auth, no route
  groups** — every route is public. This is where a customer submits an RFQ or
  reviews/responds to a quote via a tokenized link.

Both apps share the same toolchain: Next.js 16 (App Router, React 19), Tailwind
v4 (CSS-first — no `tailwind.config`), TS 5.9, and the shared design system
`@repo/ui`. Its primitives wrap Radix and are **hand-authored on Coti's semantic
tokens** — there is no shadcn CLI in this repo, and generating one would emit
shadcn's own token vocabulary and dark-mode variants, neither of which Coti has.
The API is Go + Gin (snake_case JSON) —
see "API boundary" in `web-components-pages`. Product language is Argentine
Spanish; `<html lang="es">`. **i18n runs on next-intl**, pinned to a single
locale (`es-AR`) with no locale routing — UI copy lives in `translations/es.json`
and all number/currency/date formatting goes through the `lib/i18n/` formatters
(see `web-components-pages` → "Copy, i18n, and formatting").

## Path aliases (both apps)

From each app's `tsconfig.json`. Always import through these — never `.`/`..`:

- `@/app/*` → `app/*`
- `@/components/*` → `components/*` (app-wide components)
- `@/config/*` → `config/*`
- `@/hooks/*` → `hooks/*` (app-level client hooks — note: top-level `hooks/`, not `lib/hooks/`)
- `@/lib/*` → `lib/*`
- `@/translations/*` → `translations/*` (the message catalog)
- `@/types/*` → `types/*` (shared TS types)
- `@/public/*` → `public/*`
- `@repo/ui/components`, `@repo/ui/hooks`, `@repo/ui/lib`, `@repo/ui/styles` — the shared package.

## App Router layout — backoffice (authenticated)

- **`app/(auth)/`** — Route group for unauthenticated routes: login, signup, the
  forgot/reset-password pair, email verification and `session-ended`. Its
  `layout.tsx` does **not** require a session — bouncing an already-authenticated
  caller to `ROUTES.home` is the gate's job, before the layout renders, and it does
  it for the routes in `SIGNED_OUT_ONLY_ROUTES` only. `verify-email` and
  `session-ended` deliberately opt out: signup hands the caller a session and sends
  them to the first, and the second exists to clear the cookies of someone who still
  looks signed in, so bouncing it would loop.
- **`app/(protected)/`** — Route group for authenticated routes: RFQ inbox,
  quote (cotización) review, catalog (productos), sucursal-scoped surfaces,
  account/settings. Its `layout.tsx` calls `getSession()` and redirects to
  `LOGIN_ROUTE` when there is no valid session. It also hosts the app shell
  (header + branch switcher).
- **`app/layout.tsx`** — Root layout: async server component that resolves the
  locale + messages via next-intl, wraps `children` in `NextIntlClientProvider`,
  and sets `<html lang={locale}>` (always `es` today); imports `globals.css`.
  Each route group has its own `layout.tsx` for its shared wrapper.
- **`proxy.ts`** at the app root — the gate: reachability and the one place a
  session is renewed. This is the **Next 16 name** for what used to be
  `middleware.ts`, and the exported function is `proxy`. Never add a
  `middleware.ts` beside it — the build refuses both at once.

## App Router layout — webapp (public)

- **No route groups.** Every route is public; there is no session gate and no
  `(auth)`/`(protected)` split. Do not add one.
- **`app/layout.tsx`** — Root layout: async server component that resolves the
  locale + messages via next-intl, wraps `children` in `NextIntlClientProvider`,
  and sets `<html lang={locale}>` (always `es` today); imports `globals.css`.
- Routes live directly under `app/` — e.g. a public RFQ submission form and a
  tokenized quote-review route (`quotes/[token]/`). Access control for a customer
  route is the **unguessable token in the URL**, resolved server-side — never a
  session.

## `error.tsx` and `not-found.tsx` — both apps, both required

Next's own fallbacks are unstyled English (_"404 · This page could not be found."_), so every app
ships its own at the **root of `app/`**:

- **`error.tsx`** is a client component taking `{ error, reset }`. Production hands a boundary only
  a digest, so its copy is the generic one from the catalog — a failure a screen can name is worded
  where it happened, not here.
- **`not-found.tsx`** is a server component, so it can read whatever decides its call to action. In
  the backoffice that is whether a token cookie exists: offering the login screen to someone already
  signed in is a dead end, and offering the home page to someone signed out bounces them back to
  login. Read the **cookie**, not `getSession()` — a 404 must not depend on the API being up, and
  `getSession` rethrows anything that is not a 401/403, which would turn an unrelated outage into an
  error screen where a plain "this page does not exist" belonged. A stale token costs one bounce off
  the gate, which is exactly what the gate is for.
- Both render **outside every route group's layout** — an unmatched URL is exactly what failed to
  reach one — so they bring their own page frame rather than inheriting it.

## Where to create files (both apps, unless noted)

- **New page (route):** Add a folder with `page.tsx`. In backoffice, place it in
  the correct route group (`(auth)` vs `(protected)`). In webapp, place it
  directly under `app/`. Add the path/builder to `config/routes.ts` and use that
  constant for every link and redirect — never hardcode a URL string.
- **Page-specific components:** In `_components/` next to the page, e.g.
  `app/(protected)/quotes/[quoteId]/_components/quote-review-table.tsx`. Only that
  page (and its children) import these.
- **Shared across all protected pages (backoffice only):** In
  `app/(protected)/_components/`, e.g. `app-header.tsx`, `branch-switcher.tsx`.
  For components used by more than one protected route but not outside it.
- **Used on multiple pages within one app:** The app's `components/` folder.
- **Reusable across BOTH apps (design system):** `packages/ui/src/components`,
  add to the package's `src/components/index.ts`, import from `@repo/ui/components`.
  See "App vs @repo/ui" below — this is the promotion rule that matters most here.
- **Shared logic (auth, API, utils):** `lib/` — e.g. `lib/utils/page.tsx`. Use for
  anything used by more than one route or shared between server and client within
  the app. The backoffice's session layer is deliberately **two** modules:
  `lib/auth/session.ts` is `server-only` and asks the API who the caller is, while
  `lib/auth/tokens.ts` holds the edge-safe primitives (cookie names, the expiry
  read, the raw token calls) because `proxy.ts` imports them and the proxy runs on
  the edge, where `next/headers` and `server-only` are unavailable.
  **A cookie reader returns `undefined` or a real value, never `''`.** Next implements
  `cookies().delete(name)` as a set to an empty string, so a read after a delete in the same
  request still finds the entry — blank. A caller falling back with `??` takes that as a real
  choice and looks up a value nobody set. Normalise at the reader (`?.value || undefined`), and
  use the `cookieJar()` double from `@repo/vitest-config/cookies` in tests: a hand-rolled jar
  that drops the key on delete is kinder than production and hides exactly this.
- **Client hooks (app-level):** `hooks/<name>.ts` (imported `@/hooks/...`), one
  hook per file, kebab-case named after the hook. A hook needed by both apps goes
  in `packages/ui/src/hooks` + its `index.ts` (imported `@repo/ui/hooks`).
- **Server-side data fetching (reads):** `lib/api/<feature>.ts` with
  `import 'server-only'`. Called directly from server components (`page.tsx`).
  Backoffice reads use the authenticated fetch (JWT from session); webapp reads
  are unauthenticated (public / token-scoped). Can be imported by multiple pages.
  **Wrap a read that a layout and a page both perform in React's `cache()`**, so the
  nested server components share one round trip instead of one each — a layout cannot
  pass props to the page under it, so re-calling is the only way to get the data there.
- **Server mutations:** `actions.ts` colocated with the page (`'use server'`).
  Called from client components. Feature-specific — do not put in `lib/`.
- **Cross-entity API contract types:** `lib/api/types.ts` (e.g. a shared
  `SortOrder`) — shared by multiple `lib/api/<feature>.ts` modules; entity-specific
  types stay in their feature module.
- **Routes:** `config/routes.ts` for `ROUTES`. The backoffice also exports what the
  gate reads — `PUBLIC_ROUTES`, `SIGNED_OUT_ONLY_ROUTES`, `LOGIN_ROUTE`, `NEXT_PARAM`
  — and `safeNextPath`, which is what makes a `?next=` round trip same-origin only.
- **Constants:** `lib/constants/<topic>.ts` — one file per topic (the backoffice
  has `auth.ts`, `branch.ts`, `brand.ts`, `forms.ts`, `password.ts`). Only for
  constants imported by 2+ files; single-file constants stay in the file that uses
  them. **Motion values are not among them** — durations and easings are tokens in
  `@repo/ui` (`MOTION`/`EASE` from `@repo/ui/lib`), never an app constant.
- **Shared TS types:** `types/` (imported `@/types/...`) for app-wide types that
  aren't tied to one `lib/api` module.
- **i18n (per app):** `translations/es.json` (the message catalog — one file,
  namespaced by feature), `i18n/request.ts` (next-intl request config, pins locale
  `es` + the Argentina timezone), and `lib/i18n/` (the formatter stack:
  `locales.ts` registry, `intl-cache.ts`, `format.ts`, `currency.ts`,
  `create-formatters.ts`, and the `formatters.ts` / `formatters-server.ts` hooks).
  This layer is **identical across both apps** except `translations/es.json`; when
  you touch a shared `lib/i18n/*` file, mirror the change to the other app. See
  `web-components-pages` → "Copy, i18n, and formatting".

## App vs `@repo/ui` — the promotion rule

This is the single most important structural decision in Coti, because there are
two apps.

- **Used in one app only** → that app's `components/` (or a page's `_components/`
  if single-page). Never reach across apps — `apps/webapp` must not import from
  `apps/backoffice` and vice-versa.
- **Genuinely needed by BOTH backoffice and webapp** → promote to
  `packages/ui/src/components`, export from `src/components/index.ts`, import via
  `@repo/ui/components`. Examples that belong in `@repo/ui`: the form primitives
  (both a login form and a public RFQ form need them), brand/logo, buttons and
  the other primitives, empty/loading states.
- **Primitives shared across both apps live in `@repo/ui`**, not duplicated per app.
  Add a shared primitive there and consume it from both apps. A new one is written by
  hand on the semantic tokens, wrapping Radix where a behaviour needs it — match the
  neighbouring components rather than generating anything.
- Do not speculatively promote. Build in the app; promote the moment a second app
  needs it, then delete the app-local copy.

### The stylesheet pipeline

`packages/ui/src/styles/index.css` is the **single Tailwind entry for the monorepo**:
it imports Tailwind, declares the tokens, and is compiled by the Tailwind CLI to
`packages/ui/dist/index.css`. Each app's `globals.css` imports **only**
`@repo/ui/styles` — never `tailwindcss` again, because the built bundle already
carries preflight and the utility layer, so a second import emits both twice. App-only
one-offs (a third-party library's CSS, a selector for DOM the app doesn't render) go
below that import.

Class scanning is monorepo-wide from inside the package: `@source` registers the
`packages/ui` and `apps` **directories**, not globs, so Tailwind walks them itself and a
class used only in an app is still generated. Tailwind's automatic detection is off, so
those two entries are the whole input — a class name only _mentioned_ in prose never
reaches the bundle. Keep them as directories; a glob has to reckon with the parentheses in
Next's route-group folders.

**`@repo/ui` ships its CSS prebuilt** (`exports["./styles"]` → `dist/index.css`;
components are consumed from `src`, so their logic is live but their styles are not).
After editing a `packages/ui` component's classNames, rebuild it
(`pnpm --filter @repo/ui build`, or rely on its `dev` watcher) — otherwise the new
classes never reach the app and the change silently does nothing. `pnpm dev` builds
`@repo/ui` before starting the apps, so a cold start can't race it.

**Its build declares the app sources as inputs** (`packages/ui/turbo.json`), because the CSS is
produced by scanning them. A task cached on its own package alone is wrong here: a class used for
the first time in app code would not invalidate it, so `pnpm build` would hand back a bundle
missing that class while reporting success. The failure is local-only — CI has no cache and always
scans — which is exactly what makes it easy to verify a screen against a stale bundle and see a
layout that will look different in production. If a class is missing, confirm it **positively**
(list what the bundle does contain) before doubting anything else.

## Directory layout — apps/backoffice/

Both trees below are the **target** shape: they name where a thing goes, not only what
is built. `lib/api/` holds `account.ts`, `branches.ts`, `users.ts`, `client.ts` and
`errors.ts` today — `rfqs.ts` and `quotes.ts` are where those reads will go.

```
app/
├── layout.tsx                       # root: <html lang="es">, imports globals.css
├── globals.css
├── error.tsx                        # root error boundary (client)
├── not-found.tsx                    # branded 404, CTA follows the token cookie
├── page.tsx                         # entry (redirect to inbox or login)
├── (auth)/                          # No session; redirect out if already logged in
│   ├── layout.tsx
│   └── login/
│       ├── page.tsx
│       ├── _components/
│       ├── actions.ts               # 'use server' — sign in
│       └── form-schema.ts
├── (protected)/                     # getSession() + redirect to LOGIN_ROUTE
│   ├── layout.tsx                   # session check + app shell + branch switcher
│   ├── _components/                 # shared across protected pages (app-header, branch-switcher)
│   ├── rfqs/                        # RFQ inbox (solicitudes)
│   │   ├── page.tsx
│   │   ├── _components/
│   │   └── [rfqId]/page.tsx
│   ├── quotes/                      # cotizaciones
│   │   ├── page.tsx
│   │   └── [quoteId]/               # review-ready quote detail (human-in-the-loop)
│   │       ├── page.tsx
│   │       ├── _components/
│   │       └── actions.ts
│   ├── catalog/                     # productos
│   │   └── page.tsx
│   └── settings/
├── components/                      # app-wide (used by 2+ routes)
├── hooks/                           # app-level client hooks (@/hooks)
├── lib/
│   ├── auth/                        # session.ts (server-only) + tokens.ts (edge-safe)
│   ├── api/                         # server-only reads (authenticated fetch)
│   │   ├── rfqs.ts
│   │   ├── quotes.ts
│   │   └── catalog.ts
│   ├── constants/<topic>.ts
│   └── utils/page.tsx
├── config/
│   └── routes.ts                    # ROUTES, PUBLIC_ROUTES, LOGIN_ROUTE, safeNextPath
├── proxy.ts                         # the gate (Next 16's name for middleware.ts)
├── i18n/request.ts                  # next-intl request config (locale es, AR timezone)
├── translations/es.json            # message catalog (namespaced by feature)
├── lib/i18n/                        # formatter stack (shared shape across both apps)
├── types/
└── public/
```

## Directory layout — apps/webapp/

```
app/
├── layout.tsx                       # root: <html lang="es">, imports globals.css
├── globals.css
├── error.tsx                        # root error boundary (client)
├── not-found.tsx                    # branded 404
├── page.tsx                         # public landing / entry
├── rfq/                             # public RFQ submission
│   ├── page.tsx
│   ├── _components/
│   ├── actions.ts                   # 'use server' — submit RFQ
│   └── form-schema.ts
└── quotes/[token]/                  # customer reviews/responds to a quote via tokenized link
    ├── page.tsx
    ├── _components/
    └── actions.ts
components/                          # app-wide (used by 2+ routes)
hooks/                               # app-level client hooks (@/hooks)
lib/
├── api/                             # server-only reads (public / token-scoped, no auth)
│   └── public-quotes.ts
├── constants/<topic>.ts
└── utils/page.tsx
config/
└── routes.ts                        # ROUTES only — no gate, so nothing it would read
i18n/request.ts                      # next-intl request config (locale es, AR timezone)
translations/es.json                # message catalog (namespaced by feature)
lib/i18n/                            # formatter stack (shared shape across both apps)
types/
public/

packages/ui                         # Workspace — shared React + shadcn design system (@repo/ui)
```

## Related skills

- **`web-components-pages`** — how to actually create a page or component, the
  `@repo/ui` catalogue, the form stack, the API boundary mapping, icons, order/style,
  Tailwind class order, design tokens, and copy conventions.
- **`ux-motion`** — interaction states, motion, elevation, reduced motion.
- **`agent-workflow`** — branch/ticket flow before you start.
- **`commit`** / **`pr-format`** — commit message and PR conventions.
