'use client';

import * as React from 'react';

/* Whether the enclosing Dialog or Sheet is open, for its content to hold itself through the exit. */
const OverlayOpenContext = React.createContext(false);

interface OverlayRootState {
  open?: boolean;
  defaultOpen?: boolean;
  onOpenChange?: (open: boolean) => void;
}

/* Mirrors Radix's controlled/uncontrolled contract, so the root knows `open` in both modes. */
function useOverlayRootState({
  open: openProp,
  defaultOpen = false,
  onOpenChange,
}: OverlayRootState) {
  const [uncontrolled, setUncontrolled] = React.useState(defaultOpen);
  const open = openProp ?? uncontrolled;

  const handleOpenChange = React.useCallback(
    (next: boolean) => {
      if (openProp === undefined) setUncontrolled(next);
      onOpenChange?.(next);
    },
    [openProp, onOpenChange],
  );

  return { open, onOpenChange: handleOpenChange };
}

export { OverlayOpenContext, useOverlayRootState };
