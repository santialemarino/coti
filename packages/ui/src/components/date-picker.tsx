'use client';

import * as React from 'react';
import { addYears, startOfYear } from 'date-fns';
import type { Matcher } from 'react-day-picker';

import { displayDay, parseDay, toDay } from '../lib/dates';
import { Calendar } from './calendar';
import { DateFieldTrigger } from './date-field-trigger';
import { Popover, PopoverContent, PopoverTrigger } from './popover';

// How far the year select reaches when the field sets no bound of its own.
const YEARS_BACK = 80;
const YEARS_AHEAD = 10;

interface DatePickerProps {
  // The chosen day as `YYYY-MM-DD`, or '' while none is.
  value: string;
  onValueChange: (value: string) => void;
  placeholder: string;
  // Earliest and latest selectable days, as `YYYY-MM-DD`.
  min?: string;
  max?: string;
  disabled?: boolean;
  id?: string;
  className?: string;
  onBlur?: () => void;
  'aria-label'?: string;
  'aria-invalid'?: boolean;
  'aria-describedby'?: string;
  ref?: React.Ref<HTMLButtonElement>;
}

/*
 * A single day, picked from the design system's calendar. Picking is the whole job, so the panel
 * closes on it and focus returns to the field; a click on the day already chosen keeps it and closes
 * too. The month and year selects reach decades back without paging month by month.
 */
function DatePicker({
  value,
  onValueChange,
  placeholder,
  min,
  max,
  disabled = false,
  id,
  className,
  onBlur,
  'aria-label': ariaLabel,
  'aria-invalid': ariaInvalid,
  'aria-describedby': ariaDescribedBy,
  ref,
}: DatePickerProps) {
  const [open, setOpen] = React.useState(false);
  const selected = parseDay(value);
  const minDate = parseDay(min);
  const maxDate = parseDay(max);
  const today = new Date();

  // A list, not one `{ before, after }`: that shape reads as "between", and a day here is out when it matches either.
  const bounds: Matcher[] = [];
  if (minDate) bounds.push({ before: minDate });
  if (maxDate) bounds.push({ after: maxDate });

  function handleSelect(day: Date | undefined) {
    if (day) onValueChange(toDay(day));
    setOpen(false);
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <DateFieldTrigger
          ref={ref}
          id={id}
          open={open}
          unset={!selected}
          invalid={ariaInvalid}
          disabled={disabled}
          label={selected ? displayDay(value) : placeholder}
          className={className}
          onBlur={onBlur}
          aria-label={ariaLabel}
          aria-describedby={ariaDescribedBy}
        />
      </PopoverTrigger>
      <PopoverContent
        align="start"
        className="w-auto"
        // The calendar moves focus onto the chosen day (or today) itself, so arrows work at once.
        onOpenAutoFocus={(event) => event.preventDefault()}
      >
        <Calendar
          mode="single"
          autoFocus
          captionLayout="dropdown"
          selected={selected}
          onSelect={handleSelect}
          defaultMonth={selected ?? maxDate ?? today}
          startMonth={minDate ?? startOfYear(addYears(today, -YEARS_BACK))}
          endMonth={maxDate ?? addYears(today, YEARS_AHEAD)}
          disabled={bounds}
        />
      </PopoverContent>
    </Popover>
  );
}

export { DatePicker };
export type { DatePickerProps };
