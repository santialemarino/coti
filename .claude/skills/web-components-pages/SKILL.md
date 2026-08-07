---
name: web-components-pages
description: How to create a component or a page in Coti's Next.js apps (backoffice + webapp, App Router). Use when adding new UI, forms, or routes in apps/backoffice or apps/webapp.
---

# Web components and pages (Coti frontend)

> **Load `web-structure` first** — it decides which app you are in and where each
> kind of file goes, and **`ux-motion` alongside this one** whenever the change is
> visual — it owns interaction states, motion, elevation and reduced motion, which
> this skill points at but does not restate. This skill covers _how_ to build the
> page/component once placement is decided. Coti has two apps: **backoffice** (authenticated, port
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

`@repo/ui` is a real design system now — check it before writing markup. The
catalogue:

| Group    | Components                                                                                                                                                         |
| -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Controls | `Button` `PendingButton` `Badge` `Input` `SearchInput` `Textarea` `Label` `Checkbox` `RadioGroup` `Switch` `ToggleGroup` `Combobox` `Pagination` `RowActionButton` |
| Surfaces | `Card` `Separator` `Table` `Skeleton` `Spinner` `Progress` `Avatar` `Hint` `Callout`                                                                               |
| Overlays | `Dialog` `Sheet` `Popover` `DropdownMenu` `Tooltip` `Collapsible` `Command` `ConfirmDialog`                                                                        |
| Patterns | `StatusScreen` `Stepper` `EmptyState` `TableEmptyRow` `SortableTableHead` `InlineLink` `DropdownChevron`                                                           |
| Forms    | `Form` `FormField` `FormItem` `FormLabel` `FormControl` `FormDescription` `FormMessage` `FormRootMessage`                                                          |

- **Reuse-first, in search order.** Look in this order: (1) the page's
  `_components/`, (2) the app's `components/`, (3) `@repo/ui`. Restyle through the
  props and CVA variants a component already exposes — don't fork it to change one
  style, and don't rebuild something the table above already names.
- **Use the props before reaching for `className`.** `Button` has `variant`/`size`,
  `Badge` has `tone`/`size`/`dot`, `Card` has `interactive`, `Label` and `FormItem`
  have `required`, `Input` has `startIcon`/`endIcon`/`prefix`/`suffix`, `Dialog` has
  `closeOnClickOutside`, `PendingButton` has `pending`/`pendingLabel`. Passing raw
  classes where a prop exists is how two call
  sites end up looking different.
- **One dropdown: `Combobox`.** Radix `Select` is deliberately absent from the
  design system because it has no exit animation. Never add it back.
- **A control used in both apps is one shared component** in `@repo/ui`, exported
  from `src/components/index.ts`. One app only → that app's `components/`. Never
  reach across apps.
- **A domain enum maps to a tone in the app, not in `@repo/ui`.** The design system
  ships `tone="success" | "warning" | "danger" | …`; the surface that owns the quote
  status or the match confidence decides which one it is.
- **Componentize repeated structure.** An RFQ list row, a quote line-item row, a
  page header, a section card — ONE component rendered by mapping data, never
  copy-pasted markup. Sibling sections that share a shape become section components
  a thin page composes.
- **Consistency beats local cleverness.** The same control means the same thing, in
  the same place and order, across pages and across both apps.
- **Button as a link.** `<Button asChild>` wrapping a `<Link>`/`<a>`, so it keeps
  the variant, ring and states. Same for `InlineLink asChild` — that is why it takes
  `asChild` at all, so the design system needs no framework dependency.
- **Adding a new shared primitive:** hand-write it in `packages/ui/src/components`
  against the tokens and export it. A `shadcn add` is a starting point at best — its
  output uses raw palette colours and `dark:` variants, both of which are wrong here,
  so adapt it rather than committing it as generated. Then rebuild `@repo/ui`.

## Forms and inputs

The stack is **react-hook-form + zod** with the shared `Form` primitives from
`@repo/ui`. Never wire a bare `<input>` to ad-hoc `useState`.

- **A field is** `FormField` → `FormItem` → `FormLabel` + `FormControl` (wrapping
  the input) + optional `FormDescription` + `FormMessage`. `FormControl` stamps the
  id, `aria-invalid`, `aria-describedby` and `aria-required` onto whatever it wraps,
  which is what makes the error styling and the screen-reader wiring automatic.
- **Colocate the schema** in `form-schema.ts` next to the page, as a factory taking
  the two translators (`loginSchema({ field, shared })`) so every message is a catalog
  key the form resolves. Build it from the shared validators in `lib/forms/validators.ts`
  — `requiredText`, `optionalText`, `emailAddress`, `currentSecret`, `newPassword`,
  `passwordConfirmation` — and add to that module rather than re-deriving a rule per form.
- **Every `useForm` spreads `FORM_VALIDATION`** from `lib/forms/options.ts`, so when a
  message appears is one decision for the whole app, not nine.
- **`noValidate` on every `<form>`.** The browser's native bubbles are untranslated
  and ugly; the inline `FormMessage` is the single source of validation feedback. A
  `minLength`/`pattern` attribute is dead weight next to it — the schema is the rule.
  `maxLength` stays, because stopping the keystroke is kinder than reporting it after.
- **Required marker is a prop.** `<FormLabel required>` or `required` on `FormItem`
  — never a hand-written asterisk `<span>`.
- **Errors reveal without layout shift** — `FormMessage` and `FormRootMessage`
  animate height and collapse to zero footprint. Don't hand-roll an error `<p>` or
  reserve fixed space for one.
- **A form-level rejection is `FormRootMessage`** fed by `form.setError('root', …)` — for a
  failure that belongs to no single field, like a rate limit or an unreachable API. **A rejection
  the caller can act on goes on the field they will edit**, even when the API deliberately does not
  say which one was wrong: a refused login sits on the password, because a root error only clears
  on the next submit and would otherwise outlive the attempt it belongs to.
- **Surface a server-side field error inline** with `form.setError(name, …)`, so an address
  the API already holds reads like a validation error in the same place with the same animation,
  instead of only a toast. The action names the field and the resolver names the sentence — see
  "Errors across the boundary".
- **A password input is `PasswordField`**, never a bare `Input type="password"`: it owns the cap,
  the reveal toggle, the autocomplete hint and the meter, so eight fields across five forms cannot
  drift apart. `meter` goes on the field where a password is **chosen** and never on its
  confirmation or on a login, where the caller is presenting one they already have. The meter marks
  what is missing in red only once that field has actually been rejected — keyed on the field's own
  error, not on the form's submit count, which a wizard step would trip on arrival.
- **Pass the accessible names the design system can't own:** `passwordToggleLabel`
  on a password `Input`, `clearLabel` on a `SearchInput`. They live under
  `common.form.*`.
- **Pending state is the submit button, and it is `PendingButton`** — not a hand-rolled ternary:
  `<PendingButton type="submit" pending={form.formState.isSubmitting} pendingLabel={t('submitting')}>`.
  It disables itself, sets `aria-busy`, shows the spinner, and animates the width change the two
  labels cause — which plain CSS cannot do at all, because `width: auto` is not transitionable.
- **`Button` defaults to `type="button"`.** A submit opts in with `type="submit"`, so a button added
  to a form can never submit it by accident. `asChild` passes `type` through untouched.
- **One `useTransition` per action, never one shared between several.** A shared transition only
  reports that _something_ is running, so a screen with export / preview / confirm buttons lights
  all three at once. **A flag naming the running action does not fix it:** a form `action` already
  runs inside a transition, and a state update made there does not commit until that transition
  ends, so the flag arrives after the spinner is gone. Give each action its own hook and disable on
  the union when they are mutually exclusive.
- **A form spanning steps is one `useForm`, it validates per step, and its step follows the error.**
  Advancing is a **submit of that step**: give the form a resolver that validates the current step's
  schema (the step read from a ref, since the resolver runs outside a render) and advance from
  `form.handleSubmit`. Never gate with `form.trigger` — it raises errors without marking the form
  submitted, and react-hook-form only re-checks a field on change once it is, so every message
  raised that way stands there while the caller types the value that answers it. **Track submission
  per step**, though: `isSubmitted` is a property of the whole form, so once one step has advanced
  the next one starts reporting errors on the first character typed into it. A ref the resolver
  reads — set on submit, cleared on every step change — keeps a step quiet until the caller has
  tried to leave it. Validating the
  whole object instead of the step is the other half of the trap: it marks fields nobody has
  reached. The last step validates everything, because a field two steps back can still have been
  emptied on the way through. When the server rejects a field, move to that field's step _and_
  `setError`: nothing ties a wizard's position to the form's state, so a message off screen reads as
  a button that did nothing. Keep the field-to-step map in one module and pin every field of the
  schema to a step with a test.
- **A disabled submit button stops a second click, not a second submit.** Enter still reaches the
  form, so a handler behind a write that must happen once refuses to re-enter while one is in
  flight.

### Validation messages

One rejection, one message, naming what is actually wrong:

- **Empty is not malformed.** Presence is checked before format, and the two carry different
  messages — `Ingresá tu correo electrónico.` then `Ingresá una dirección de correo válida.`.
  Chained zod checks all run and produce two issues for one field, so short-circuit with `.pipe()`
  (`min(1, required).pipe(z.email(invalid).max(…))`) and guard a cross-field refinement on the
  field being present, or a blank confirmation reports both "obligatorio" and "no coinciden".
- **A message about the field lives in the flow's catalog; a message every form shares lives in
  `common.form.errors`.** "Ingresá el nombre del corralón." is the field's; `tooLong`,
  `invalidEmail`, `passwordTooShort`, `passwordTooLong` and `passwordMismatch` are shared, and
  interpolate their numbers (`Máximo {max} caracteres.`) so a constant is never retyped as copy.
- **A rule the interface shows, the API enforces.** A requirement list the caller can read is a
  promise; if only the form checks it, an admin-created or API-created password walks straight past
  it. Mirror the constants in one module per side and let the API answer 422 when they drift.
- **Mirror the API's limits, maximums included.** A cap the column or the binding tag enforces is
  refused inline instead of becoming a 400 with nothing to point at. A field the API only compares
  (a current password) carries no floor — a rule that grew later must not lock out an account that
  predates it.
- **Never say the same thing twice on one field.** A `FormDescription` repeating the message the
  error will show is a defect; the hint keeps only what the error does not say.
- **One tone.** A required message is an instruction in Argentine Spanish naming the field
  (`Ingresá…`, `Elegí…`, `Repetí…`); a format message states the rule. Match the neighbours.

## Feedback: toast, callout, or field message

Three different things — using the wrong one is a common drift:

- **`toast`** (sonner, mounted once in the root layout) — a transient confirmation of
  something the user just did. "Se actualizaron 128 productos."
- **`Callout`** — a standing message about the content on screen. "Hay 2 ítems sin
  match en el catálogo."
- **`FormMessage` / `FormRootMessage`** — a field's or a form's rejection. Never a
  toast for a validation error; it belongs next to the input.

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

The token layer is `packages/ui/src/styles/index.css` (the tokens themselves) and
`theme.css` (the `@theme inline` block mapping them onto Tailwind namespaces). Full
reference, including how the ramp was derived and every contrast figure:
[docs/technical/design-system.md](../../../docs/technical/design-system.md).

- **Colours — always semantic tokens, never raw values.** `bg-background`/`text-foreground`,
  `bg-card`, `bg-popover`, `bg-sunken`, `text-foreground-muted`/`text-foreground-subtle`,
  `bg-primary`/`text-primary-foreground` (+ `primary-hover`/`primary-active`),
  `bg-secondary`(+`-hover`), `bg-muted`/`text-muted-foreground`,
  `bg-accent`/`bg-accent-strong`/`text-accent-foreground`, `border`/`border-strong`,
  `bg-input`/`bg-input-readonly`, `ring`, `bg-backdrop`. Never inline a hex or oklch
  value, and **never reach for a raw Tailwind palette colour** (`slate-*`, `blue-*`,
  `sky-*`) — no Tailwind ramp matches Coti's azure, and the tokens are the only place
  the mapping lives.
- **`brand-50`…`brand-950` exist, but reach for a semantic token first.** Use a ramp step
  only for a deliberately brand-coloured surface (a gradient, a brand tile). `primary`
  is `brand-600`; the ink `foreground` is `brand-950`.
- **Status tones come in four steps** — `success`/`warning`/`danger`, each with `-subtle`
  (tinted surface), `-border`, the base (`bg-success`, fills and icons) and `-foreground`
  (text). `destructive-*` aliases `danger-*`. Use `-foreground` for copy: `-base` is
  tuned for fills and does not carry text contrast. **Mapping a domain enum to a tone is
  the app's job** — `@repo/ui` ships tones, the app that owns the enum picks one.
- **Light-only.** There is no `.dark` block and no `dark:` variant. Never write a `dark:`
  class; it cannot match.
- **Type — use the scale, never a raw size/weight pair.** Headings are
  `text-heading-1`…`-6` (40/32/24/20/18/16px) and carry the display face themselves.
  Paragraphs are `text-paragraph{,-medium,-semibold}` and the same for `-sm` (14px),
  `-xs` (12px), `-mini` (11px). Replace `text-sm font-medium` with
  `text-paragraph-sm-medium`, and **never add a `font-*` weight next to a scale token** —
  the token already encodes it. Adding a scale token means also registering it in `cn()`'s
  tailwind-merge `font-size` group (`packages/ui/src/lib/utils.ts`), or two scale classes
  on one element will both survive and the loser will win on source order.
- **Elevation is a token.** `shadow-e1` (control hairline) → `shadow-e2` (resting card) →
  `shadow-e3` (dropdown, popover, hover-lifted card) → `shadow-e4` (dialog, sheet).
  Don't hand-roll a `shadow-[...]`.
- **Rounding — never fully square unless intentional.** `--radius` is 10px. Controls
  (buttons, inputs, selects) use `rounded-lg`; cards use `rounded-1.5xl` (14px); badges
  and pills `rounded-full`. Grouped surfaces and callouts round too — audit every box.
- **Motion durations and easings are tokens.** `duration-150/200/300/500` and
  `ease-out-soft`/`ease-in-out-soft` in classes; `MOTION`/`EASE` from `@repo/ui/lib` for
  `motion/react`. Never hardcode a duration.
- **Variants via CVA.** Extend a component's CVA variants — don't fork the component to
  add a look.

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

### Errors across the boundary

The API answers a refusal with a **stable `code`** beside the status (`docs/technical/api-specification.md`,
"The error envelope"). The code is the contract; the envelope's `error` prose is English, kept for
the log, and never reaches a screen. The status alone is not enough — one route answers 422 for the
password policy and another for the account's last active branch — so nothing branches on it.

- **`lib/api/client.ts` reads the code into `ApiError`**, narrowed against `API_ERROR_CODES` in
  `lib/api/errors.ts`. A code this app cannot word falls back to the one the status implies, which
  is also what covers the aborts the API writes before a handler is reached. The client mints two
  codes the API cannot answer: `UNREACHABLE`, for a request that never arrived, and
  `SESSION_EXPIRED`, for a session a re-check confirmed is over.
- **An action returns the code — never a sentence, never a key of its own.** `errorCodeOf(error)`
  is the whole mapping. Where a refusal belongs on a field, the action says _which_ field with a
  `Partial<Record<ApiErrorCode, …>>` map: placement is the action's business, wording is not.
- **One resolver turns a code into a sentence.** `useApiErrorMessage(namespace)` in a client
  component, `apiErrorMessage(await getTranslations(), namespace, code)` in a server one. Bind the
  flow's namespace once and pass the code straight through. **A ladder of `code === …` in a screen
  is the thing the codes exist to delete.**
- **The catalog is the only place wording is decided.** `errors.<CODE>` words every code once; a
  flow that says one differently repeats it under its own `<flow>.errors.<CODE>`. The namespace is
  walked back a segment at a time, so `users.passwordReset` inherits `users` and then the shared
  catalog — which is how one action words a code the rest of its flow shares. Override only where
  the shared sentence is wrong, never to restate it.
- **Every code carries an entry**, or next-intl renders the key. `lib/api/errors.test.ts` pins
  that, and `lib/i18n/api-error.test.ts` pins that no override names a code the wire cannot
  produce — a typo there falls through silently and the screen keeps working while saying the
  wrong thing.
- **A refusal the screen cannot act on is still a real answer.** A 429 says wait, a 400 says the
  body was refused; neither is "Ocurrió un error inesperado". Letting a status fall through to the
  generic sentence is a defect, not a default.
- **`common.form.errors` is a different catalog** — the schema's, keyed by rejection (`tooLong`,
  `invalidEmail`) rather than by API code. No API code is ever resolved against it.

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

**A placeholder is always an instruction, never an example.** One rule for every
field in the product, so two styles can never sit in one card — an example
(`tu@correo.com`) beside an instruction (`Ingresá tu contraseña`) is the specific
drift this rule exists to stop. By input type:

- **Text inputs:** `"Ingresá el/la ..."` (`"Ingresá el nombre del producto"`).
- **Search inputs:** `"Buscá ..."` (`"Buscá productos..."`).
- **Select / `Combobox`:** `"Seleccioná un/una ..."` (`"Seleccioná una sucursal"`).
- **Number inputs:** `"Ingresá ..."` (`"Ingresá la cantidad"`).
- **A value the caller invents:** `"Elegí ..."` (`"Elegí una contraseña"`); its
  confirmation, `"Repetí ..."`.
- **Optional/notes:** `"Notas opcionales..."`.

**When the _shape_ matters, the example moves to the hint line** — a CUIT, an
image URL, a hex colour, a unit. The caller has to see the form the value takes,
and a greyed-out example in the box vanishes on the first keystroke, exactly when
they still need it. So the `FormDescription` carries it (`"Son 11 dígitos, con
guiones o sin ellos: 30-70123456-8."`) and the placeholder stays an instruction.
A field whose shape is obvious — an email, a person's name, an amount — gets no
hint at all.

**A field with a default names it on the hint line**, interpolated from the same
source the default comes from (`"Por defecto {count} días."`), never retyped into
the string. The placeholder is still the instruction: an input pre-filled-looking
with a number reads as a value already chosen.

**The hint never repeats the error.** Both are visible at once — see "Validation
messages": the hint keeps only what the message does not say.

### Calls to action and links

- **A CTA is plain text plus a short link, never one long link.** The prompt is a
  muted `<p>` and only the words you act on are the `InlineLink` inside it:
  _¿Todavía no tenés cuenta?_ then _Registrá tu corralón_. A link wrapping the
  whole sentence is what a screen reader announces as the link's name, and it
  leaves the eye nothing to aim at. The linked words must read alone too —
  `"Iniciá sesión"`, never `"acá"`.
- **Link tone is decided by this rule, not per screen.** `InlineLink` ships
  `tone="brand"` (default), `"muted"` and `"danger"`:
  - **brand** on a **terminal** screen, where the link _is_ the next action —
    a sent recovery mail, a completed reset, a confirmed address, an expired
    link's `"Pedir un enlace nuevo"`.
  - **brand** for a **form screen's primary alternative**, when it offers more
    than one — login's `"Registrá tu corralón"`: someone without an account has
    nowhere else to go from that screen.
  - **muted** for every other alternative beside a form — login's
    `"¿Olvidaste tu contraseña?"`, signup's `"Iniciá sesión"`, a resend footer's
    `"Volver a iniciar sesión"`.
  - **danger** only for a destructive action worded as a link; an error's link
    is not danger-toned.
- **A screen has one loud link at most.** Two brand-toned links in one footer is
  the same defect as two primary buttons.

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
- **Only the essential ones.** The bar is: _would a competent reader get this wrong
  without the comment?_ Comment a non-obvious **why**, a constraint that looks
  arbitrary, or a footgun. Never restate the signature, never narrate the steps, never
  label a section the code already names. Prefer one tight line over three. A comment
  that repeats the symbol name is noise and rots on the next edit — delete it rather
  than update it. When in doubt, leave it out; a reviewer asking "why?" is cheaper than
  a file nobody reads.
- **End full-sentence comments with a period.** Exceptions: inline comments on the
  same line as code, single-line `/* ... */` labels, and title-style `//` section
  headers (`// --- Mappers ---`).

## Related skills

- **`web-structure`** — file placement across both apps and the app-vs-`@repo/ui`
  promotion rule (read it first).
- **`ux-motion`** — interaction states, the motion vocabulary, elevation, reduced
  motion. Load it with this skill for any visual change.
- **`agent-workflow`** — branch/ticket flow.
- **`commit`** / **`pr-format`** — commit and PR conventions.
