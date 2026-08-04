import * as React from 'react';
import * as LabelPrimitive from '@radix-ui/react-label';

import { cn } from '../lib/utils';

/*
 * The asterisk is a prop rather than markup at the call site, so every required field is marked
 * identically and the marker's colour and spacing live in one place. It is brand-tinted, not red:
 * red is reserved for something having gone wrong, and a form of red asterisks reads as a form full
 * of errors before the user has typed anything.
 */
function Label({
  className,
  required = false,
  children,
  ...props
}: React.ComponentProps<typeof LabelPrimitive.Root> & { required?: boolean }) {
  return (
    <LabelPrimitive.Root
      data-slot="label"
      className={cn(
        'flex items-center gap-x-1 select-none text-paragraph-sm-medium text-foreground',
        'group-data-[disabled=true]:pointer-events-none group-data-[disabled=true]:opacity-50',
        'peer-disabled:cursor-not-allowed peer-disabled:opacity-50',
        className,
      )}
      {...props}
    >
      {children}
      {required ? (
        /* -ml-0.5 pulls the marker back against the label text: the row's gap-x-1 is sized for an
           icon, and left alone it reads as a floating asterisk rather than part of the label. */
        <span aria-hidden="true" className="-ml-0.5 text-primary">
          *
        </span>
      ) : null}
    </LabelPrimitive.Root>
  );
}

export { Label };
