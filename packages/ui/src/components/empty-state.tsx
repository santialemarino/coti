import * as React from 'react';
import type { LucideIcon } from 'lucide-react';

import { cn } from '../lib/utils';

interface EmptyStateProps {
  icon: LucideIcon;
  title: string;
  description?: string;
  /* A primary action, when the emptiness is something the user can resolve. */
  children?: React.ReactNode;
  className?: string;
}

/*
 * Shown where content would be when there is none. `whitespace-normal` is explicit because a table
 * cell inherits `whitespace-nowrap`, which would otherwise stop the description wrapping.
 */
function EmptyState({ icon: Icon, title, description, children, className }: EmptyStateProps) {
  return (
    <div
      data-slot="empty-state"
      className={cn(
        'flex flex-col items-center justify-center px-6 py-12 gap-y-3 whitespace-normal text-center',
        className,
      )}
    >
      <span className="grid size-12 shrink-0 place-items-center bg-muted rounded-full text-foreground-subtle">
        <Icon aria-hidden="true" className="size-6" />
      </span>
      <div className="flex flex-col items-center gap-y-1">
        <p className="text-paragraph-sm-semibold text-foreground">{title}</p>
        {description ? (
          <p className="max-w-sm text-paragraph-xs text-foreground-muted">{description}</p>
        ) : null}
      </div>
      {children}
    </div>
  );
}

export { EmptyState };
