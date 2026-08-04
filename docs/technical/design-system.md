# Design system

The shared design system is `packages/ui` (`@repo/ui`): one set of tokens, one type
scale, one motion vocabulary, consumed by both `apps/backoffice` and `apps/webapp`.

## The stylesheet pipeline

`packages/ui/src/styles/index.css` is the **single Tailwind entry for the monorepo**.
It imports Tailwind and `tw-animate-css`, pulls in `theme.css`, declares the tokens,
and is compiled by the Tailwind CLI to `packages/ui/dist/index.css`.

```
packages/ui/src/styles/index.css   ← the entry: @import 'tailwindcss', tokens, base, motion
packages/ui/src/styles/theme.css   ← @theme inline: maps tokens onto Tailwind namespaces
                ↓ tailwindcss CLI
packages/ui/dist/index.css         ← the built bundle (gitignored)
                ↓ @import '@repo/ui/styles'
apps/<app>/app/globals.css         ← nothing else
```

Each app's `globals.css` imports **only** `@repo/ui/styles`. It must not import
`tailwindcss` again: the built bundle already contains preflight and the utility
layer, so a second import emits both twice.

Two consequences worth knowing:

- **Class scanning is monorepo-wide from inside the package, and explicit.** `index.css`
  declares `@source "../../**/*.{ts,tsx}"` and `@source "../../../../apps/**/*.{ts,tsx}"`,
  so a class used only in an app is still generated. Tailwind's automatic detection is
  turned off (`@import 'tailwindcss' source(none)`) so those two globs are the whole
  input — otherwise it also scans prose, and a class name merely _mentioned_ in a README
  or a skill would end up in the bundle.
- **`@repo/ui` ships its CSS prebuilt.** Components are consumed from `src` (their
  logic is live), but styles come from `dist`. After changing a `packages/ui`
  component's classNames, rebuild (`pnpm --filter @repo/ui build`, or rely on its
  `dev` watcher) — otherwise the new classes never reach the app and the change
  silently does nothing. `pnpm dev` builds `@repo/ui` before starting the apps, so a
  cold start can't race it.

## Colour

### The brand ramp is measured from the logo

The mark is one azure hue family at three lightnesses: chroma holds near **0.130**
while hue rotates from **228°** at the light end to **260°** at the ink. The ramp
reproduces that drift rather than pinning a single hue, and four steps are exact
logo pixels:

| Token       | oklch                      | Hex       | Source                       |
| ----------- | -------------------------- | --------- | ---------------------------- |
| `brand-400` | `oklch(0.746 0.132 228.2)` | `#35BCEE` | isotype gradient, light stop |
| `brand-500` | `oklch(0.655 0.130 239)`   | `#309AD7` | the dot over the i           |
| `brand-600` | `oklch(0.527 0.129 254.2)` | `#2F6CB3` | isotype gradient, dark stop  |
| `brand-950` | `oklch(0.272 0.043 259.8)` | `#1A273C` | the wordmark ink             |

The remaining steps (`50`–`300`, `700`–`900`) are interpolated along the same curve,
with chroma tapered at both ends to stay inside sRGB.

No Tailwind palette ramp is a substitute: `sky-*` is ~30% more chromatic and 8°
toward cyan, `blue-*` ~65% more chromatic and 15° toward violet. Both read visibly
wrong next to the mark.

Neutrals (`neutral-0`–`neutral-900`) carry a trace of the brand hue (chroma ≤ 0.031
at hue 258) so greys read as related to the brand rather than dirty.

### Use semantic tokens, not ramp steps

Components consume the semantic layer. Reach for `brand-*` only when building a
deliberately brand-coloured surface.

| Token                                            | Resolves to                            | For                         |
| ------------------------------------------------ | -------------------------------------- | --------------------------- |
| `background` / `foreground`                      | white / `brand-950`                    | page base and body ink      |
| `body-background`                                | `oklch(0.974 0.007 240)`               | the wash cards sit on       |
| `card` / `card-foreground`, `popover` / …        | white / `brand-950`                    | raised surfaces             |
| `sunken`                                         | `neutral-100`                          | table headers, inset panels |
| `foreground-muted` / `foreground-subtle`         | `neutral-500` / `neutral-400`          | secondary and tertiary copy |
| `primary` / `primary-foreground`                 | `brand-600` / white                    | the main action             |
| `primary-hover` / `primary-active`               | `brand-700` / `brand-800`              | its pressed states          |
| `secondary` / `secondary-hover`                  | `neutral-100` / `neutral-200`          | the neutral action          |
| `accent` / `accent-strong` / `accent-foreground` | `brand-50` / `brand-100` / `brand-700` | tinted hover surfaces       |
| `muted` / `muted-foreground`                     | `neutral-100` / `neutral-500`          | quiet fills and copy        |
| `border` / `border-strong`                       | `neutral-200` / `neutral-300`          | hairlines                   |
| `input` / `input-readonly`                       | `neutral-50` / `neutral-100`           | field fills                 |
| `ring`                                           | `brand-500`                            | the focus ring              |
| `backdrop`                                       | ink at 55%                             | dialog scrim                |

### Status families

`success`, `warning` and `danger` each ship four steps — `-subtle` (tinted surface),
`-border` (its hairline), the base (`bg-success`, for fills and icons) and
`-foreground` (text). `destructive-*` aliases `danger-*` so shadcn primitives and
`aria-invalid` styling drop in unchanged.

Every `-foreground` clears WCAG AA on white, and white clears AA on `bg-danger`:

| Pair                              | Contrast |
| --------------------------------- | -------- |
| `foreground` on white             | 14.99:1  |
| `primary-foreground` on `primary` | 5.37:1   |
| `muted-foreground` on white       | 4.73:1   |
| `success-foreground` on white     | 4.67:1   |
| `warning-foreground` on white     | 4.75:1   |
| `danger-foreground` on white      | 5.49:1   |
| white on `bg-danger`              | 4.67:1   |

`-base` values are tuned for fills and icons, not for text — `success-base` and
`warning-base` do not carry text contrast. Use `-foreground` for copy.

**Mapping a domain enum to a tone is the app's job, not the design system's.** A quote
status (`GENERATED`, `QUOTED`, `SENT`, …) maps to a `Badge` tone in the app that owns
the enum; `@repo/ui` only ships the tones.

### Light-only, on purpose

There is no `.dark` token block and no `dark:` variant. A registered-but-unsatisfiable
variant invites `dark:` classes that can never match, which is worse than no dark mode
at all. Adding it later is a token block plus a provider, not a rewrite.

## Typography

Two faces, both via `next/font/google`, applied as CSS variables on `<html>` by each
app's root layout (`apps/<app>/lib/fonts.ts`):

- **Inter** (`--font-inter` → `--font-sans`) — the workhorse. It holds up at the 11–12px
  the quote and catalog tables run at, where a geometric face does not.
- **Poppins** (`--font-poppins` → `--font-display`) — geometric like the wordmark, and
  carries the heading scale. Weights 500/600 only, since Poppins is not variable on
  Google Fonts.

### The scale

Headings are `@utility` classes rather than `--text-*` theme tokens, because the display
family is part of a heading's identity and Tailwind's text scale cannot carry a
`font-family`. One class therefore sets family, size, weight, line-height and tracking
together — there is no way to apply a heading size and forget the face.

| Heading          | Size | Weight | Line height | Tracking |
| ---------------- | ---- | ------ | ----------- | -------- |
| `text-heading-1` | 40px | 600    | 1.1         | −0.02em  |
| `text-heading-2` | 32px | 600    | 1.15        | −0.02em  |
| `text-heading-3` | 24px | 600    | 1.2         | −0.015em |
| `text-heading-4` | 20px | 600    | 1.25        | −0.01em  |
| `text-heading-5` | 18px | 600    | 1.3         | −0.01em  |
| `text-heading-6` | 16px | 600    | 1.35        | —        |

Paragraphs are theme tokens and inherit Inter. Each size comes in three weights —
`text-paragraph{,-medium,-semibold}`, and the same for `-sm` (14px), `-xs` (12px) and
`-mini` (11px).

Never write a raw `text-sm` / `font-medium` pair, and never add a `font-*` weight next
to a scale token: the token already encodes the weight.

Both scales are registered in `cn()`'s tailwind-merge `font-size` group
(`packages/ui/src/lib/utils.ts`). Without that, two scale classes on one element would
both survive and the loser would win on source order instead of the last one winning.
**Adding a scale token means adding it there too.**

## Elevation

Four steps, tinted with the brand ink so a shadow reads as depth rather than dirt.
Registered in `cn()`'s `shadow` group alongside the type scale.

| Token       | For                                      |
| ----------- | ---------------------------------------- |
| `shadow-e1` | field and control hairline lift          |
| `shadow-e2` | a resting card                           |
| `shadow-e3` | dropdowns, popovers, a hover-lifted card |
| `shadow-e4` | dialogs and sheets                       |

## Radius

`--radius` is 10px and is the control baseline. Controls (buttons, inputs, selects) use
`rounded-lg`; cards use `rounded-1.5xl` (14px); badges and pills use `rounded-full`.
Nothing is fully square unless that is a deliberate decision — audit every box you add.

## Motion

Durations and easings are tokens, and the JS and CSS sides read the same numbers:
`MOTION` / `EASE` in `@repo/ui/lib` for `motion/react` transitions, `--duration-*` and
`--ease-*` in CSS, `duration-150/200/300/500` and `ease-out-soft` / `ease-in-out-soft`
as utilities. Never hardcode a duration; add a token if a new one is genuinely needed.
The keyframe animations below deliberately sit outside this scale — a keyframe's duration
is part of its character, not a reusable step.

| Token     | Seconds | Utility        | For                                      |
| --------- | ------- | -------------- | ---------------------------------------- |
| `fast`    | 0.15    | `duration-150` | icon swaps, hover feedback, chips        |
| `default` | 0.2     | `duration-200` | most state changes, open/close, reveals  |
| `slow`    | 0.3     | `duration-300` | height reveals, crossfades between steps |
| `slower`  | 0.5     | `duration-500` | page-level entrances                     |

### Animation utilities

Declared as `--animate-*` theme tokens, which is what makes `focus-visible:` and
`group-focus-visible/<name>:` variants of them generate.

| Utility                     | Motion            | For                               |
| --------------------------- | ----------------- | --------------------------------- |
| `animate-focus-bump`        | 1 → 1.5 → 1       | icon-only triggers                |
| `animate-focus-bump-soft`   | 1 → 1.15 → 1      | inline icons                      |
| `animate-focus-bump-subtle` | 1 → 1.05 → 1      | text links                        |
| `animate-status-pop`        | 0.4 → 1.06 → 1    | a status screen's icon            |
| `animate-status-halo`       | 0.6 → 1.9, fading | the one-shot ring behind it       |
| `animate-rise-in`           | +6px, fading in   | copy entering under a status icon |

Radix open/close, fades, zooms and slides come from `tw-animate-css` — don't
re-declare those.

### Reduced motion

Decorative motion (`status-halo`, `status-pop`, `rise-in`, the Collapsible height
reveal) collapses to nothing under `prefers-reduced-motion: reduce`. The focus-bump
family deliberately does **not**, because it is functional feedback — and every element
carrying a bump also shifts colour on `focus-visible`, so the affordance survives for a
user who can't perceive the bump. See the `ux-motion` skill for the interaction-state
rules that go with these tokens.

The gate is intentionally unlayered: a normal declaration outside any cascade layer
beats every `@layer utilities` rule regardless of specificity or source order, which is
what lets it override `tw-animate-css`'s own ungated `animate-collapsible-*` utilities.
It matches on an attribute substring so it holds under any Tailwind variant.

## Brand assets

Generated from the 2481px logo masters. Both apps carry their own copy, since Next
serves static assets per app.

| Path                                 | Size    | Wiring                                    |
| ------------------------------------ | ------- | ----------------------------------------- |
| `app/favicon.ico`                    | 16–64   | App Router convention                     |
| `app/icon.png`                       | 512     | App Router convention                     |
| `app/apple-icon.png`                 | 180     | App Router convention (opaque background) |
| `public/icons/icon-{192,512}.png`    | 192/512 | `manifest.ts`, `purpose: any`             |
| `public/icons/icon-maskable-512.png` | 512     | `manifest.ts`, `purpose: maskable`        |
| `public/brand/isotype.png`           | h144    | the mark alone                            |
| `public/brand/wordmark.png`          | h132    | "Coti"                                    |
| `public/brand/lockup.png`            | h288    | mark over wordmark                        |
| `public/brand/tagline.png`           | w720    | "COTIZÁ. CONVERTÍ. CERRÁ."                |

Served art is exported at 3× the largest size the UI renders it, so it stays crisp on
any display.

Two size-specific decisions:

- **The 16px and 32px favicon frames come from a gap-closed silhouette.** The mark is a
  segmented C; below ~48px those gaps go sub-pixel and turn it into a blur. The small
  frames morphologically close the gaps and refill with the mark's own
  `brand-400 → brand-600` vertical gradient. 48 and 64 keep the real segmented mark.
- **`apple-icon.png` is opaque** (`brand-50`), because iOS composites a transparent
  apple-icon onto black. `icon-maskable-512.png` is opaque and more heavily padded,
  because Android crops it to its own shape.

There is deliberately **no Open Graph image**. It would need an absolute `metadataBase`,
so a new web-origin env var, and neither surface benefits: the backoffice is
authenticated, and a webapp quote link is a private tokenized URL that should not
produce a rich preview.
