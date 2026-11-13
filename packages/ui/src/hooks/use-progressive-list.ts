'use client';

import * as React from 'react';

/*
 * Reveals a long list a step at a time, or all of it at once. After a reveal, focus moves onto the
 * first item it added (attach `revealedRef` to it): the control that was pressed may have just
 * disappeared, and focus left on a removed element falls back to the top of the page.
 */
function useProgressiveList<T extends HTMLElement = HTMLElement>(total: number, step: number) {
  const [shown, setShown] = React.useState(step);
  const [revealedFrom, setRevealedFrom] = React.useState<number | null>(null);
  const revealedRef = React.useRef<T | null>(null);
  const visible = Math.min(shown, total);

  React.useEffect(() => {
    if (revealedFrom !== null) revealedRef.current?.focus();
  }, [revealedFrom]);

  function reveal(next: number) {
    setRevealedFrom(visible);
    setShown(next);
  }

  return {
    visible,
    total,
    step,
    revealedFrom,
    revealedRef,
    showMore: () => reveal(visible + step),
    showAll: () => reveal(total),
  };
}

type ProgressiveList<T extends HTMLElement = HTMLElement> = ReturnType<
  typeof useProgressiveList<T>
>;

export { useProgressiveList, type ProgressiveList };
