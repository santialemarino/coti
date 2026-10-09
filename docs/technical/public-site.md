# Public site

The landing, the privacy policy and the terms of use are a `(public)` route group inside
`apps/backoffice`, on the primary domain. Living in the same app as the session is the point: the
pages and the 404 read the token cookie to tell a seller from a visitor, which a site on another
host could not do, because the session cookies carry no `domain` attribute.

## Routes

| Path       | What it is                                     |
| ---------- | ---------------------------------------------- |
| `/`        | the landing, for everyone, signed in or not    |
| `/privacy` | legal text from `legal.privacy` in the catalog |
| `/terms`   | legal text from `legal.terms` in the catalog   |

A seller's home is the queue at `/inbox` (`ROUTES.home`): login, the "Pedidos" tab and every "Ir a
Pedidos" go there, so the root can be the same public page for everyone, at one address.

All three are in `PUBLIC_ROUTES` and none is signed-out-only: the header and the 404 send a seller
there too. The header offers a visitor "Ingresar" and "Probar Coti", and a seller "Ir a Pedidos".

## Not found and errors

- An unknown path answers **404 for anyone**. The proxy redirects to login only for a path
  `isProtectedPath` recognises (see `backoffice-session.md`, "The gate").
- `app/not-found.tsx` offers a seller "Ir a Pedidos" and "Ir al inicio" (the landing), and a visitor
  "Volver al inicio" (the landing). `app/(protected)/not-found.tsx` answers a
  `notFound()` raised inside the app, keeping the shell.
- Every route group has its own `error.tsx` rendering `ErrorCard` (`components/error-card.tsx`)
  inside the group's frame; its retry refreshes the router and resets the boundary in one
  transition. `app/error.tsx` catches a failing group layout, and `app/global-error.tsx` a failing
  root layout — it brings its own document and reads its copy straight from the catalog file,
  because no translation provider sits above it.

## Search engines and sharing

- The root layout is `noindex` by default; `generatePublicPageMetadata` (`lib/utils/page.tsx`) makes
  a public page indexable and gives it a canonical URL plus Open Graph and Twitter tags. It restates
  both objects in full on every page, because Next replaces them instead of merging them and a
  partial one silently drops the image.
- `app/opengraph-image.tsx` renders the social card (1200×630). It draws outside the stylesheet, so
  its colours are the literal values behind the brand tokens.
- `app/robots.ts` allows `/$`, `/privacy`, `/terms` and the social card, and disallows the rest;
  `app/sitemap.ts` lists the three public addresses. The webapp ships `noindex` and a robots file
  that disallows everything.
- Absolute URLs come from the request (`lib/utils/site-origin.ts`, `x-forwarded-host` /
  `x-forwarded-proto` first), so one build serves whatever domain the deployment is given and no
  environment variable names the host.
- The landing carries `Organization` and `SoftwareApplication` JSON-LD.

## Content

- The sections follow the order a buyer asks their questions in: the hero (what Coti does, with both
  calls to action above the fold), three benefits, the four steps, everything Release 1 includes (intake to ARCA invoicing), the
  copilot rules, the FAQ and a closing call to action that only a visitor sees. The copy describes
  the Release 1 scope, not only what has shipped so far.
- The product preview is drawn with the app's own components and fixed demo figures; nothing on
  the page computes a price.
- **No testimonial, logo or metric is shown until a real one exists**, and there is no pricing
  section until the commercial model is decided. Neither has a hidden placeholder in the code.
- Motion follows `ux-motion`: the hero's page-level entrance (`animate-rise-in`), and each section
  fades up once as it first enters the screen through `Reveal` — the one place Coti allows a scroll
  reveal. A section already on screen, or any section without JavaScript or under reduced motion,
  is simply shown.
