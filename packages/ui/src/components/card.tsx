import * as React from 'react';

import { cn } from '../lib/utils';

/*
 * `interactive` marks a card the user can click (a summary tile, a list entry). It deepens the
 * elevation and the border on hover and carries a focus ring, but deliberately does not translate:
 * when the element that owns the `:hover` moves, the pointer falls outside it at the boundary and
 * the state oscillates. A lift has to be driven from a stationary wrapper — see the ux-motion skill.
 */
function Card({
  className,
  interactive = false,
  ...props
}: React.ComponentProps<'div'> & { interactive?: boolean }) {
  return (
    <div
      data-slot="card"
      className={cn(
        'flex flex-col w-full min-w-0 gap-y-5 py-6 bg-card border border-border rounded-1.5xl shadow-e2 text-card-foreground',
        interactive &&
          cn(
            'transition-[border-color,box-shadow] duration-200 ease-out-soft',
            'hover:border-border-strong hover:shadow-e3',
            'outline-none focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/45',
          ),
        className,
      )}
      {...props}
    />
  );
}

/* Sections supply their own horizontal padding, so a full-bleed child (a table, a divider) can opt out. */
function CardHeader({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="card-header"
      className={cn('flex flex-col px-6 gap-y-1.5', className)}
      {...props}
    />
  );
}

function CardTitle({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="card-title"
      className={cn('text-heading-5 text-foreground', className)}
      {...props}
    />
  );
}

function CardDescription({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="card-description"
      className={cn('text-paragraph-sm text-foreground-muted', className)}
      {...props}
    />
  );
}

function CardContent({ className, ...props }: React.ComponentProps<'div'>) {
  return <div data-slot="card-content" className={cn('px-6', className)} {...props} />;
}

function CardFooter({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div data-slot="card-footer" className={cn('flex items-center px-6', className)} {...props} />
  );
}

export { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle };
