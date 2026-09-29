'use client';

import * as React from 'react';

/*
 * Returns `value` while `open`, and the last value it had while open once `open` goes false — so an
 * overlay animating out keeps showing what the caller saw, even after the caller has cleared it.
 */
function useHeldWhileClosed<T>(value: T, open: boolean): T {
  const held = React.useRef(value);
  if (open) held.current = value;
  return open ? value : held.current;
}

export { useHeldWhileClosed };
