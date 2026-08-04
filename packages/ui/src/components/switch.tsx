'use client';

import * as React from 'react';
import * as SwitchPrimitive from '@radix-ui/react-switch';

import { cn } from '../lib/utils';

function Switch({ className, ...props }: React.ComponentProps<typeof SwitchPrimitive.Root>) {
  return (
    <SwitchPrimitive.Root
      data-slot="switch"
      className={cn(
        'peer inline-flex shrink-0 h-5 w-9 items-center bg-border-strong border border-transparent rounded-full shadow-e1 outline-none',
        'transition-[background-color,box-shadow] duration-200 ease-out-soft',
        'data-[state=checked]:bg-primary',
        /* The same hover cue Checkbox and RadioGroupItem carry — a switch is the most obviously
           clickable of the three and was the only one with no hover state. */
        'hover:border-ring',
        'focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/45',
        'disabled:cursor-not-allowed disabled:opacity-50',
        className,
      )}
      {...props}
    >
      <SwitchPrimitive.Thumb
        data-slot="switch-thumb"
        className={cn(
          'pointer-events-none block size-4 bg-background rounded-full shadow-e1 ring-0',
          'transition-transform duration-200 ease-out-soft',
          'translate-x-0.5 data-[state=checked]:translate-x-[calc(100%+0.125rem)]',
        )}
      />
    </SwitchPrimitive.Root>
  );
}

export { Switch };
