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

  **The colour shift is not optional**, because the bump is a one-shot: it ends, and once it does
  nothing marks the focused element unless something persistent does. Note the reason is _not_ reduced
  motion — Coti's gate deliberately keeps the bump for a user who asked for less movement, since it is
  functional feedback. This is the one rule in this skill most likely to be forgotten; check it on
  every icon-only control you write.

  **`InlineLink` is the one deliberate exception.** Its focus signal is the bump alone, because focus
  reading differently from hover is the point there, so a focused link is indistinguishable from an
  unfocused one once the bump ends. That is known and accepted — do not add a colour shift to it.

The bump comes in three intensities: `animate-focus-bump` (→1.5, icon-only buttons),
`animate-focus-bump-soft` (→1.15, inline icons), `animate-focus-bump-subtle` (→1.05, text links,
where more movement reads as a glitch).

**A one-shot bump is for a control you focus and leave; a control you sit on and press repeatedly gets
a held scale instead** — `transition-[scale]` plus `group-hover/x:scale-110
group-focus-visible/x:scale-110`. A keyframe that returns to rest stops signalling the moment it ends,
so the password reveal and the search clear hold 1.1 while the dialog close and the sort header, which
you focus once and move on from, keep the 1.5 one-shot.

Keyboard focus must be **as visible as hover**. Tab through what you built.

### hover, active, disabled

- **Pair every hover with a matching focus-visible.** Never signal interactivity with colour alone —
  add the underline, the shape change, or the elevation step too.
- **`active` where a press reads:** `active:bg-primary-active` plus `active:scale-[0.98]` on buttons,
  `active:scale-[0.97]` on segmented items. **`:active` covers pointer and Space, never Enter** —
  browsers fire a button's click on Enter without ever matching `:active`, so an Enter press is
  acknowledged by the focus ring and by whatever the action does, not by the press state. That is
  native behaviour, not a missing style; don't go looking for the bug.
- **`disabled:pointer-events-none disabled:opacity-50`**, and `aria-disabled:` the same where the
  element must stay focusable. Cursor comes free — `button:not([disabled])` gets `cursor-pointer`
  from the base layer.
- **Never disable a control to explain why it is unavailable.** A Radix tooltip does not fire on a
  disabled trigger, so the explanation becomes unreachable exactly when it is needed. Hide the action
  and render something that explains the absence.

### Transitions enumerate their properties

`transition-[color,background-color,border-color,box-shadow] duration-200 ease-out-soft` — not
`transition-all`, which also animates layout and makes a resize look like it is wobbling.

**An enumerated list must name the property the utility actually sets, and for the transform family
that is never `transform`.** Tailwind v4 compiles `scale-*`, `translate-*` and `rotate-*` to the
individual CSS properties `scale`, `translate` and `rotate`:

```css
.scale-50    { …; scale: var(--tw-scale-x) var(--tw-scale-y) }
.rotate-180  { rotate: 180deg }
.-translate-y-1 { translate: 0 calc(var(--spacing) * -1) }
```

So `transition-[opacity,transform]` next to `scale-0`/`scale-100` compiles fine, generates a valid
rule, and animates **nothing** — the crossfade snaps and no tooling warns you. Write
`transition-[opacity,scale]`, `transition-[color,translate]`, `transition-[color,rotate]`.

This is the single easiest mistake to make in this skill and the hardest to see, because the end state
is correct and only the interpolation is missing. Two ways to stay safe:

- **`transition-transform` is fine** — v4 expands it to `transform, translate, scale, rotate`, which
  is why `Switch`'s thumb travels correctly. Reach for it when you are animating the whole family.
- **When you touch any `transition-[…]`, re-read every utility on that element** and confirm each
  animated property is named. Verify in the browser by sampling `getComputedStyle` mid-flight, not by
  reading the class list — a snapped animation and a working one look identical at rest.

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
travels out and back, and `ease-out-back` for something arriving into place, which overshoots its
target and settles. If you need a duration or a curve that isn't here, add a token — don't inline one.

**An overshoot belongs in the easing, never in an extra keyframe stop.** `ease-out-soft` covers most
of its progress in the first third of its duration, so a `60% { scale: 1.06 }` stop is reached in the
first ~50ms of a 420ms animation and then crawls back for the remaining 370ms — which reads as a snap
followed by a drift, not a settle. Two keyframes plus `ease-out-back` keeps the peak mid-flight where
the eye expects it. Note the overshoot scales with the travel distance: `0.75 → 1` under
`--ease-out-back` peaks around 1.027, while `0.8 → 1` peaks at 1.010 and is invisible. Measure it.

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
- **Under reduced motion both directions collapse**, which is also why the exit needs no special
  handling there: Radix decides whether to hold a closing element by reading its computed animation
  name, so `animation: none` unmounts it at once instead of stranding it.
- **Radix `Select` is deliberately not in the design system.** It has no exit presence, so it snaps
  shut. `Combobox` is the one dropdown; reach for it every time.
- **A dialog must keep its content mounted through the exit.** Toggle only `open` and pass the row or
  entity as a stable prop; nulling it on close blanks the body and the dialog visibly empties before
  it fades. `ConfirmDialog` already holds the last entity in a ref — copy that if you build another.
- **Crossfade between mutually exclusive stages** (a form and its result, a loading state and its
  content) with `AnimatePresence mode="wait"`, so the incoming stage waits for the outgoing one.
  `AuthStage` is the reference.
- **A held exit only works when nothing outside the stage changes with it.** `mode="wait"` keeps
  the outgoing children mounted, so anything that swaps in the same state change but renders
  _outside_ the animated box — a step label, a description, the submit button — is already showing
  the next stage while the box still shows the previous one. That frame is incoherent, and a click
  in it acts on the stage the caller cannot see. When siblings change too, make the swap atomic
  (`key` the box and animate the entrance only) and reserve the crossfade for a stage that owns
  everything it changes.
- **A swap that unmounts the focused element drops focus to the body.** Tabbing then restarts from
  the top of the page and a screen reader is told nothing happened, so move focus into the incoming
  stage — onto whatever the caller has to act on. Not on a first render, which would skip the
  heading.
- **A content-driven size change is not a CSS animation.** `width: auto` cannot be transitioned, so no
  `transition-*` will ever smooth a box that resizes because its text changed — it needs motion's
  `layout`. `PendingButton` is the reference.
- **`mode="wait"` and `layout` do not compose.** Under `wait` the incoming child mounts in a later
  commit that the `layout` parent never re-renders for, so it measures nothing and the resize snaps.
  Use `mode="popLayout"`, which pulls the outgoing child out of flow in the same commit the state
  changes, and give the parent `relative` so the popped child is positioned against it.
- **`layout` is the wrong tool for a box that contains a form.** A layout animation scale-corrects
  its children, which squashes inputs mid-flight. Measure the height and animate it directly
  instead — `AnimatedHeight` in the backoffice's auth flow is the reference, and three things in it
  are load-bearing:
  - **Arm on the state change, travel on the resize that follows.** They are not the same moment: an
    atomic swap resizes in the commit its key changes, but a crossfade holds the outgoing stage for
    its exit first, so measuring when the key changes reads the size the box already has and
    concludes there is nothing to animate.
  - **Animate imperatively, from an explicit `[from, to]` pair.** Rendering the old height and then
    the new one is two commits, and the box pins to the first while the second never arrives.
  - **Read the resize through a ref, and never re-create the observer.** Re-observing delivers a
    measurement immediately, and that delivery spends the arm on a size that has not changed yet.
    Release the box to its natural height afterwards, so a message opening under a field still resizes
    it live — that is already smooth, and following it would only make the container lag behind it.
- **Clipping an animated box costs the elevation inside it.** `overflow: hidden` cuts a card's own
  shadow off on all four sides. `overflow: clip` with an `overflow-clip-margin` keeps the clip a few
  pixels outside the border box, so growing content is still contained and the card still reads as
  lifted off the page.
- **A disclosure chevron rotates on open**, driven off the open state, and its transition must name
  **`rotate`** (see the transform-family rule above). Use the shared `DropdownChevron`, not a new one.

## No layout shift

- Micro-interactions animate **transform and opacity only**.
- **Toggling an icon between states stacks both in one grid cell** (`col-start-1 row-start-1`) and
  crossfades with `scale-*`/`opacity-*` — never swap them, which reflows. The password reveal and the
  sort icon are the references.
- **A height reveal is `grid-template-rows` 0fr → 1fr** with `overflow-hidden` on the inner element.
  No JS, no measured pixels. `FormMessage` is the reference.
- **A height animation is only half of an exit.** Whatever is inside has to be held and faded with
  the box, or the words vanish on frame one and an empty box animates its own collapse — which is
  what "the error disappearing is not animated" looks like from the outside, even with the height
  transition working perfectly. Hold the last non-empty body in a ref, fade it with the same duration
  the box uses, and `aria-hidden` the wrapper so a screen reader is not read a rejection that no
  longer applies. `FormMessage` is the reference; the same applies to any panel that empties its
  content on the state change that closes it.
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

- **Decorative motion collapses to nothing** — the status halo, the circle's settle, content rises,
  the Collapsible height reveal, and **every overlay's entrance and exit** (`animate-in` /
  `animate-out`, so a dialog, sheet, popover, menu, tooltip or combobox appears and disappears at
  once). The gate for the shared utilities lives in `packages/ui/src/styles/index.css`; a new
  decorative keyframe must be added to it.
- **Functional feedback stays** — the focus bump, the held focus scale, small hover transitions, and
  the spinner and skeleton, which say "still working" where a frozen one would say "stuck".
  Removing these removes the affordance.
- **Anything that animates from invisible must be visible at rest.** With `animation: none` the
  element renders at its base style, so a one-shot that ends invisible needs that end state as its
  base, and anything relying on `both` fill-mode to start hidden must not — or reduced-motion users
  get blank copy.
- **JS-driven motion** uses `useReducedMotion()` from `motion/react` and keeps the same tree, zeroing
  the timings (`duration: reduced ? 0 : MOTION.default`) and dropping `layout`. Do not swap in a plain
  element: motion inlines its own styles during SSR and a swapped element strands them, which is how a
  label ends up permanently invisible.

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
