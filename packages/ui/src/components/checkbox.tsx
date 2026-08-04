'use client';

import * as React from 'react';
import * as CheckboxPrimitive from '@radix-ui/react-checkbox';
import { CheckIcon, MinusIcon } from 'lucide-react';

import { cn } from '../lib/utils';

/*
 * The indicator is force-mounted and animated with opacity and scale in both directions. Radix
 * unmounts it by default, which means unchecking has no exit animation and the tick just vanishes —
 * the one direction users see most. Both glyphs share a grid cell so switching to indeterminate
 * never reflows.
 */
function Checkbox({ className, ...props }: React.ComponentProps<typeof CheckboxPrimitive.Root>) {
  return (
    <CheckboxPrimitive.Root
      data-slot="checkbox"
      className={cn(
        'group/checkbox peer grid shrink-0 size-4 place-items-center bg-input border border-border-strong rounded-sm shadow-e1 outline-none',
        'transition-[background-color,border-color,box-shadow] duration-200 ease-out-soft',
        'data-[state=checked]:bg-primary data-[state=checked]:border-primary data-[state=checked]:text-primary-foreground',
        'data-[state=indeterminate]:bg-primary data-[state=indeterminate]:border-primary data-[state=indeterminate]:text-primary-foreground',
        'hover:border-ring',
        'focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/45',
        'aria-invalid:border-danger aria-invalid:focus-visible:ring-danger/30',
        'disabled:cursor-not-allowed disabled:opacity-50',
        className,
      )}
      {...props}
    >
      <CheckboxPrimitive.Indicator
        forceMount
        data-slot="checkbox-indicator"
        className={cn(
          'grid place-items-center text-current',
          'transition-[opacity,transform] duration-200 ease-out-soft',
          'data-[state=unchecked]:scale-50 data-[state=unchecked]:opacity-0',
          'data-[state=checked]:scale-100 data-[state=checked]:opacity-100',
          'data-[state=indeterminate]:scale-100 data-[state=indeterminate]:opacity-100',
        )}
      >
        <CheckIcon
          aria-hidden="true"
          className="col-start-1 row-start-1 size-3.5 group-data-[state=indeterminate]/checkbox:opacity-0"
        />
        <MinusIcon
          aria-hidden="true"
          className="col-start-1 row-start-1 size-3.5 opacity-0 group-data-[state=indeterminate]/checkbox:opacity-100"
        />
      </CheckboxPrimitive.Indicator>
    </CheckboxPrimitive.Root>
  );
}

export { Checkbox };
