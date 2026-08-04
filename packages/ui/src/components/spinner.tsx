import * as React from 'react';
import { cva, type VariantProps } from 'class-variance-authority';

import { cn } from '../lib/utils';

/*
 * Borders rather than an SVG, so the ring inherits `currentColor` and a spinner dropped inside a
 * button or a tinted row takes that surface's colour with no prop. The track is the same colour at
 * low alpha, which keeps it legible on both light and tinted backgrounds.
 */
const spinnerVariants = cva(
  'shrink-0 rounded-full border-current/20 border-t-current animate-spin',
  {
    variants: {
      size: {
        xs: 'size-3 border-[1.5px]',
        sm: 'size-4 border-2',
        default: 'size-5 border-2',
        lg: 'size-8 border-[3px]',
        xl: 'size-10 border-4',
      },
    },
    defaultVariants: { size: 'default' },
  },
);

function Spinner({
  className,
  size = 'default',
  label,
  ...props
}: React.ComponentProps<'div'> & VariantProps<typeof spinnerVariants> & { label?: string }) {
  return (
    <div
      data-slot="spinner"
      role="status"
      aria-label={label}
      aria-live="polite"
      className={cn(spinnerVariants({ size }), className)}
      {...props}
    />
  );
}

export { Spinner, spinnerVariants };
