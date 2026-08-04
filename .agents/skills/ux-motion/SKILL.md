---
name: ux-motion
description: Interaction states (focus-visible, hover, active, disabled), motion and animation, elevation, and reduced motion for Coti's frontend. Use when creating or restyling any component or page in apps/backoffice, apps/webapp, or packages/ui.
---

# UX and motion (Coti frontend)

Applies to every component and page in `apps/backoffice`, `apps/webapp` and `packages/ui`. Load
alongside `web-components-pages` (how to build it) and `web-structure` (where it goes).

The token reference — the ramps, the contrast figures, the full utility tables — is
[docs/technical/design-system.md](../../../docs/technical/design-system.md). This skill is the rules.

## Coti's motion has one register

Both apps are tools, not marketing. The backoffice is where a rep works all day; the webapp is where
a customer reads a quote and answers it. Neither wants spectacle. **Motion here is feedback and
orientation only** — a state change that would otherwise be a hard cut, a reveal that would otherwise
snap, an entrance that orients someone on a result screen.

There is no scroll-reveal, no parallax, no ambient loop, no decorative hover-float anywhere in Coti.
If a motion does not answer "what just happened?" or "where am I?", it does not ship.

## Interaction states

Every interactive element gets **all four** that apply: hover, active, focus-visible, disabled.
Audit each one you add. A control with a hover and no focus-visible is unfinished.

### focus-visible — never the browser default, and never colour alone

Always `outline-none`, then replace it. There are exactly two treatments, chosen by what the element
_is_:

- **Surfaces** — buttons, inputs, selects, cards you can click, segmented items, table rows you can
  activate. These get **the ring**:
  `outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/45`.
  For an input the ring belongs on the bordered container via `focus-within:`, not on the `<input>`,
  because the ring has to trace the box the user sees. Variant-tinted rings follow the variant
  (`focus-visible:ring-danger/40` on a destructive button). Inside a segmented group add
  `focus-visible:z-10`, or the neighbour clips the ring.
- **Icon-only and inline text triggers** — a dialog's ✕, a row action, a reveal toggle, a sort header,
  a text link. These get **a colour shift plus the bump**, and no rectangular ring:

  ```tsx
  // The focusable element owns a named group; the icon inside carries the bump.
  className="group/clear outline-none transition-colors duration-200 ease-out-soft
             text-foreground-subtle hover:text-foreground focus-visible:text-foreground"
  //   …and on the icon:
  className="group-focus-visible/clear:animate-focus-bump"
  ```

  **The colour shift is not optional.** `prefers-reduced-motion` removes the bump, and a focus
  indicator that disappears for a user who needs less motion is not a focus indicator. This is the
  one rule in this skill most likely to be forgotten — check it on every icon-only control you write.

The bump comes in three intensities: `animate-focus-bump` (→1.5, icon-only buttons),
`animate-focus-bump-soft` (→1.15, inline icons), `animate-focus-bump-subtle` (→1.05, text links,
where more movement reads as a glitch).

Keyboard focus must be **as visible as hover**. Tab through what you built.

### hover, active, disabled

- **Pair every hover with a matching focus-visible.** Never signal interactivity with colour alone —
  add the underline, the shape change, or the elevation step too.
- **`active` where a press reads:** `active:bg-primary-active` plus `active:scale-[0.98]` on buttons,
  `active:scale-[0.97]` on segmented items.
- **`disabled:pointer-events-none disabled:opacity-50`**, and `aria-disabled:` the same where the
  element must stay focusable. Cursor comes free — `button:not([disabled])` gets `cursor-pointer`
  from the base layer.
- **Never disable a control to explain why it is unavailable.** A Radix tooltip does not fire on a
  disabled trigger, so the explanation becomes unreachable exactly when it is needed. Hide the action
  and render something that explains the absence.

### Transitions enumerate their properties

`transition-[color,background-color,border-color,box-shadow] duration-200 ease-out-soft` — not
`transition-all`, which also animates layout and makes a resize look like it is wobbling.

## Motion tokens — never hardcode a duration

`duration-150` / `duration-200` / `duration-300` / `duration-500` in classes; `MOTION.fast` /
`.default` / `.slow` / `.slower` from `@repo/ui/lib` for `motion/react`; `--duration-*` and
`--ease-*` in raw CSS. One set of numbers, three consumers.

| Use                                      | Duration |
| ---------------------------------------- | -------- |
| icon swaps, hover feedback, chips        | 150ms    |
| most state changes, open/close, reveals  | 200ms    |
| height reveals, crossfades between steps | 300ms    |
| page-level entrances                     | 500ms    |

Easing is `ease-out-soft` for entrances and most state changes, `ease-in-out-soft` for something that
travels out and back. If you need a duration that isn't here, add a token — don't inline a number.

## Open and close are both animations

**Close is the direction users see most, and the one most often left out.** Every overlay in
`@repo/ui` already animates both ways; if you build one, declare both:

```
data-[state=open]:animate-in  data-[state=open]:fade-in-0  data-[state=open]:zoom-in-95
data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95
```

Directional slides need the exit too (`slide-out-to-top-2` alongside `slide-in-from-top-2`), or the
panel enters from the trigger and then dissolves in place.

- **Reuse the primitives** — `Dialog`, `Popover`, `DropdownMenu`, `Sheet`, `Tooltip`, `Collapsible`,
  `Combobox`. Don't hand-roll an overlay.
- **Radix `Select` is deliberately not in the design system.** It has no exit presence, so it snaps
  shut. `Combobox` is the one dropdown; reach for it every time.
- **A dialog must keep its content mounted through the exit.** Toggle only `open` and pass the row or
  entity as a stable prop; nulling it on close blanks the body and the dialog visibly empties before
  it fades. `ConfirmDialog` already holds the last entity in a ref — copy that if you build another.
- **Crossfade between mutually exclusive stages** (a form and its result, a loading state and its
  content) with `AnimatePresence mode="wait"`, so the incoming stage waits for the outgoing one.
  `AuthStage` is the reference.
- **A disclosure chevron rotates on open**, driven off the open state, and its transition must name
  **`rotate`** — in Tailwind v4 `rotate-180` sets the `rotate` property, so a `transform`-only
  transition leaves it snapping. Use the shared `DropdownChevron` rather than a new one.

## No layout shift

- Micro-interactions animate **transform and opacity only**.
- **Toggling an icon between states stacks both in one grid cell** (`col-start-1 row-start-1`) and
  crossfades with `scale-*`/`opacity-*` — never swap them, which reflows. The password reveal and the
  sort icon are the references.
- **A height reveal is `grid-template-rows` 0fr → 1fr** with `overflow-hidden` on the inner element.
  No JS, no measured pixels. `FormMessage` is the reference.
- **Cancel the parent's gap when a collapsible block sits in a flex column.** A collapsed
  height-animated child still contributes the column's `gap`, so it leaves a permanent band of empty
  space. Give the wrapper a negative margin equal to the gap and restore the gap as padding _inside_
  the animated box (`-mt-2` + `pt-2` for a `gap-y-2` parent). Getting this wrong is the difference
  between a form that breathes and one with a mystery 8px under every field.
- **Reserve space for conditional content** and use fixed icon sizes.
- **A hover-lift must not move the element that owns the `:hover`.** When it does, the pointer falls
  outside it at the boundary and the state oscillates hover→un-hover forever. Drive the lift from a
  stationary wrapper and move an inner layer. `Card`'s `interactive` deliberately animates elevation
  and border only, for this reason.

## Elevation

Four tokens, tinted with the brand ink: `shadow-e1` (control hairline) → `shadow-e2` (resting card) →
`shadow-e3` (dropdown, popover, hover-lifted card) → `shadow-e4` (dialog, sheet). Never hand-roll a
`shadow-[...]`, and don't skip steps to signal importance — depth means distance from the page, not
priority.

## Reduced motion is a contract, not a courtesy

Honour `prefers-reduced-motion: reduce`. The split:

- **Decorative motion collapses to nothing** — the status halo, the icon pop, content rises, the
  Collapsible height reveal. The gate for the shared utilities is already in
  `packages/ui/src/styles/index.css`; a new decorative keyframe must be added to it.
- **Functional feedback stays** — the focus bump, small hover transitions. Removing these removes the
  affordance.
- **A one-shot animation that ends invisible must also be invisible at rest.** With `animation: none`
  the element renders at its base style, so `animate-status-halo` carries `opacity-0`: without it the
  halo would sit there as a permanent stray ring. Equally, anything relying on `both` fill-mode to
  start hidden must be visible at rest, or reduced-motion users get blank copy.
- **JS-driven motion** uses `useReducedMotion()` from `motion/react` and swaps the whole props object
  for a static one, rather than conditionally rendering a plain element — motion inlines `opacity: 0`
  during SSR, and a plain element would keep it and stay invisible.

## Verify what you built

For any UI change, drive it in a real browser with `playwright-cli` (see the `playwright-cli` skill)
rather than reasoning about it. Check, at minimum:

- **Tab through every interactive element.** A ring or a bump on each, no native outline left, nothing
  skipped.
- **Open and close every overlay.** Confirm the exit animates — inspect `data-state="closed"` and
  `getAnimations()` if in doubt, because a missing exit looks like a fast close.
- **Emulate `prefers-reduced-motion: reduce`.** Decorative motion gone, no stray artefacts, copy
  visible, focus feedback intact.
- **Check contrast** for any new colour pair against `docs/technical/design-system.md`; if a pair
  isn't in the table, compute it before shipping.

## Related skills

- **`web-components-pages`** — component authoring, the type scale, Tailwind class order, forms.
- **`web-structure`** — file placement and the `@repo/ui` promotion rule.
- **`playwright-cli`** — driving the browser.
