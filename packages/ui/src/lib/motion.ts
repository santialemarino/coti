/*
 * Motion durations, in seconds, for JS-driven animation (motion/react transitions). The CSS side
 * reads the same values from --duration-* in packages/ui/src/styles/index.css; the Tailwind class
 * equivalents are in the table below. Both sides exist because a `transition` prop needs seconds
 * and a utility class needs milliseconds — but there is one set of numbers, and it is this one.
 *
 * | token     | seconds | CSS var            | Tailwind     | use for                                  |
 * | --------- | ------- | ------------------ | ------------ | ---------------------------------------- |
 * | `fast`    | 0.15    | --duration-fast    | duration-150 | icon swaps, hover feedback, chips        |
 * | `default` | 0.2     | --duration-default | duration-200 | most state changes, open/close, reveals  |
 * | `slow`    | 0.3     | --duration-slow    | duration-300 | height reveals, crossfades between steps |
 * | `slower`  | 0.5     | --duration-slower  | duration-500 | page-level entrances                     |
 *
 * Never hardcode a duration at a call site. If a new one is genuinely needed, add it here first.
 */
export const MOTION = {
  fast: 0.15,
  default: 0.2,
  slow: 0.3,
  slower: 0.5,
} as const;

/* Matches --ease-out-soft / --ease-in-out-soft, as motion/react cubic-bezier arrays. */
export const EASE = {
  outSoft: [0.22, 1, 0.36, 1],
  inOutSoft: [0.65, 0, 0.35, 1],
} as const;

/* Delay before a debounced search fires, in milliseconds. */
export const DEBOUNCE_MS = 300;
