'use client';

import * as React from 'react';
import { CalendarDaysIcon } from 'lucide-react';

import { cn } from '../lib/utils';
import { Button } from './button';
import { DropdownChevron } from './dropdown-chevron';

interface DateFieldTriggerProps extends React.ComponentProps<typeof Button> {
  open: boolean;
  // True while nothing is chosen: the trigger shows the placeholder, muted.
  unset: boolean;
  invalid?: boolean;
  label: React.ReactNode;
}

/*
 * The field a date picker opens from. It is a combobox, as `Combobox`'s trigger is, so a screen reader
 * reads the chosen date as its value; a button would be named by its label alone and never say it.
 */
const DateFieldTrigger = React.forwardRef<HTMLButtonElement, DateFieldTriggerProps>(
  ({ open, unset, invalid, label, className, ...props }, ref) => (
    <Button
      ref={ref}
      type="button"
      variant="outline"
      role="combobox"
      aria-haspopup="dialog"
      aria-expanded={open}
      aria-invalid={invalid || undefined}
      className={cn(
        'w-full justify-between bg-input hover:bg-input active:bg-input aria-expanded:bg-input text-paragraph-sm',
        unset && 'text-foreground-subtle',
        invalid && 'border-danger focus-visible:border-danger focus-visible:ring-danger/30',
        className,
      )}
      {...props}
    >
      <span className="flex min-w-0 flex-1 items-center gap-x-2">
        <CalendarDaysIcon aria-hidden="true" className="size-4 text-foreground-muted" />
        <span className="flex-1 truncate text-left tabular-nums">{label}</span>
      </span>
      <DropdownChevron open={open} />
    </Button>
  ),
);

DateFieldTrigger.displayName = 'DateFieldTrigger';

export { DateFieldTrigger };
