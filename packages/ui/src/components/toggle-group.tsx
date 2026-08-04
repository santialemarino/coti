'use client';

import * as React from 'react';
import * as ToggleGroupPrimitive from '@radix-ui/react-toggle-group';
import { cva, type VariantProps } from 'class-variance-authority';

import { cn } from '../lib/utils';

const toggleGroupVariants = cva('flex w-fit items-center', {
  variants: {
    variant: {
      /* Joined segments in a shared bordered track — a mode switch. */
      segmented: 'p-1 bg-sunken border border-border rounded-xl gap-x-1',
      /* Free-standing pills — filters, where "none selected" is a valid state. */
      pills: 'gap-x-2',
    },
  },
  defaultVariants: { variant: 'segmented' },
});

const ToggleGroupContext = React.createContext<{
  variant: 'segmented' | 'pills';
  size: 'sm' | 'default';
}>({ variant: 'segmented', size: 'default' });

function ToggleGroup({
  className,
  variant = 'segmented',
  size = 'default',
  children,
  ...props
}: React.ComponentProps<typeof ToggleGroupPrimitive.Root> &
  VariantProps<typeof toggleGroupVariants> & { size?: 'sm' | 'default' }) {
  return (
    <ToggleGroupPrimitive.Root
      data-slot="toggle-group"
      data-variant={variant}
      className={cn(toggleGroupVariants({ variant }), className)}
      {...props}
    >
      <ToggleGroupContext.Provider value={{ variant: variant ?? 'segmented', size }}>
        {children}
      </ToggleGroupContext.Provider>
    </ToggleGroupPrimitive.Root>
  );
}

/*
 * A segmented item is a surface, so its focus indicator is the ring — not the icon bump, which is
 * for icon-only triggers. `z-10` on focus keeps the ring from being clipped by the next segment.
 */
function ToggleGroupItem({
  className,
  children,
  ...props
}: React.ComponentProps<typeof ToggleGroupPrimitive.Item>) {
  const { variant, size } = React.useContext(ToggleGroupContext);

  return (
    <ToggleGroupPrimitive.Item
      data-slot="toggle-group-item"
      className={cn(
        'relative inline-flex shrink-0 items-center justify-center gap-x-1.5 whitespace-nowrap border border-transparent outline-none',
        'transition-[color,background-color,border-color,box-shadow,transform] duration-200 ease-out-soft',
        'focus-visible:z-10 focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/45',
        'active:scale-[0.97]',
        'disabled:pointer-events-none disabled:opacity-50',
        "[&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
        size === 'sm' ? 'h-7 px-2.5 text-paragraph-xs-medium' : 'h-8 px-3 text-paragraph-sm-medium',
        variant === 'segmented'
          ? cn(
              'flex-1 rounded-lg text-foreground-muted hover:text-foreground',
              'data-[state=on]:bg-background data-[state=on]:text-foreground data-[state=on]:shadow-e1',
            )
          : cn(
              'rounded-full border-border bg-background text-foreground-muted shadow-e1 hover:border-border-strong hover:text-foreground',
              'data-[state=on]:border-primary data-[state=on]:bg-primary data-[state=on]:text-primary-foreground',
            ),
        className,
      )}
      {...props}
    >
      {children}
    </ToggleGroupPrimitive.Item>
  );
}

export { ToggleGroup, ToggleGroupItem, toggleGroupVariants };
