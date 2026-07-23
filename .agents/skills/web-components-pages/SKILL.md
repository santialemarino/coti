---
name: web-components-pages
description: How to create a component or a page in Coti's Next.js apps (backoffice + webapp, App Router). Use when adding new UI, forms, or routes in apps/backoffice or apps/webapp.
---

# Web components and pages (Coti frontend)

> **Load `web-structure` first** — it decides which app you are in and where each
> kind of file goes. This skill covers _how_ to build the page/component once
> placement is decided. Coti has two apps: **backoffice** (authenticated, port
> 3000, uses `(auth)`/`(protected)` route groups) and **webapp** (public, port
> 3001, no auth, no route groups). Product language is Argentine Spanish; i18n
> runs on **next-intl** (single locale `es-AR`, no routing) — see "Copy, i18n,
> and formatting".

## Creating a page

1. **Choose the location (app-dependent):**
   - **backoffice:** Auth (login, password reset) → `app/(auth)/<name>/`.
     Authenticated (RFQ inbox, quote review, catalog, settings) →
     `app/(protected)/<name>/`.
   - **webapp:** directly under `app/<name>/` — there are no route groups.
2. **Register the path in config:** In `config/routes.ts`, add the path or
   builder to `ROUTES` (e.g. a `quoteReview(quoteId)` builder returning
   `/quotes/{quoteId}`). Use `ROUTES.*` everywhere — no hardcoded paths.
3. **Add `page.tsx`:** Default-export a server component.
   - **backoffice `(protected)`:** the layout already enforces a session; call
     `getSession()` in the page only if you need the user/branch. **backoffice
     `(auth)`:** redirect to `ROUTES.home` when already logged in.
   - **webapp:** the page is public. A customer-scoped route (e.g.
     `quotes/[token]`) resolves the **token** server-side and 404s an invalid one —
     access control is the token, never a session.
4. **Metadata:** Use `generatePageMetadata(...)` from `lib/utils/page.tsx` for a
   page title/description, so tab titles stay consistent across the app.

Keep the page thin — a server component that fetches (via `lib/api/*`) and
composes components. Put UI and interactivity in components.

## Creating a component

- **Only used on one page** → that page's `_components/` folder, e.g.
  `app/(protected)/quotes/[quoteId]/_components/quote-review-table.tsx`.
- **Used on multiple pages in one app** → the app's `components/` folder.
- **Reusable across BOTH apps** → `packages/ui/src/components` + export from
  `src/components/index.ts`; import via `@repo/ui/components`. (See `web-structure`
  → "App vs `@repo/ui`". Rebuild `@repo/ui` after changing a shared component's
  classNames.)
- **shadcn primitives** (`@repo/ui` CVA components generated/adapted from shadcn,
  like `Button`): do not enforce Tailwind class order or the design-token
  conventions below on these files. They are third-party-sourced — leave them
  as-is unless there's a specific functional reason to edit them.
- **Client interactivity:** Add `'use client'` at the top. Use for forms, hooks,
  event handlers, and anything using browser APIs or React state.
- **Importing:** Always import through the aliases in `tsconfig.json`
  (`@/app/...`, `@/components/...`, `@/lib/...`, `@repo/ui/components`) — never
  relative `.`/`..`.

## Reuse and componentization

Coti's design system is **nascent** — today `@repo/ui` ships only `Button`. So
these are the operating principles, not a catalog of existing parts:

- **Reuse-first, in search order.** Before building anything, look in this order:
  (1) the page's `_components/`, (2) the app's `components/`, (3) `@repo/ui`. Reuse
  or restyle-through-props what exists before adding new. When you do add
  something genuinely shared, extend via the base component's props / CVA variants
  — don't fork a component to change one style.
- **A control used in two apps is one shared component.** If both the backoffice
  and the webapp render the same thing (a logo, a form field set, an empty state),
  it lives in `@repo/ui`, built once. If it's one app only, it lives in that app's
  `components/`.
- **Componentize repeated structure.** Structure that repeats — an RFQ list row, a
  quote line-item row, a page header (title + meta), a section card — is ONE
  component rendered by mapping data, never copy-pasted markup. Sibling sections
  that share a shape become section components that a thin page composes.
- **Consistency beats local cleverness.** The same control means the same thing,
  in the same place and order, across pages and across both apps (e.g. don't flip
  primary/secondary CTA order between the backoffice quote view and the public
  quote view).
- **Button as a link.** Use `<Button asChild>` wrapping a `<Link>`/`<a>` rather
  than a hand-rolled styled anchor, so it keeps the variant, ring, and states.
- **When a new shared primitive is needed** (Dialog, Input, Card, Badge, Form
  primitives, table, etc.), add it to `@repo/ui` via shadcn (both apps'
  `components.json` are configured, `iconLibrary: lucide`, base color slate) and
  export it — do not generate the same primitive separately into each app.

## Forms and inputs

The form stack is **react-hook-form + zod** (to be added to the app when the first
form lands). Never wire a bare `<input>` to ad-hoc `useState`. The first forms
are the backoffice **login** and the webapp **public RFQ submission** — since both
apps need the same field UX, build the shared Form primitives
(`Form`/`FormField`/`FormItem`/`FormLabel`/`FormControl`/`FormMessage`) in
`@repo/ui` and consume them from both apps.

- **Colocate the schema.** The zod schema lives in `form-schema.ts` next to the
  page (`app/(auth)/login/form-schema.ts`, `app/rfq/form-schema.ts`). Reuse shared
  validators (e.g. an email regex, common error messages) rather than re-deriving
  per form.
- **`noValidate` on every `<form>`.** The browser's native validation bubbles are
  ugly and English — suppress them so the inline `FormMessage` is the single
  source of validation feedback.
- **Errors reveal without layout shift.** The field message animates height
  open/closed and reserves no space when absent, so showing/clearing an error
  never snaps the layout. Don't hand-roll an error `<p>` or pad fixed space for one.
- **Required marker is a prop, not markup.** Mark required fields through the
  label's `required` prop, so every asterisk matches — never hand-write a per-label
  `<span>`.
- **Surface server-side field errors inline.** When the API rejects a specific
  field (e.g. a 409 "email already registered", an invalid RFQ line), map it onto
  the field with `form.setError(name, …)` so it reads like a validation error —
  same place, same animation — instead of only a toast.

## Icons

- **Prefer Lucide.** Use `lucide-react` (a dependency of `@repo/ui`) whenever an
  icon exists there — `import { Check, FileText, Package } from 'lucide-react'`;
  render as `<IconName className="..." />`.
- **Custom SVG fallback.** When no Lucide icon fits, add a `.svg` under
  `public/icons/` and import it **as a React component** — both apps' `next.config.js`
  wire `@svgr/webpack` (webpack + turbopack), so `import Logo from '@/public/icons/logo.svg'`
  gives a component; render `<Logo className="..." />`. Use `*.svg?url` only when
  you specifically need the URL. Don't use `<Image src="/icons/...">` for icons
  that need CSS styling/scaling.

## Iteration

Prefer higher-order functions (`map`, `filter`, `reduce`, `some`, `every`, `find`,
`flatMap`, `forEach`) over `for` / `for...of` when the intent maps to one of them.
Use a loop only for early `break`/`continue`, async-sequential iteration
(`for await`), or when a loop is genuinely clearer.

## Colocate feature files

Keep feature-specific modules in the same folder as the page that uses them:
`actions.ts`, `form-schema.ts`, `schema.ts`. Same hierarchy as the route — e.g.
`app/(auth)/login/actions.ts`, `app/(protected)/quotes/[quoteId]/actions.ts`
(backoffice), `app/rfq/actions.ts` (webapp). Do not put them in a global `lib/`
unless they are shared across multiple routes.

## Order and style

**Outside the component (file-level order):**

1. **Consts** — Module-level constants at the top. **Every magic number is a named
   const** — no raw literals in logic (timeouts, page sizes, cookie durations,
   etc.). Single-file constants stay in-file; multi-file constants go in
   `lib/constants/<topic>.ts` (one file per topic).
2. **Metadata** — Pages only: `export const metadata` / `generateMetadata`.
3. **Props type** — Immediately above the component. Define it for every component
   that receives props; omit only when there are none.

**Inside the component (declaration order):**

1. **Session** — `getSession()` or similar, if needed (backoffice server
   components).
2. **Router** — `useRouter()` (client components).
3. **Formatters & translations** — `const fmt = useFormatters()` (client) or
   `const fmt = await getFormatters()` (server), immediately followed by
   `const t = useTranslations('<namespace>')` / `await getTranslations('<namespace>')`.
   Keep `fmt` right before `t`. Omit either when the component formats no values /
   has no copy.
4. **State and refs** — `useState`, `useForm`, `useRef`, `useWatch`, etc.
5. **Derived state / memo** — `useMemo` and computed values depending on the above.
6. **Effects** — `useEffect` and similar.
7. **Handlers and functions** — `onSubmit`, event handlers, callbacks.
8. **Early returns** — Guard clauses (loading, null checks).
9. **Return** — The JSX return, last.

One main component per file; small helpers can live in the same file or a sibling.
Use `ROUTES` consistently and pull all user-facing copy from `t(...)` (see
"Copy, i18n, and formatting").

## Tailwind class order (className)

Coti uses Tailwind v4 (CSS-first config). Order classes so they are predictable
and easy to scan. Apply in both apps (and in `@repo/ui` when adding/editing
non-shadcn components):

1. **Display & flex** — `flex`, `grid`, `flex-col`, `flex-1`, `flex-wrap`, …
2. **Sizing** — `size-*`, `w-*`, `min-w-*`, `max-w-*`, `h-*`, `min-h-*`,
   `min-h-screen`, … Use `size-*` when width and height are equal (`size-4`, not
   `w-4 h-4`).
3. **Alignment** — `items-*`, `justify-*`, `self-*`, `content-*`.
4. **Padding** — `p-*`, `px-*`, `py-*`, `pt-*`, …
5. **Gap** — Prefer `gap-x-*` / `gap-y-*` over `gap-*` unless both axes truly need
   the same gap. Flex column → `gap-y-*`; row → `gap-x-*`.
6. **Background & border** — `bg-*`, `border`, `border-*`.
7. **Rounded** — `rounded-*`.
8. **State & interactive** — `hover:*`, `active:*`, `focus:*`, `focus-visible:*`,
   `disabled:*`, `dark:*`, …
9. **Typography & misc** — `text-*`, `font-*`, `leading-*`, `whitespace-*`,
   `overflow-*`, `relative`, `absolute`, …

Example: `flex flex-col min-h-screen items-center justify-center px-6 gap-y-8 bg-muted/30 rounded-lg hover:bg-muted/50 text-foreground`.

## Typography, rounding, and design tokens

The theme lives in `packages/ui/src/styles/theme.css` and defines **color tokens
(oklch) and a radius only** — there is **no custom typography token scale yet**.
Be accurate about that; don't reference type tokens that don't exist.

- **Colors — always tokens, never raw values.** Use the semantic tokens:
  `bg-background`/`text-foreground`, `bg-card`/`text-card-foreground`,
  `bg-primary`/`text-primary-foreground`, `bg-secondary`, `bg-muted`/`text-muted-foreground`,
  `bg-accent`, `bg-destructive`, `border`, `border-input`, `ring`/`focus-visible:ring-ring`.
  Never inline a hex/oklch value, and **never reach for a raw Tailwind palette
  color** (`slate-*`, `gray-*`, `zinc-*`) even though the shadcn base color is
  slate — the tokens abstract it and carry dark mode (`.dark` class,
  `@custom-variant dark`).
- **Rounding — never fully square unless intentional.** `--radius` is 0.625rem
  with the scale `rounded-sm`/`rounded-md`/`rounded-lg` (from `@theme` in
  `theme.css`). Buttons/inputs use `rounded-md` (see `Button`); cards and grouped
  surfaces/callouts round too — audit every box you add.
- **Type — no scale yet, so keep it consistent and centralized.** Today, use
  Tailwind's standard text utilities (the home pages' `text-3xl font-bold` is the
  current baseline). Do not scatter one-off sizes ad hoc. When the product needs a
  real type scale, **define semantic text tokens in `packages/ui/src/styles/theme.css`**
  (via `@theme`) and consume them as tokens across both apps, rather than sprinkling
  raw `text-sm`/`font-*` everywhere. Note: `cn()` (`@repo/ui/lib`) is a vanilla
  `twMerge(clsx())` with no custom class-group config — if you add overlapping
  custom text tokens, name them so they don't collide (or extend `cn()` with a
  tailwind-merge config); don't rely on the merger to dedupe unknown tokens.
- **Variants via CVA.** Extend a component's CVA variants (as `Button` does) —
  don't fork the component to add a look.

## API boundary and naming conventions

The API is **Go + Gin** and speaks **snake_case JSON** (via `json` struct tags).
The frontend is **camelCase TypeScript**. The `lib/api/` layer is the boundary
where the two meet.

### camelCase in TypeScript

All TS identifiers use camelCase — interface properties, params, variables, form
field names, zod keys. Exceptions:

- **UPPERCASE constants** stay as-is (`DEFAULT_PAGE_SIZE`, `RFQ_STATUSES`).
- **String values that are API enum values** stay as-is (e.g. a quote status
  `'pending_review'`, `'sent'`) — they must match the backend enum.
- **URL param name strings** passed to `URLSearchParams` / `searchParams.get(...)`
  stay snake_case (they're API contract strings, e.g. `qs.append('branch_id', …)`).
- **Request body keys** sent to the API stay snake_case (the Go API reads them via
  its `json` tags), e.g. `{ branch_id: values.branchId }`.

### Mapping at the API boundary

Functions in `lib/api/` receive raw snake_case JSON and **must return camelCase
objects**. Never return `res.json()` directly when the response has snake_case
keys — define explicit raw types and map.

**File structure order in `lib/api/*.ts`:**

```
1. Raw types      — unexported, snake_case, match JSON exactly
2. Frontend types — exported, camelCase, what the rest of the app uses
3. Mappers        — unexported, one per entity
4. API functions  — exported: build query strings, fetch, call mappers
```

Full example (backoffice — authenticated read):

```ts
import 'server-only';

// --- Raw types (API JSON shape, snake_case) ---

interface QuoteRaw {
  id: string;
  rfq_id: string;
  branch_id: string;
  status: string;
  total: string; // NUMERIC(14,2) decimal string, never float
  created_at: string;
}

// --- Frontend types (camelCase) ---

export interface Quote {
  id: string;
  rfqId: string;
  branchId: string;
  status: string;
  total: string; // NUMERIC(14,2) decimal string, never float
  createdAt: string;
}

// --- Mappers ---

function mapQuote(raw: QuoteRaw): Quote {
  return {
    id: raw.id,
    rfqId: raw.rfq_id,
    branchId: raw.branch_id,
    status: raw.status,
    total: raw.total,
    createdAt: raw.created_at,
  };
}

// --- API functions ---

export async function getQuotes(branchId: string): Promise<Quote[]> {
  const res = await authenticatedFetch(`/v1/quotes?branch_id=${branchId}`, { method: 'GET' });
  if (!res.ok) throw new Error('Failed to fetch quotes');
  const raw: QuoteRaw[] = await res.json();
  return raw.map(mapQuote);
}
```

- **backoffice** reads/writes go through the **authenticated** fetch (JWT from
  session). **webapp** reads are **unauthenticated** — a public/token-scoped fetch,
  e.g. `getPublicQuoteByToken(token)` resolves a tokenized quote with no session.
- **Actions (`actions.ts`) build explicit snake_case bodies:**

```ts
// app/(protected)/quotes/[quoteId]/actions.ts
const { branchId, total, ...rest } = values;
await authenticatedFetch('/v1/quotes', {
  body: { ...rest, branch_id: branchId, total },
});
```

## Copy, i18n, and formatting

All UI copy and all number/currency/date formatting go through **next-intl**
(single locale `es-AR`, no routing — see `web-structure`). Never hardcode a
user-facing string, and never format a number or date by hand.

**Copy — the catalog.** UI strings live in the app's `translations/es.json`,
namespaced. Each page/feature gets a top-level namespace (`quotes`, `catalog`,
`rfq`, `auth`, …); copy used by 2+ pages goes under `common`.

- **Server components:** `const t = await getTranslations('quotes')`, then `t('title')`.
- **Client components:** `const t = useTranslations('quotes')`, then `t('title')`.
- Keys are dotted within the namespace (`t('form.email.label')`). Interpolation is
  single-brace ICU (`t('total', { amount })`); counts use ICU `plural`
  (`"itemCount": "{count, plural, =0 {Sin ítems} one {# ítem} other {# ítems}}"`).
- Argentine Spanish, voseo ("Ingresá…", "Buscá…", "Seleccioná…"). The text lives in
  the catalog; the wording conventions below describe what to write there.

**Formatting — the formatters.** Never call `Intl.*`, `toLocaleString`, or build a
currency/date string inline. Use the locale-bound formatters from `lib/i18n/`:
`const fmt = useFormatters()` (client) / `const fmt = await getFormatters()`
(server). They close over the locale + timezone so you never thread them.

- **Money:** `fmt.currency(quote.total, 'ARS')` — the API sends money as
  **NUMERIC(14,2) decimal strings, never float**, and `quote.total` is one such
  decimal string; `fmt.currency` is the one place it becomes a display string
  (`$ 1.234,56`). `'ARS'` is the default.
- **Numbers:** `fmt.value(n)` (thousand separators; `{ compact: true }` → "1,5 M"),
  `fmt.signedValue(n)`, `fmt.ratePct(ratio)` (0.21 → "21%").
- **Dates:** `fmt.date(iso)` (date-only, "2 ene 2025"), `fmt.timestamp(iso)`
  (date + time in the Argentina zone). **Lists:** `fmt.list(items)` ("cemento, arena y cal").
- Interpolate a formatted value into copy by formatting first, then passing it as an
  ICU arg: `t('total', { amount: fmt.currency(quote.total) })`.

To add a formatter, extend `format.ts` (a pure fn) and expose it in
`create-formatters.ts` — keep both apps' `lib/i18n/` in sync. Adding a second locale
later is one entry in `lib/i18n/locales.ts` + a new `translations/<code>.json` +
reintroducing negotiation in `i18n/request.ts`; nothing at the call sites changes.

### Placeholder wording (Argentine Spanish) — the copy you author in the catalog

Descriptive action phrases, by input type:

- **Text inputs:** `"Ingresá el/la ..."` (e.g. `"Ingresá el nombre del producto"`).
- **Search inputs:** `"Buscá ..."` (e.g. `"Buscá productos..."`, `"Buscá cotizaciones..."`).
- **Select inputs:** `"Seleccioná un/una ..."` (e.g. `"Seleccioná una sucursal"`).
- **Number inputs:** `"Ingresá ..."` (e.g. `"Ingresá la cantidad"`).
- **Optional/notes:** `"Notas opcionales..."`.

Only use examples (`"ej.: 25kg"`) when the field needs domain knowledge the user
might not have. Never use examples for standard fields like names or amounts.

### Fields with defaults

When a field has a default value:

- **Placeholder:** the default value itself (`placeholder="50"`) — shows what will
  be used if left empty. Don't use "Ingresá ..." for these.
- **Hint below input:** a "por defecto" line with the value interpolated (never
  hardcode the default in the string). Both placeholder and hint refer to the same
  default from the same source.

## Comments

- **Default:** `//`. Consecutive `//` lines are fine for a sequence of remarks.
- **Block `/* */`:** only when one explanation is long enough to wrap across lines
  (function-level description / strategy note), using the leading ` *` style:
  ```ts
  /*
   * Long explanation that wouldn't fit on one line and forms
   * a single cohesive thought.
   */
  ```
- Comment the non-obvious; don't comment the obvious.
- **End full-sentence comments with a period.** Exceptions: inline comments on the
  same line as code, single-line `/* ... */` labels, and title-style `//` section
  headers (`// --- Mappers ---`).

## Related skills

- **`web-structure`** — file placement across both apps and the app-vs-`@repo/ui`
  promotion rule (read it first).
- **`agent-workflow`** — branch/ticket flow.
- **`commit`** / **`pr-format`** — commit and PR conventions.
  </content>
