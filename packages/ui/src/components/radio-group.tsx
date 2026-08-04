'use client';

import * as React from 'react';
import * as RadioGroupPrimitive from '@radix-ui/react-radio-group';

import { cn } from '../lib/utils';

function RadioGroup({
  className,
  ...props
}: React.ComponentProps<typeof RadioGroupPrimitive.Root>) {
  return (
    <RadioGroupPrimitive.Root
      data-slot="radio-group"
      className={cn('grid gap-y-2', className)}
      {...props}
    />
  );
}

/* Force-mounted indicator, for the same reason as Checkbox: deselection animates out too. */
function RadioGroupItem({
  className,
  ...props
}: React.ComponentProps<typeof RadioGroupPrimitive.Item>) {
  return (
    <RadioGroupPrimitive.Item
      data-slot="radio-group-item"
      className={cn(
        'grid shrink-0 size-4 place-items-center bg-input border border-border-strong rounded-full shadow-e1 outline-none',
        'transition-[background-color,border-color,box-shadow] duration-200 ease-out-soft',
        'data-[state=checked]:border-primary',
        'hover:border-ring',
        'focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/45',
        'aria-invalid:border-danger aria-invalid:focus-visible:ring-danger/30',
        'disabled:cursor-not-allowed disabled:opacity-50',
        className,
      )}
      {...props}
    >
      <RadioGroupPrimitive.Indicator
        forceMount
        data-slot="radio-group-indicator"
        className={cn(
          'grid place-items-center size-2 bg-primary rounded-full',
          'transition-[opacity,scale] duration-200 ease-out-soft',
          'data-[state=unchecked]:scale-0 data-[state=unchecked]:opacity-0',
          'data-[state=checked]:scale-100 data-[state=checked]:opacity-100',
        )}
      />
    </RadioGroupPrimitive.Item>
  );
}

export { RadioGroup, RadioGroupItem };
