'use client';

import { usePathname } from 'next/navigation';

/*
 * Each screen fades in on the column's own curve and timing, so the two arrive together. Keyed by
 * path rather than left to a template, which only remounts when the top-level section changes and
 * would let one order or one settings page replace another without it.
 */
export function ScreenFade({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();

  return (
    <div
      key={pathname}
      className="flex min-w-0 flex-1 flex-col animate-in fade-in-0 duration-300 ease-in-out-soft"
    >
      {children}
    </div>
  );
}
