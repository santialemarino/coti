'use client';

import { useCallback, useLayoutEffect, useState } from 'react';

interface Placement {
  input: HTMLInputElement;
  start: number;
  end: number;
}

// Puts the caret or selection back after a rewritten (or refused) value commits; React would
// otherwise send it to the end. Every call renders, so even an edit that changed nothing gets it.
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
