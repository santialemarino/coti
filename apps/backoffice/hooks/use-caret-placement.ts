'use client';

import { useCallback, useLayoutEffect, useState } from 'react';

interface Placement {
  input: HTMLInputElement;
  start: number;
  end: number;
}

/*
 * Puts the caret, or a selection, back where an edit left it once the reformatted value has
 * committed. A controlled input whose value was rewritten — or refused, and restored by React —
 * otherwise jumps to the end. Every call renders, so a refused edit that changes nothing still gets
 * its caret back.
 */
export function useCaretPlacement() {
  const [placement, setPlacement] = useState<Placement | null>(null);

  useLayoutEffect(() => {
    placement?.input.setSelectionRange(placement.start, placement.end);
  }, [placement]);

  return useCallback(
    (input: HTMLInputElement, start: number, end = start) => setPlacement({ input, start, end }),
    [],
  );
}
