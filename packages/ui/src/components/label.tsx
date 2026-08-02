import * as React from 'react';

import { cn } from '../lib/utils';

// The asterisk is a prop rather than markup at the call site, so every required
// field is marked the same way.
function Label({
  className,
  required = false,
  children,
  ...props
}: React.ComponentProps<'label'> & { required?: boolean }) {
  return (
    <label
      data-slot="label"
      className={cn(
        'flex items-center gap-x-1 text-sm font-medium leading-none select-none',
        'peer-disabled:cursor-not-allowed peer-disabled:opacity-70',
        className,
      )}
      {...props}
    >
      {children}
      {required ? (
        <span aria-hidden="true" className="text-destructive">
          *
        </span>
      ) : null}
    </label>
  );
}

export { Label };
