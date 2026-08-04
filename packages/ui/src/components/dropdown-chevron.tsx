import * as React from 'react';
import { ChevronDownIcon } from 'lucide-react';

import { cn } from '../lib/utils';

/*
 * One chevron for every dropdown affordance, so a combobox, a date picker and a custom trigger all
 * announce themselves identically. `rotate` is named in the transition list because Tailwind v4's
 * `rotate-180` sets the `rotate` property rather than `transform` — a transform-only transition
 * leaves the chevron snapping.
 */
function DropdownChevron({
  open = false,
  className,
  ...props
}: React.ComponentProps<'svg'> & { open?: boolean }) {
  return (
    <ChevronDownIcon
      aria-hidden="true"
      data-slot="dropdown-chevron"
      className={cn(
        'size-4 shrink-0 opacity-60 transition-[color,rotate] duration-200 ease-out-soft',
        open && 'rotate-180',
        className,
      )}
      {...props}
    />
  );
}

export { DropdownChevron };
