'use client';

import { createContext, useContext } from 'react';

export interface NavigationPendingState {
  // The destination's path while a navigation to another page runs, so the nav marks it at once.
  pendingPathname: string | null;
  // Running past the delay: the progress bar shows.
  showProgress: boolean;
  // As `showProgress`, for another page: the old screen dims and is busy.
  busy: boolean;
  // Reports a navigation a link click does not reveal; returns its id for `settle`.
  begin: (href: string) => number;
  // The navigation `id` ended without moving the URL.
  settle: (id: number) => void;
}

// Outside the protected shell there is nowhere to show anything: a no-op.
const OUTSIDE_SHELL: NavigationPendingState = {
  pendingPathname: null,
  showProgress: false,
  busy: false,
  begin: () => -1,
  settle: () => undefined,
};

export const NavigationPendingContext = createContext<NavigationPendingState>(OUTSIDE_SHELL);

// The navigation in flight, as `NavigationPendingProvider` sees it.
export function useNavigationPending(): NavigationPendingState {
  return useContext(NavigationPendingContext);
}
