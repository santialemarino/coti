import * as React from 'react';

import { cn } from '../lib/utils';

/*
 * A scrolling list of cards or rows. The 4px inset keeps focus rings unclipped and the 16px end lane
 * holds an overlay scrollbar's thumb; negative margins cancel both, so the list aligns with its
 * siblings and the lane sits in the parent's padding, which needs 16px on the end side.
 */
function ScrollArea({ className, ...props }: React.ComponentProps<'div'>) {
  return (
    <div
      data-slot="scroll-area"
      className={cn('-ms-1 -me-4 p-1 pe-4 scroll-area', className)}
      {...props}
    />
  );
}

export { ScrollArea };
