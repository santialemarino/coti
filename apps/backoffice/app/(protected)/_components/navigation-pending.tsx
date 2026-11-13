'use client';

import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react';
import { usePathname, useSearchParams } from 'next/navigation';

import { NAVIGATION_PENDING_DELAY_MS } from '@repo/ui/lib';
import { NavigationPendingContext, useNavigationPending } from '@/hooks/use-navigation-pending';
import {
  activeNavigation,
  beginNavigation,
  linkDestination,
  pathAndQuery,
  pathOf,
  settleNavigation,
  type PendingNavigation,
} from '@/lib/navigation-pending';

// Applied to the old screen while a navigation to another page runs past the delay.
export const PENDING_SCREEN_CLASS =
  'pointer-events-none opacity-60 transition-opacity duration-150 ease-out-soft motion-reduce:transition-none';

/*
 * The shell's navigation feedback; there is no route loading state, so the old screen stays until the
 * new one is ready. The destination's nav item turns active on the click; past the delay the progress
 * bar shows and, for another page, the old screen dims. It sees every in-app link click (captured on
 * the document), back and forward, and pushes reported through `usePendingRouter`.
 */
export function NavigationPendingProvider({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const query = useSearchParams().toString();
  const current = query ? `${pathname}?${query}` : pathname;

  const [pending, setPending] = useState<PendingNavigation | null>(null);
  const [shownId, setShownId] = useState<number | null>(null);
  const currentRef = useRef(current);
  const lastId = useRef(0);

  // Ends in the render the URL commits, so the new screen never paints dimmed.
  const active = activeNavigation(pending, current);
  const showProgress = active !== null && shownId === active.id;

  useLayoutEffect(() => {
    currentRef.current = current;
  }, [current]);

  const begin = useCallback((href: string) => {
    const id = ++lastId.current;
    const to = pathAndQuery(new URL(href, window.location.href));
    setPending(beginNavigation(id, currentRef.current, to));
    return id;
  }, []);

  const settle = useCallback((id: number) => {
    setPending((previous) => settleNavigation(previous, id));
  }, []);

  useEffect(() => {
    const onClick = (event: MouseEvent) => {
      const anchor = (event.target as Element | null)?.closest?.('a[href]');
      if (!(anchor instanceof HTMLAnchorElement)) return;
      const to = linkDestination(event, anchor, new URL(window.location.href));
      if (to !== null) begin(to);
    };
    // The browser already shows the destination's URL; the app has not committed it yet.
    const onPopState = () => begin(window.location.href);
    document.addEventListener('click', onClick, true);
    window.addEventListener('popstate', onPopState);
    return () => {
      document.removeEventListener('click', onClick, true);
      window.removeEventListener('popstate', onPopState);
    };
  }, [begin]);

  // Dropped once over, so a later navigation from this URL starts clean.
  useEffect(() => {
    if (pending && !active) setPending(null);
  }, [pending, active]);

  useEffect(() => {
    if (!active) return;
    const timer = setTimeout(() => setShownId(active.id), NAVIGATION_PENDING_DELAY_MS);
    return () => clearTimeout(timer);
  }, [active]);

  const value = useMemo(
    () => ({
      pendingPathname: active?.page ? pathOf(active.to) : null,
      showProgress,
      busy: active !== null && showProgress && active.page,
      begin,
      settle,
    }),
    [active, showProgress, begin, settle],
  );

  return (
    <NavigationPendingContext.Provider value={value}>{children}</NavigationPendingContext.Provider>
  );
}

// The protected shell's outer frame: the provider, the bar, and a box for the rest of the shell.
export function NavigationPendingShell({
  className,
  children,
}: {
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <NavigationPendingProvider>
      <NavigationProgressBar />
      <div className={className}>{children}</div>
    </NavigationPendingProvider>
  );
}

// A 2px bar across the top of the viewport, over the header; it takes no pointer input.
export function NavigationProgressBar() {
  const { showProgress } = useNavigationPending();
  if (!showProgress) return null;

  return (
    <div
      aria-hidden="true"
      data-slot="navigation-progress"
      className="h-0.5 fixed inset-x-0 top-0 z-50 overflow-hidden pointer-events-none"
    >
      <div className="w-1/3 h-full bg-primary animate-navigation-progress" />
    </div>
  );
}
