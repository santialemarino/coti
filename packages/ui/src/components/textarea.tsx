import * as React from 'react';

import { cn } from '../lib/utils';

function Textarea({ className, ...props }: React.ComponentProps<'textarea'>) {
  return (
    <textarea
      data-slot="textarea"
      className={cn(
        'flex w-full min-h-20 px-3 py-2 field-sizing-content resize-y bg-input border border-border rounded-lg shadow-e1 outline-none',
        'transition-[border-color,box-shadow] duration-200 ease-out-soft',
        'focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/45',
        'aria-invalid:border-danger aria-invalid:focus-visible:border-danger aria-invalid:focus-visible:ring-danger/30',
        'read-only:bg-input-readonly',
        'disabled:pointer-events-none disabled:cursor-not-allowed disabled:opacity-50',
        'text-paragraph-sm text-foreground placeholder:text-foreground-subtle',
        className,
      )}
      {...props}
    />
  );
}

export { Textarea };
