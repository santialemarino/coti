import * as React from 'react';
import { cva, type VariantProps } from 'class-variance-authority';

import { cn } from '../lib/utils';

const progressVariants = cva('w-full overflow-hidden bg-border rounded-full', {
  variants: {
    size: {
      sm: 'h-1',
      default: 'h-1.5',
      lg: 'h-2.5',
    },
  },
  defaultVariants: { size: 'default' },
});

const TONE_FILLS = {
  brand: 'bg-primary',
  success: 'bg-success',
  warning: 'bg-warning',
  danger: 'bg-danger',
  neutral: 'bg-foreground-subtle',
} as const;

interface ProgressProps
  extends Omit<React.ComponentProps<'div'>, 'children'>, VariantProps<typeof progressVariants> {
  /* 0–100. Clamped, so a caller computing a ratio can't overflow the track. */
  value: number;
  tone?: keyof typeof TONE_FILLS;
  /* Omit for a purely decorative bar that a label already describes. */
  label?: string;
}

/*
 * Width is an inline style because the value is a runtime number — Tailwind can only generate
 * classes it can see. The fill transitions, so a value stepping up reads as filling rather than
 * jumping.
 */
function Progress({
  className,
  value,
  size = 'default',
  tone = 'brand',
  label,
  ...props
}: ProgressProps) {
  const clamped = Math.max(0, Math.min(100, value));

  return (
    <div
      data-slot="progress"
      role="progressbar"
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={Math.round(clamped)}
      aria-label={label}
      className={cn(progressVariants({ size }), className)}
      {...props}
    >
      <div
        data-slot="progress-fill"
        className={cn(
          'h-full rounded-full transition-[width,background-color] duration-300 ease-out-soft',
          TONE_FILLS[tone],
        )}
        style={{ width: `${clamped}%` }}
      />
    </div>
  );
}

export { Progress, progressVariants };
