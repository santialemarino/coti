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
v4 (CSS-first — no `tailwind.config`), shadcn (see `components.json`), TS 5.9,
and the shared design system `@repo/ui`. The API is Go + Gin (snake_case JSON) —
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
- `@/types/*` → `types/*` (shared TS types)
- `@/public/*` → `public/*`
- `@repo/ui/components`, `@repo/ui/hooks`, `@repo/ui/lib`, `@repo/ui/styles`, `@repo/ui/theme` — the shared package.

## App Router layout — backoffice (authenticated)

- **`app/(auth)/`** — Route group for unauthenticated routes: login (and later
  password reset / invitation acceptance). Its `layout.tsx` does **not** require a
  session; it should redirect an already-authenticated user to `ROUTES.home`.
- **`app/(protected)/`** — Route group for authenticated routes: RFQ inbox,
  quote (cotización) review, catalog (productos), sucursal-scoped surfaces,
  account/settings. Its `layout.tsx` calls `getSession()` and redirects to
  `LOGIN_ROUTE` when there is no valid session. It also hosts the app shell
  (header + branch switcher).
- **`app/layout.tsx`** — Root layout: async server component that resolves the
  locale + messages via next-intl, wraps `children` in `NextIntlClientProvider`,
  and sets `<html lang={locale}>` (always `es` today); imports `globals.css`.
  Each route group has its own `layout.tsx` for its shared wrapper.

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
- **Shared logic (auth, API, utils):** `lib/` — e.g. `lib/auth.ts` (backoffice
  session helpers), `lib/utils/page.tsx`. Use for anything used by more than one
  route or shared between server and client within the app.
- **Client hooks (app-level):** `hooks/<name>.ts` (imported `@/hooks/...`), one
  hook per file, kebab-case named after the hook. A hook needed by both apps goes
  in `packages/ui/src/hooks` + its `index.ts` (imported `@repo/ui/hooks`).
- **Server-side data fetching (reads):** `lib/api/<feature>.ts` with
  `import 'server-only'`. Called directly from server components (`page.tsx`).
  Backoffice reads use the authenticated fetch (JWT from session); webapp reads
  are unauthenticated (public / token-scoped). Can be imported by multiple pages.
- **Server mutations:** `actions.ts` colocated with the page (`'use server'`).
  Called from client components. Feature-specific — do not put in `lib/`.
- **Cross-entity API contract types:** `lib/api/types.ts` (e.g. a shared
  `SortOrder`) — shared by multiple `lib/api/<feature>.ts` modules; entity-specific
  types stay in their feature module.
- **Routes:** `config/routes.ts` for `ROUTES` (and in backoffice also `AUTH_ROUTES` and `LOGIN_ROUTE`).
- **Constants:** `lib/constants/<topic>.ts` — one file per topic (e.g.
  `rfq.ts`, `quotes.ts`, `catalog.ts`, `animations.ts`). Only for constants
  imported by 2+ files. Single-file constants stay in the file that uses them.
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
  other shadcn primitives, empty/loading states.
- **shadcn primitives** shared across both apps live in `@repo/ui` (that is where
  `Button` already is), not duplicated per app. Add a shared primitive there and
  consume it from both apps. Per-app `components.json` sets `ui` → `@/components/ui`
  for the shadcn CLI's default target, but a primitive both apps use should be
  moved into `@repo/ui` rather than generated twice.
- Do not speculatively promote. Build in the app; promote the moment a second app
  needs it, then delete the app-local copy.

**`@repo/ui` ships its CSS prebuilt** (`exports["./styles"]` → `dist/index.css`,
built by the Tailwind CLI over `@source '../components'`; components are consumed
from `src`, so their logic is live but their styles are not). After editing a
`packages/ui` component's classNames, rebuild it (`pnpm --filter @repo/ui build`,
or rely on its `dev` watcher) and restart the app — otherwise the new classes
never reach the app and the change silently does nothing.

## Directory layout — apps/backoffice/

```
app/
├── layout.tsx                       # root: <html lang="es">, imports globals.css
├── globals.css
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
│   ├── auth.ts                      # getSession + session helpers
│   ├── api/                         # server-only reads (authenticated fetch)
│   │   ├── rfqs.ts
│   │   ├── quotes.ts
│   │   └── catalog.ts
│   ├── constants/<topic>.ts
│   └── utils/page.tsx
├── config/
│   └── routes.ts                    # ROUTES, AUTH_ROUTES, LOGIN_ROUTE
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
└── routes.ts                        # ROUTES (no AUTH_ROUTES/LOGIN_ROUTE — public app)
i18n/request.ts                      # next-intl request config (locale es, AR timezone)
translations/es.json                # message catalog (namespaced by feature)
lib/i18n/                            # formatter stack (shared shape across both apps)
types/
public/

packages/ui                         # Workspace — shared React + shadcn design system (@repo/ui)
```

## Related skills

- **`web-components-pages`** — how to actually create a page or component, the
  form stack, the API boundary mapping, icons, order/style, Tailwind class order,
  design tokens, and copy conventions.
- **`agent-workflow`** — branch/ticket flow before you start.
- **`commit`** / **`pr-format`** — commit message and PR conventions.
