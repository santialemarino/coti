import * as React from 'react';
import { Slot } from '@radix-ui/react-slot';
import { cva, type VariantProps } from 'class-variance-authority';

import { cn } from '../lib/utils';

/*
 * Tones, not meanings. A badge knows nothing about quote states or match confidence — the surface
 * that owns the enum picks the tone, so the same vocabulary covers a status pill, a confidence
 * score and a count. `dot` prepends a filled marker for the cases where colour alone would be the
 * only signal.
 */
const badgeVariants = cva(
  cn(
    'inline-flex w-fit shrink-0 items-center justify-center gap-x-1.5 border whitespace-nowrap rounded-full',
    'transition-[color,background-color,border-color] duration-200 ease-out-soft',
    "[&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-3",
  ),
  {
    variants: {
      tone: {
        neutral: 'border-border bg-muted text-foreground-muted',
        brand: 'border-brand-200 bg-accent text-accent-foreground',
        success: 'border-success-border bg-success-subtle text-success-foreground',
        warning: 'border-warning-border bg-warning-subtle text-warning-foreground',
        danger: 'border-danger-border bg-danger-subtle text-danger-foreground',
        solid: 'border-transparent bg-primary text-primary-foreground',
        outline: 'border-border-strong bg-transparent text-foreground-muted',
      },
      size: {
        sm: 'h-5 px-2 text-paragraph-mini-medium',
        default: 'h-6 px-2.5 text-paragraph-xs-medium',
      },
    },
    defaultVariants: {
      tone: 'neutral',
      size: 'default',
    },
  },
);

const DOT_TONES: Record<string, string> = {
  neutral: 'bg-foreground-subtle',
  brand: 'bg-brand-500',
  success: 'bg-success',
  warning: 'bg-warning',
  danger: 'bg-danger',
  solid: 'bg-primary-foreground',
  outline: 'bg-foreground-subtle',
};

function Badge({
  className,
  tone = 'neutral',
  size = 'default',
  dot = false,
  asChild = false,
  children,
  ...props
}: React.ComponentProps<'span'> &
  VariantProps<typeof badgeVariants> & { asChild?: boolean; dot?: boolean }) {
  const Comp = asChild ? Slot : 'span';

  return (
    <Comp
      data-slot="badge"
      data-tone={tone}
      className={cn(badgeVariants({ tone, size }), className)}
      {...props}
    >
      {dot ? (
        <span
          aria-hidden="true"
          className={cn('size-1.5 shrink-0 rounded-full', DOT_TONES[tone ?? 'neutral'])}
        />
      ) : null}
      {children}
    </Comp>
  );
}

export { Badge, badgeVariants };
