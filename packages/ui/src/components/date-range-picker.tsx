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
  // Commits the range being edited and closes the panel.
  apply: string;
  // Drops the edit and closes the panel.
  cancel: string;
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
  // Ranges one press away; a preset fills the range being edited.
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

// Two days, in whichever order they came, as a range.
function ordered(a: string, b: string): DateRangeValue {
  return a <= b ? { from: a, to: b } : { from: b, to: a };
}

/*
 * One field for an interval. The panel edits a draft and commits only on the apply button. Two picks
 * make the range in either order, the earlier day being the start; choosing an end first replaces only
 * that end, swapping the two when the new day crosses the other; a preset fills the draft; an end that
 * may stay open is cleared beside it. Cancel, Escape or a click outside drop the draft. Days are
 * `YYYY-MM-DD` strings in and out, so a form keeps the shape it had.
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
  // The first of two picks, waiting for the second.
  const [anchor, setAnchor] = React.useState<string>();
  // The end chosen explicitly, which the next pick replaces alone.
  const [editing, setEditing] = React.useState<RangeEnd>();
  const [month, setMonth] = React.useState(() => monthFor(value, 'from', false));
  const [hovered, setHovered] = React.useState<string>();
  const unset = !value.from && !value.to;

  const minDate = parseDay(min);
  const maxDate = parseDay(max);
  const complete =
    (draft.from !== '' || openStart !== undefined) && (draft.to !== '' || openEnd !== undefined);
  // Applying the range already committed would change nothing, so there is nothing to apply.
  const changed = draft.from !== value.from || draft.to !== value.to;
  // The day the next pick pairs with: the first pick, or the end that is not being replaced.
  const pivot = anchor ?? (editing === 'from' ? draft.to : editing === 'to' ? draft.from : '');
  // While a pick is pending, the range follows the pointer from the pivot, in either direction.
  const preview = pivot && hovered && hovered !== pivot ? ordered(pivot, hovered) : undefined;
  const shown = preview ?? draft;
  const from = parseDay(shown.from);
  const to = parseDay(shown.to);

  const bounds: Matcher[] = [];
  if (minDate) bounds.push({ before: minDate });
  if (maxDate) bounds.push({ after: maxDate });

  // Closing without applying drops the draft; the panel fades out on the committed range.
  function handleOpenChange(next: boolean) {
    setDraft(value);
    setAnchor(undefined);
    setEditing(undefined);
    setHovered(undefined);
    if (next) setMonth(monthFor(value, 'from', twoMonths));
    setOpen(next);
  }

  // The panel keeps showing what was applied while it fades out.
  function apply() {
    setAnchor(undefined);
    setEditing(undefined);
    setHovered(undefined);
    onValueChange(draft);
    setOpen(false);
  }

  function fill(next: DateRangeValue) {
    setDraft(next);
    setAnchor(undefined);
    setEditing(undefined);
  }

  function pickDay(date: Date) {
    const day = toDay(date);
    if (editing) {
      const other = editing === 'from' ? draft.to : draft.from;
      const otherMayStayOpen = (editing === 'from' ? openEnd : openStart) !== undefined;
      if (other) fill(ordered(day, other));
      else if (otherMayStayOpen) fill({ ...draft, [editing]: day });
      // The other end must be set, so this day waits for it like a first pick.
      else startRange(day);
      return;
    }
    if (anchor) fill(ordered(anchor, day));
    else startRange(day);
  }

  function startRange(day: string) {
    setAnchor(day);
    setEditing(undefined);
    setDraft({ from: day, to: '' });
  }

  function chooseEnd(end: RangeEnd) {
    setEditing(end);
    setAnchor(undefined);
    setMonth(monthFor(draft, end, twoMonths));
  }

  function clearEnd(end: RangeEnd) {
    setDraft({ ...draft, [end]: '' });
    setEditing(end);
    setAnchor(undefined);
  }

  const selectedFrom = parseDay(draft.from);
  const selectedTo = parseDay(draft.to);
  const selected: DateRange = {
    from: selectedFrom ?? selectedTo,
    to: selectedFrom ? selectedTo : undefined,
  };
  const between = from && to ? { after: from, before: to } : false;
  // A one-day range has no stretch to draw.
  const spans = shown.from !== '' && shown.to !== '' && shown.from < shown.to;
  const bandTo = preview ? 'to-accent/60' : 'to-accent';

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
                    onClick={() => {
                      fill(preset.value);
                      // A preset can land far from the months on screen; bring it into view with its end's
                      // month last, so a range inside one month never opens on an empty one.
                      const end = parseDay(preset.value.to) ?? new Date();
                      setMonth(twoMonths ? addMonths(end, -1) : end);
                    }}
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
                value={shown.from}
                empty={openStart ?? labels.pick}
                active={editing === 'from'}
                clearLabel={openStart !== undefined && draft.from ? labels.clearFrom : undefined}
                onChoose={chooseEnd}
                onClear={clearEnd}
              />
              <RangeEndTab
                end="to"
                label={labels.to}
                value={shown.to}
                empty={openEnd ?? labels.pick}
                active={editing === 'to'}
                clearLabel={openEnd !== undefined && draft.to ? labels.clearTo : undefined}
                onChoose={chooseEnd}
                onClear={clearEnd}
              />
            </div>
            {/* A day focused from the keyboard previews like a hovered one, until focus leaves the grid. */}
            <div
              onBlur={(event) => {
                if (!event.currentTarget.contains(event.relatedTarget)) setHovered(undefined);
              }}
            >
              <Calendar
                mode="range"
                autoFocus
                numberOfMonths={twoMonths ? 2 : 1}
                month={month}
                onMonthChange={setMonth}
                selected={selected}
                // Controlled: without onSelect day-picker keeps its own copy of the range, which drifts
                // from the draft and marks the wrong days selected (and announces them so).
                onSelect={() => {}}
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
                  in_range: preview ? false : between,
                  preview: preview ? between : false,
                }}
                // The band lives on the cells: whole behind the days between, half behind each end.
                modifiersClassNames={{
                  in_range: 'bg-accent',
                  preview: 'bg-accent/60',
                  range_from: spans
                    ? cn('bg-linear-to-r from-transparent from-50% to-50%', bandTo)
                    : '',
                  range_to: spans
                    ? cn('bg-linear-to-l from-transparent from-50% to-50%', bandTo)
                    : '',
                }}
              />
            </div>
            <div className="flex justify-end gap-x-2">
              <Button variant="outline" size="sm" onClick={() => handleOpenChange(false)}>
                {labels.cancel}
              </Button>
              <Button size="sm" disabled={!complete || !changed} onClick={apply}>
                {labels.apply}
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
  // Names the clear button, which shows only while the end holds a day it may drop.
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
  const clearable = clearLabel !== undefined;
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
