import * as React from 'react';

import { cn } from '../lib/utils';

/* Helper text under a field: what to enter, what the default is, what the format looks like. */
function Hint({ className, ...props }: React.ComponentProps<'p'>) {
  return (
    <p
      data-slot="hint"
      className={cn('text-paragraph-xs text-foreground-muted', className)}
      {...props}
    />
  );
}

export { Hint };
