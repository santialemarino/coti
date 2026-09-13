import * as React from 'react';
import type { LucideIcon } from 'lucide-react';

import { cn } from '../lib/utils';

interface EmptyStateProps {
  icon: LucideIcon;
  title: string;
  description?: string;
  /*
   * `sm` is the in-place emptiness of a table body or a panel inside a card; `lg` is a whole pane
   * with nothing in it, where the same treatment at the same size reads as a loading glitch.
   */
  size?: 'sm' | 'lg';
  /* A primary action, when the emptiness is something the user can resolve. */
  children?: React.ReactNode;
  className?: string;
}

/*
 * Shown where content would be when there is none — every one of them, so "nothing here" never
 * arrives as a bare sentence in one place and a designed block in another. `whitespace-normal` is
 * explicit because a table cell inherits `whitespace-nowrap`, which would otherwise stop the
 * description wrapping.
 */
function EmptyState({
  icon: Icon,
  title,
  description,
  size = 'sm',
  children,
  className,
}: EmptyStateProps) {
  const large = size === 'lg';

  return (
    <div
      data-slot="empty-state"
      data-size={size}
      className={cn(
        'flex flex-col items-center justify-center px-6 gap-y-3 whitespace-normal text-center',
        large ? 'py-20 gap-y-4' : 'py-12',
        className,
      )}
    >
      <span
        className={cn(
          'grid shrink-0 place-items-center bg-muted rounded-full text-foreground-subtle',
          large ? 'size-16' : 'size-12',
        )}
      >
        <Icon aria-hidden="true" className={large ? 'size-8' : 'size-6'} />
      </span>
      <div className="flex flex-col items-center gap-y-1">
        <p
          className={cn('text-foreground', large ? 'text-heading-6' : 'text-paragraph-sm-semibold')}
        >
          {title}
        </p>
        {description ? (
          <p
            className={cn(
              'max-w-sm text-foreground-muted',
              large ? 'text-paragraph-sm' : 'text-paragraph-xs',
            )}
          >
            {description}
          </p>
        ) : null}
      </div>
      {children}
    </div>
  );
}

export { EmptyState };
