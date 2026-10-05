'use client';

import { useLayoutEffect, useRef, useState } from 'react';

interface FitLabelProps {
  full: string;
  short: string;
}

/*
 * Shows `full` while it fits on one line in the room it is given and `short` once it would be cut.
 * The full text is measured on an invisible copy, so the check still holds while `short` is on show,
 * and it re-runs whenever the room or the text's own width changes (a resize, a late font).
 */
export function FitLabel({ full, short }: FitLabelProps) {
  const box = useRef<HTMLSpanElement>(null);
  const probe = useRef<HTMLSpanElement>(null);
  const [fits, setFits] = useState(true);

  // A layout effect, so the cut label is never painted before it is swapped.
  useLayoutEffect(() => {
    const room = box.current;
    const text = probe.current;
    if (!room || !text) return;
    // Fractional widths: rounded ones can call a text that overflows by a sub-pixel a fit.
    const check = () =>
      setFits(text.getBoundingClientRect().width <= room.getBoundingClientRect().width);
    check();
    const observer = new ResizeObserver(check);
    observer.observe(room);
    observer.observe(text);
    return () => observer.disconnect();
  }, []);

  return (
    // `relative` keeps the probe inside this box, so the trigger's own clip hides it.
    <span ref={box} className="block truncate relative">
      <span ref={probe} aria-hidden="true" className="invisible absolute whitespace-nowrap">
        {full}
      </span>
      {fits ? full : short}
    </span>
  );
}
