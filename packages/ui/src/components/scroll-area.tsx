import * as React from 'react';

import { cn } from '../lib/utils';

/*
 * A vertical scroll region for a list of cards or rows. The inset keeps a focus ring or a border from
 * being clipped at the edge, and the extra end padding gives an overlay scrollbar a lane of its own,
 * so its thumb never sits on top of the content.
 */
function ScrollArea({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div data-slot="scroll-area" className={cn('p-1 pe-2 scroll-area', className)} {...props} />
  );
}

export { ScrollArea };
