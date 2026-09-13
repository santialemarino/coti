import * as React from 'react';
import type { LucideIcon } from 'lucide-react';

import { cn } from '../lib/utils';

interface EmptyStateProps {
  icon: LucideIcon;
  title: string;
  description?: string;
  /*
   * `inline` is a note where a short list would be — a folded panel, a sidebar section — and takes
   * one line. `sm` is the in-place emptiness of a table body or a panel that owns its own block of
   * the page. `lg` is a whole pane with nothing in it, where the same treatment at the same size
   * reads as a loading glitch.
   *
   * The size follows the space the missing content would have taken, not the importance of the
   * message: a centred block with a haloed icon inside a two-row panel reserves more room for the
   * absence than the presence ever needed, and pushes everything under it away.
   */
  size?: 'inline' | 'sm' | 'lg';
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
  const inline = size === 'inline';

  return (
    <div
      data-slot="empty-state"
      data-size={size}
      className={cn(
        'flex whitespace-normal',
        inline
          ? 'items-center py-2 gap-x-2 text-left'
          : 'flex-col items-center justify-center px-6 gap-y-3 text-center',
        large && 'py-20 gap-y-4',
        size === 'sm' && 'py-12',
        className,
      )}
    >
      {/* Inline drops the halo: at one line of copy the disc is bigger than the message. */}
      {inline ? (
        <Icon aria-hidden="true" className="size-4 shrink-0 text-foreground-subtle" />
      ) : (
        <span
          className={cn(
            'grid shrink-0 place-items-center bg-muted rounded-full text-foreground-subtle',
            large ? 'size-16' : 'size-12',
          )}
        >
          <Icon aria-hidden="true" className={large ? 'size-8' : 'size-6'} />
        </span>
      )}

      <div className={cn('flex flex-col gap-y-1', !inline && 'items-center')}>
        <p
          className={cn(
            large
              ? 'text-heading-6 text-foreground'
              : inline
                ? 'text-paragraph-sm text-foreground-muted'
                : 'text-paragraph-sm-semibold text-foreground',
          )}
        >
          {title}
        </p>
        {description ? (
          <p
            className={cn(
              'max-w-sm',
              large
                ? 'text-paragraph-sm text-foreground-muted'
                : inline
                  ? 'text-paragraph-xs text-foreground-subtle'
                  : 'text-paragraph-xs text-foreground-muted',
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
