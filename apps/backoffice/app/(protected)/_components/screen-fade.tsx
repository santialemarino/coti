'use client';

import { ViewTransition } from 'react';
import { usePathname } from 'next/navigation';

/*
 * The screen crossfades on every navigation — out, then the next one in — on the column's own curve
 * and timing (the `screen` rules in @repo/ui's styles). Keyed by path, so one order or one settings
 * page replacing another fades too; updates inside a screen never animate it.
 */
export function ScreenFade({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();

  return (
    <ViewTransition key={pathname} name="screen" default="none" share="auto">
      <div className="flex min-w-0 flex-1 flex-col">{children}</div>
    </ViewTransition>
  );
}
