'use client';

import { useState } from 'react';
import { usePathname } from 'next/navigation';

import { surfaceOf } from '@/config/routes';

/*
 * Fades the whole page in when a navigation crosses between the public site, the sign-in screens and
 * the app. Never on the first render — a page load is not a crossing — and never within one surface,
 * where the app's own screen swap already runs.
 */
export function SurfaceFade({ children }: { children: React.ReactNode }) {
  const surface = surfaceOf(usePathname());
  const [shown, setShown] = useState({ surface, crossed: false });

  // Derived during render, React's way of reacting to a prop change without an effect.
  if (shown.surface !== surface) setShown({ surface, crossed: true });

  return (
    <div key={surface} className={shown.crossed ? 'animate-surface-in' : undefined}>
      {children}
    </div>
  );
}
