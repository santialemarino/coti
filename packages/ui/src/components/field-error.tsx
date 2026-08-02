import * as React from 'react';

import { cn } from '../lib/utils';

// Renders nothing when there is no message, so a field reserves no space for an
// error it does not have.
function FieldError({
  className,
  children,
  ...props
}: React.ComponentProps<'p'>): React.ReactElement | null {
  if (!children) return null;
  return (
    <p
      data-slot="field-error"
      role="alert"
      className={cn('text-sm text-destructive', className)}
      {...props}
    >
      {children}
    </p>
  );
}

export { FieldError };
