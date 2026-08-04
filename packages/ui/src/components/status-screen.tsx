import * as React from 'react';
import type { LucideIcon } from 'lucide-react';

import { cn } from '../lib/utils';

const TONES = {
  info: { circle: 'bg-accent', icon: 'text-primary' },
  success: { circle: 'bg-success-subtle', icon: 'text-success-foreground' },
  warning: { circle: 'bg-warning-subtle', icon: 'text-warning-foreground' },
  danger: { circle: 'bg-danger-subtle', icon: 'text-danger-foreground' },
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
 * Only the icon animates. The circle, the copy and the actions arrive with the surface that owns the
 * screen — a page transition or an AuthStage crossfade — so the screen contributes one accent to
 * that entrance rather than a second sequence competing with it.
 *
 * A server component with no client JS: the entrance is CSS, and it collapses under
 * `prefers-reduced-motion` via the data-slot gate in styles/index.css.
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
      <span className={cn('grid size-14 place-items-center rounded-full', styles.circle)}>
        <Icon
          aria-hidden="true"
          data-slot="status-screen-icon"
          className={cn('size-7 animate-in zoom-in-50 duration-300 ease-out-soft', styles.icon)}
        />
      </span>

      <div className="flex flex-col gap-y-2">
        <p className="text-heading-5 text-foreground">{title}</p>
        {description ? (
          <p className="text-paragraph-sm text-foreground-muted">{description}</p>
        ) : null}
      </div>

      {children ? <div className="flex flex-col items-center gap-y-3">{children}</div> : null}
    </div>
  );
}

export { StatusScreen };
