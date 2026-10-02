'use client';

import * as React from 'react';
import * as PopoverPrimitive from '@radix-ui/react-popover';

import { cn } from '../lib/utils';

function Popover({ ...props }: React.ComponentProps<typeof PopoverPrimitive.Root>) {
  return <PopoverPrimitive.Root data-slot="popover" {...props} />;
}

function PopoverTrigger({ ...props }: React.ComponentProps<typeof PopoverPrimitive.Trigger>) {
  return <PopoverPrimitive.Trigger data-slot="popover-trigger" {...props} />;
}

function PopoverAnchor({ ...props }: React.ComponentProps<typeof PopoverPrimitive.Anchor>) {
  return <PopoverPrimitive.Anchor data-slot="popover-anchor" {...props} />;
}

/*
 * The slide is declared on both sides — enter from the trigger, exit back toward it — so closing
 * mirrors opening instead of just fading out from wherever it happened to be.
 *
 * There is deliberately no zoom. A zoom scales from the panel's transform origin, so on a surface
 * anchored by one corner the opposite edge travels — on a dropdown as wide as its trigger that reads
 * as the list being measured and resized rather than as it arriving. A dialog keeps its zoom:
 * centred, the same motion reads as depth.
 */
function PopoverContent({
  className,
  align = 'center',
  sideOffset = 6,
  ...props
}: React.ComponentProps<typeof PopoverPrimitive.Content>) {
  return (
    <PopoverPrimitive.Portal>
      <PopoverPrimitive.Content
        data-slot="popover-content"
        align={align}
        sideOffset={sideOffset}
        className={cn(
          'z-50 w-72 origin-(--radix-popover-content-transform-origin) p-3 bg-popover border border-border rounded-xl shadow-e3 text-popover-foreground outline-hidden',
          'data-[state=open]:animate-in data-[state=open]:fade-in-0',
          'data-[state=closed]:animate-out data-[state=closed]:fade-out-0',
          'data-[side=bottom]:slide-in-from-top-2 data-[side=top]:slide-in-from-bottom-2',
          'data-[side=left]:slide-in-from-right-2 data-[side=right]:slide-in-from-left-2',
          'data-[side=bottom]:slide-out-to-top-2 data-[side=top]:slide-out-to-bottom-2',
          'data-[side=left]:slide-out-to-right-2 data-[side=right]:slide-out-to-left-2',
          'duration-200 ease-out-soft',
          className,
        )}
        {...props}
      />
    </PopoverPrimitive.Portal>
  );
}

export { Popover, PopoverAnchor, PopoverContent, PopoverTrigger };
