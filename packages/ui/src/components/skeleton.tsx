import * as React from 'react';

import { cn } from '../lib/utils';

/*
 * Pass the text the skeleton stands in for as children: it renders invisibly and gives the
 * placeholder the exact size of the real content, so a loading row lines up with the loaded one and
 * no call site has to hand-tune a width.
 *
 * The fill is `border`, not `muted`: muted sits within a point of lightness of the page wash, so a
 * skeleton on anything other than a white card would be invisible.
 */
function Skeleton({ className, children, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="skeleton"
      aria-hidden="true"
      className={cn('relative bg-border animate-pulse rounded-md', className)}
      {...props}
    >
      <span className="invisible whitespace-pre-wrap">{children}</span>
    </div>
  );
}

export { Skeleton };
