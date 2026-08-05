import * as React from 'react';
import { cva, type VariantProps } from 'class-variance-authority';

import { cn } from '../lib/utils';

/*
 * An SVG arc rather than a bordered box: a border's arc meets its track at a hard corner, while a
 * round-capped stroke tapers into it. Both circles use `currentColor`, so a spinner dropped inside a
 * button or a tinted row takes that surface's colour with no prop, and the stroke is declared in
 * viewBox units so every size keeps the same ring-to-diameter ratio.
 */
const spinnerVariants = cva('shrink-0 animate-spin', {
  variants: {
    size: {
      xs: 'size-3',
      sm: 'size-4',
      default: 'size-5',
      lg: 'size-8',
      xl: 'size-10',
    },
  },
  defaultVariants: { size: 'default' },
});

/* A quarter of the r=10 circumference (2π·10 ≈ 62.8), so the arc reads as a sweep, not a gap. */
const ARC = '15.7 47.1';

function Spinner({
  className,
  size = 'default',
  label,
  ...props
}: React.ComponentProps<'svg'> & VariantProps<typeof spinnerVariants> & { label?: string }) {
  return (
    <svg
      data-slot="spinner"
      role="status"
      aria-label={label}
      aria-live="polite"
      viewBox="0 0 24 24"
      fill="none"
      strokeWidth={2.75}
      className={cn(spinnerVariants({ size }), className)}
      {...props}
    >
      <circle cx="12" cy="12" r="10" stroke="currentColor" strokeOpacity={0.2} />
      <circle
        cx="12"
        cy="12"
        r="10"
        stroke="currentColor"
        strokeLinecap="round"
        strokeDasharray={ARC}
      />
    </svg>
  );
}

export { Spinner, spinnerVariants };
