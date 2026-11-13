'use client';

import { ViewTransition } from 'react';
import { usePathname } from 'next/navigation';

import { cn } from '@repo/ui/lib';
import { PENDING_SCREEN_CLASS } from '@/app/(protected)/_components/navigation-pending';
import { useNavigationPending } from '@/hooks/use-navigation-pending';

/*
 * The screen crossfades on every navigation — out, then the next one in — on the column's own curve
 * and timing (the `screen` rules in @repo/ui's styles). Keyed by path, so one order or one settings
 * page replacing another fades too; updates inside a screen never animate it.
 */
export function ScreenFade({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();
  const { busy } = useNavigationPending();

  return (
    <ViewTransition key={pathname} name="screen" default="none" share="auto">
      {/* Below lg the context column before it can be the page itself (`data-root`); the screen
          then steps aside. */}
      {/* Dimmed while the next screen loads; it comes back at once, with no transition to run. */}
      <div
        aria-busy={busy || undefined}
        className={cn(
          'flex min-w-0 flex-1 flex-col max-lg:peer-data-[root=true]:hidden',
          busy && PENDING_SCREEN_CLASS,
        )}
      >
        {children}
      </div>
    </ViewTransition>
  );
}
