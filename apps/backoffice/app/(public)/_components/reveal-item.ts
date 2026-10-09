import type { CSSProperties } from 'react';

/*
 * Marks an element as one step of a public-site entrance: it rises in after `index` stagger steps,
 * on first paint or, inside a `Reveal` still below the fold, when that group scrolls into view.
 */
export function revealItem(index: number, className?: string) {
  return {
    className: className ? `reveal-item ${className}` : 'reveal-item',
    style: { '--reveal-index': index } as CSSProperties,
  };
}
