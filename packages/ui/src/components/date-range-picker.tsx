'use client';

import * as React from 'react';
import { addMonths, isSameMonth } from 'date-fns';
import { XIcon } from 'lucide-react';
import type { DateRange, Matcher } from 'react-day-picker';

import { displayDay, parseDay, toDay } from '../lib/dates';
import { cn } from '../lib/utils';
import { Button } from './button';
import { Calendar } from './calendar';
import { DateFieldTrigger } from './date-field-trigger';
import { Popover, PopoverContent, PopoverTrigger } from './popover';

// From this width the panel shows two months side by side.
const TWO_MONTHS_QUERY = '(min-width: 768px)';

type RangeEnd = 'from' | 'to';

// Each end is a `YYYY-MM-DD` day, or '' while it is open.
interface DateRangeValue {
  from: string;
  to: string;
}

interface DateRangePreset {
  label: string;
  value: DateRangeValue;
}

interface DateRangePickerLabels {
  // Name each end in the panel ("Desde", "Hasta").
  from: string;
  to: string;
  // What an end reads while nothing is picked for it and it cannot stay empty.
  pick: string;
  // Confirms a range edited without picking both ends again.
  done: string;
  // Names the clear button beside an end that may stay empty.
  clearFrom?: string;
  clearTo?: string;
  // Names the list of presets.
  presets?: string;
}

interface DateRangePickerProps {
  value: DateRangeValue;
  onValueChange: (value: DateRangeValue) => void;
  // Shown on the field while neither end is set.
  placeholder: string;
  labels: DateRangePickerLabels;
  // Ranges one press away, committed as they are.
  presets?: DateRangePreset[];
  // What an empty start or end means ("Desde ya", "Sin vencimiento"). Passing it lets that end stay empty.
  openStart?: string;
  openEnd?: string;
  // For a native form: each end is submitted under its own name.
  names?: { from: string; to: string };
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

function subscribeToQuery(onChange: () => void) {
  const query = window.matchMedia(TWO_MONTHS_QUERY);
  query.addEventListener('change', onChange);
  return () => query.removeEventListener('change', onChange);
}

function useTwoMonths() {
  return React.useSyncExternalStore(
    subscribeToQuery,
    () => window.matchMedia(TWO_MONTHS_QUERY).matches,
    () => false,
  );
}

function rangeLabel(
  value: DateRangeValue,
  placeholder: string,
  openStart?: string,
  openEnd?: string,
) {
  if (!value.from && !value.to) return placeholder;
  const from = value.from ? displayDay(value.from) : (openStart ?? '…');
  const to = value.to ? displayDay(value.to) : (openEnd ?? '…');
  return `${from} – ${to}`;
}

// Opening on the end being edited, with its partner in view beside it when the two fit.
function monthFor(value: DateRangeValue, end: RangeEnd, twoMonths: boolean) {
  const from = parseDay(value.from);
  const to = parseDay(value.to);
  if (end === 'to' && to) {
    return twoMonths && from && !isSameMonth(from, to) ? addMonths(to, -1) : to;
  }
  return from ?? to ?? new Date();
}

/*
 * One field for an interval. The panel names both ends and edits one at a time: a day sets the end
 * being edited and moves on to the other, and the second pick commits and closes. To change one end
 * alone, choose it and confirm with the done button. Escape or a click outside drops the edit. A preset
 * commits at once. Days are `YYYY-MM-DD` strings in and out, so a form keeps the shape it had.
 */
function DateRangePicker({
  value,
  onValueChange,
  placeholder,
  labels,
  presets = [],
  openStart,
  openEnd,
  names,
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
}: DateRangePickerProps) {
  const twoMonths = useTwoMonths();
  const [open, setOpen] = React.useState(false);
  const [draft, setDraft] = React.useState(value);
  const [active, setActive] = React.useState<RangeEnd>('from');
  const [month, setMonth] = React.useState(() => monthFor(value, 'from', false));
  const [hovered, setHovered] = React.useState<string>();
  // The end was picked before the start, so the start is now the second pick and commits.
  const [endPicked, setEndPicked] = React.useState(false);
  const unset = !value.from && !value.to;

  const from = parseDay(draft.from);
  const to = parseDay(draft.to);
  const minDate = parseDay(min);
  const maxDate = parseDay(max);
  const complete =
    (draft.from !== '' || openStart !== undefined) && (draft.to !== '' || openEnd !== undefined);
  // While the end is next, the days between the start and the pointer show where it would land.
  const previewTo =
    active === 'to' && from && hovered && hovered > draft.from ? parseDay(hovered) : undefined;

  const bounds: Matcher[] = [];
  if (minDate) bounds.push({ before: minDate });
  if (maxDate) bounds.push({ after: maxDate });

  function handleOpenChange(next: boolean) {
    if (next) {
      setDraft(value);
      setActive('from');
      setEndPicked(false);
      setMonth(monthFor(value, 'from', twoMonths));
    }
    setHovered(undefined);
    setOpen(next);
  }

  function commit(next: DateRangeValue) {
    onValueChange(next);
    setOpen(false);
  }

  function pickDay(date: Date) {
    const day = toDay(date);
    if (active === 'from') {
      const next = { from: day, to: draft.to && draft.to >= day ? draft.to : '' };
      if (endPicked && next.to) commit(next);
      else {
        setDraft(next);
        setActive('to');
      }
      return;
    }
    // An end before the start starts the range over from that day.
    if (draft.from && day < draft.from) {
      setDraft({ from: day, to: '' });
      return;
    }
    const next = { from: draft.from, to: day };
    if (next.from || openStart !== undefined) commit(next);
    else {
      setDraft(next);
      setActive('from');
      setEndPicked(true);
    }
  }

  function chooseEnd(end: RangeEnd) {
    setActive(end);
    setMonth(monthFor(draft, end, twoMonths));
  }

  function clearEnd(end: RangeEnd) {
    setDraft({ ...draft, [end]: '' });
    setActive(end);
  }

  const selected: DateRange = { from: from ?? to, to: from ? to : undefined };

  return (
    <>
      <Popover open={open} onOpenChange={handleOpenChange}>
        <PopoverTrigger asChild>
          <DateFieldTrigger
            ref={ref}
            id={id}
            open={open}
            unset={unset}
            invalid={ariaInvalid}
            disabled={disabled}
            label={rangeLabel(value, placeholder, openStart, openEnd)}
            className={className}
            onBlur={onBlur}
            aria-label={ariaLabel}
            aria-describedby={ariaDescribedBy}
          />
        </PopoverTrigger>
        <PopoverContent
          align="start"
          collisionPadding={8}
          className="flex w-auto max-w-[calc(100vw-1rem)] flex-col p-0 md:flex-row"
          // The calendar moves focus onto the start (or today) itself, so arrows work at once.
          onOpenAutoFocus={(event) => event.preventDefault()}
        >
          {presets.length > 0 ? (
            <div
              role="group"
              aria-label={labels.presets}
              className="flex shrink-0 overflow-x-auto p-2 gap-x-1 border-b border-border md:w-40 md:flex-col md:overflow-visible md:gap-y-0.5 md:border-r md:border-b-0"
            >
              {presets.map((preset) => {
                const pressed = preset.value.from === draft.from && preset.value.to === draft.to;
                return (
                  <Button
                    key={preset.label}
                    variant="ghost"
                    size="sm"
                    aria-pressed={pressed}
                    className="justify-start aria-pressed:bg-accent aria-pressed:text-accent-foreground text-paragraph-sm"
                    onClick={() => commit(preset.value)}
                  >
                    {preset.label}
                  </Button>
                );
              })}
            </div>
          ) : null}
          <div className="flex flex-col p-3 gap-y-3">
            <div className="grid grid-cols-2 gap-x-2">
              <RangeEndTab
                end="from"
                label={labels.from}
                value={draft.from}
                empty={openStart ?? labels.pick}
                active={active === 'from'}
                clearLabel={openStart !== undefined ? labels.clearFrom : undefined}
                onChoose={chooseEnd}
                onClear={clearEnd}
              />
              <RangeEndTab
                end="to"
                label={labels.to}
                value={draft.to}
                empty={openEnd ?? labels.pick}
                active={active === 'to'}
                clearLabel={openEnd !== undefined ? labels.clearTo : undefined}
                onChoose={chooseEnd}
                onClear={clearEnd}
              />
            </div>
            <Calendar
              mode="range"
              autoFocus
              numberOfMonths={twoMonths ? 2 : 1}
              month={month}
              onMonthChange={setMonth}
              selected={selected}
              onDayClick={pickDay}
              onDayMouseEnter={(date, modifiers) => {
                if (!modifiers.disabled) setHovered(toDay(date));
              }}
              onDayMouseLeave={() => setHovered(undefined)}
              onDayFocus={(date, modifiers) => {
                if (!modifiers.disabled) setHovered(toDay(date));
              }}
              disabled={bounds}
              startMonth={minDate}
              endMonth={maxDate}
              modifiers={{
                range_from: from ?? false,
                range_to: to ?? false,
                in_range: from && to ? { after: from, before: to } : false,
                preview: previewTo ? { after: from!, before: previewTo } : false,
              }}
              modifiersClassNames={{
                // Half a band behind each end, so the range reads as one stretch between them.
                range_from:
                  from && to ? 'bg-linear-to-r from-transparent from-50% to-accent to-50%' : '',
                range_to:
                  from && to ? 'bg-linear-to-l from-transparent from-50% to-accent to-50%' : '',
              }}
            />
            <div className="flex justify-end">
              <Button size="sm" disabled={!complete} onClick={() => commit(draft)}>
                {labels.done}
              </Button>
            </div>
          </div>
        </PopoverContent>
      </Popover>
      {names ? (
        <>
          <input type="hidden" name={names.from} value={value.from} />
          <input type="hidden" name={names.to} value={value.to} />
        </>
      ) : null}
    </>
  );
}

interface RangeEndTabProps {
  end: RangeEnd;
  label: string;
  value: string;
  empty: string;
  active: boolean;
  clearLabel?: string;
  onChoose: (end: RangeEnd) => void;
  onClear: (end: RangeEnd) => void;
}

// One end of the range: which end the next day sets, and what it holds now.
function RangeEndTab({
  end,
  label,
  value,
  empty,
  active,
  clearLabel,
  onChoose,
  onClear,
}: RangeEndTabProps) {
  const clearable = clearLabel !== undefined && value !== '';
  return (
    <div className="relative">
      <button
        type="button"
        aria-pressed={active}
        className={cn(
          'flex w-full min-w-32 flex-col items-start px-3 py-1.5 border border-border rounded-lg',
          'outline-none transition-[color,background-color,border-color,box-shadow] duration-200 ease-out-soft',
          'hover:bg-surface-hover active:bg-surface-active focus-visible:border-ring focus-visible:ring-3 focus-visible:ring-ring/45',
          'aria-pressed:border-primary aria-pressed:bg-accent aria-pressed:hover:bg-accent',
          clearable && 'pr-8',
        )}
        onClick={() => onChoose(end)}
      >
        <span className="text-paragraph-xs text-foreground-muted">{label}</span>
        <span
          className={cn(
            'text-paragraph-sm-medium tabular-nums',
            value ? 'text-foreground' : 'text-foreground-subtle',
          )}
        >
          {value ? displayDay(value) : empty}
        </span>
      </button>
      {clearable ? (
        <button
          type="button"
          aria-label={clearLabel}
          className="group/clear absolute top-1/2 right-1.5 inline-flex size-6 -translate-y-1/2 items-center justify-center rounded-md text-foreground-subtle outline-none transition-colors duration-200 ease-out-soft hover:text-foreground focus-visible:text-foreground"
          onClick={() => onClear(end)}
        >
          <XIcon
            aria-hidden="true"
            className="size-3.5 group-focus-visible/clear:animate-focus-bump"
          />
        </button>
      ) : null}
    </div>
  );
}

export { DateRangePicker };
export type { DateRangePickerLabels, DateRangePickerProps, DateRangePreset, DateRangeValue };
