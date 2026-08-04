import * as React from 'react';
import type { LucideIcon } from 'lucide-react';

import { cn } from '../lib/utils';

const TONES = {
  info: { circle: 'bg-accent', icon: 'text-primary', halo: 'bg-brand-300' },
  success: { circle: 'bg-success-subtle', icon: 'text-success-foreground', halo: 'bg-success' },
  warning: { circle: 'bg-warning-subtle', icon: 'text-warning-foreground', halo: 'bg-warning' },
  danger: { circle: 'bg-danger-subtle', icon: 'text-danger-foreground', halo: 'bg-danger' },
} as const;

interface StatusScreenProps {
  icon: LucideIcon;
  tone: keyof typeof TONES;
  title: string;
  description?: string;
  /* Actions under the copy — a link back, a retry button. Enters last. */
  children?: React.ReactNode;
  className?: string;
}

/*
 * The shared outcome screen: a check-your-email notice, a completed reset, an expired link, a
 * submitted RFQ. One visual language for every terminal state in both apps.
 *
 * The entrance is staggered rather than simultaneous — the icon lands, then the title, then the
 * copy, then the actions — so the eye is led through the result instead of being handed all of it
 * at once. Delays are inline arbitrary properties because they are per-element positions in one
 * sequence, not reusable tokens.
 *
 * A server component with no client JS: the whole sequence is CSS, and it collapses to a plain
 * static layout under `prefers-reduced-motion` (the gate lives in styles/index.css). The halo's base
 * opacity is 0 for exactly that reason — with the animation removed it must leave no stray ring.
 */
function StatusScreen({
  icon: Icon,
  tone,
  title,
  description,
  children,
  className,
}: StatusScreenProps) {
  const styles = TONES[tone];

  return (
    <div
      data-slot="status-screen"
      className={cn('flex flex-col items-center px-6 gap-y-5 text-center', className)}
    >
      <div className="relative grid place-items-center">
        <span
          aria-hidden="true"
          className={cn(
            'absolute size-14 rounded-full opacity-0 animate-status-halo [animation-delay:120ms]',
            styles.halo,
          )}
        />
        <span
          className={cn(
            'relative grid size-14 place-items-center rounded-full animate-status-pop',
            styles.circle,
          )}
        >
          <Icon aria-hidden="true" className={cn('size-7', styles.icon)} />
        </span>
      </div>

      <div className="flex flex-col gap-y-2">
        <p className="text-heading-5 text-foreground animate-rise-in [animation-delay:120ms]">
          {title}
        </p>
        {description ? (
          <p className="text-paragraph-sm text-foreground-muted animate-rise-in [animation-delay:180ms]">
            {description}
          </p>
        ) : null}
      </div>

      {children ? (
        <div className="flex flex-col items-center gap-y-3 animate-rise-in [animation-delay:240ms]">
          {children}
        </div>
      ) : null}
    </div>
  );
}

export { StatusScreen };
