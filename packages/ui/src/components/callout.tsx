import * as React from 'react';
import { AlertTriangleIcon, CheckCircle2Icon, InfoIcon, XCircleIcon } from 'lucide-react';

import { cn } from '../lib/utils';

const TONES = {
  info: { surface: 'bg-accent border-brand-200', icon: 'text-primary', Icon: InfoIcon },
  success: {
    surface: 'bg-success-subtle border-success-border',
    icon: 'text-success-foreground',
    Icon: CheckCircle2Icon,
  },
  warning: {
    surface: 'bg-warning-subtle border-warning-border',
    icon: 'text-warning-foreground',
    Icon: AlertTriangleIcon,
  },
  danger: {
    surface: 'bg-danger-subtle border-danger-border',
    icon: 'text-danger-foreground',
    Icon: XCircleIcon,
  },
} as const;

interface CalloutProps extends React.ComponentProps<'div'> {
  tone?: keyof typeof TONES;
  title?: string;
  /* Overrides the tone's default glyph. */
  icon?: React.ComponentType<{ className?: string; 'aria-hidden'?: boolean }>;
}

/*
 * A standing message about the surrounding content — an explanation, a caveat, a result. Not for
 * form validation (that is FormMessage, which animates in place) and not for a transient
 * confirmation (that is a toast).
 *
 * `danger` announces itself to assistive tech, because a message the user must not miss should not
 * depend on them looking at it.
 */
function Callout({ className, tone = 'info', title, icon, children, ...props }: CalloutProps) {
  const { surface, icon: iconColor, Icon: DefaultIcon } = TONES[tone];
  const Icon = icon ?? DefaultIcon;

  return (
    <div
      data-slot="callout"
      data-tone={tone}
      role={tone === 'danger' ? 'alert' : undefined}
      className={cn(
        'flex w-full items-start px-3 py-2.5 gap-x-2.5 border rounded-xl',
        surface,
        className,
      )}
      {...props}
    >
      <Icon aria-hidden className={cn('mt-0.5 size-4 shrink-0', iconColor)} />
      <div className="flex flex-col min-w-0 gap-y-0.5">
        {title ? <p className="text-paragraph-sm-semibold text-foreground">{title}</p> : null}
        <div className="text-paragraph-sm text-foreground-muted">{children}</div>
      </div>
    </div>
  );
}

export { Callout };
