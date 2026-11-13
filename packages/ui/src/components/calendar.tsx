'use client';

import * as React from 'react';
import { ChevronLeftIcon, ChevronRightIcon } from 'lucide-react';
import { useReducedMotion } from 'motion/react';
import {
  DayPicker,
  labelDayButton,
  type DayButtonProps,
  type DayProps,
  type DropdownProps,
} from 'react-day-picker';
import { es } from 'react-day-picker/locale';

import { cn } from '../lib/utils';
import { Combobox } from './combobox';

type CalendarProps = React.ComponentProps<typeof DayPicker>;

// Icon-only: a colour shift that stays plus the bump, never a ring.
const NAV_BUTTON = cn(
  'group/nav pointer-events-auto inline-flex size-(--cell) items-center justify-center rounded-lg text-foreground-muted',
  'outline-none transition-[color,background-color] duration-200 ease-out-soft',
  'hover:bg-surface-hover hover:text-foreground active:bg-surface-active focus-visible:bg-surface-hover focus-visible:text-foreground',
  'disabled:pointer-events-none disabled:opacity-50 aria-disabled:pointer-events-none aria-disabled:opacity-50',
);

/*
 * The button keeps its shape in every state; a range's band is drawn on the cell behind it. States
 * overlap (today can be selected, an outside day can sit in a range), so each colour names the states
 * it yields to: selected, then in range or previewed, then today, then outside the month. Colours
 * change at once, never fade: the band on the cell cannot fade (its ends are gradients), and a button
 * fading over a band that has already moved reads as a blink when a preset swaps the range.
 */
const DAY_BUTTON = cn(
  'relative flex size-(--cell) items-center justify-center rounded-lg text-foreground',
  'outline-none transition-[box-shadow] duration-150 ease-out-soft',
  'focus-visible:z-10 focus-visible:ring-3 focus-visible:ring-ring/45',
  'disabled:pointer-events-none disabled:opacity-40',
  // A focused disabled day keeps focus, so day-picker marks it aria-disabled instead of disabled.
  'aria-disabled:pointer-events-none aria-disabled:opacity-40',
  'text-paragraph-sm tabular-nums data-today:text-paragraph-sm-semibold',
  // A plain day.
  'hover:bg-surface-hover active:bg-surface-active',
  'data-outside:not-data-selected:not-data-in-range:not-data-preview:text-foreground-subtle',
  'data-today:not-data-selected:not-data-in-range:not-data-preview:text-primary',
  // Today's blue fails contrast on the grey hover steps, so its hover and press take the blue family.
  'data-today:not-data-selected:not-data-in-range:not-data-preview:hover:bg-accent-strong',
  'data-today:not-data-selected:not-data-in-range:not-data-preview:hover:text-accent-foreground',
  'data-today:not-data-selected:not-data-in-range:not-data-preview:active:bg-accent-stronger',
  // Pressed without a hover (touch, or Space from the keyboard) needs the darker text on its own.
  'data-today:not-data-selected:not-data-in-range:not-data-preview:active:text-accent-foreground',
  // Inside a range or its preview: the band shows through, and hover and press step up from it.
  'data-in-range:not-data-selected:text-accent-foreground data-preview:not-data-selected:text-accent-foreground',
  'data-in-range:not-data-selected:hover:bg-accent-strong data-in-range:not-data-selected:active:bg-accent-stronger',
  'data-preview:not-data-selected:hover:bg-accent-strong data-preview:not-data-selected:active:bg-accent-stronger',
  // An end of the range, or the picked day: white on primary, a step darker on hover and press.
  'data-selected:bg-primary data-selected:text-primary-foreground',
  'data-selected:hover:bg-primary-hover data-selected:active:bg-primary-active',
);

/*
 * Day-picker's own markup with every class replaced: its stylesheet is never loaded, so nothing of
 * its look reaches the page. Days are surfaces and get the ring; inside the grid a focused day lifts
 * above its neighbours or the next cell clips the ring.
 */
const CLASS_NAMES: CalendarProps['classNames'] = {
  root: 'relative w-fit',
  months: 'relative flex flex-col gap-x-6 gap-y-4 sm:flex-row',
  month: 'flex flex-col gap-y-3',
  // The bar spans the captions, so only its two buttons take the pointer.
  nav: 'pointer-events-none absolute inset-x-0 top-0 flex h-(--cell) items-center justify-between',
  button_previous: NAV_BUTTON,
  button_next: NAV_BUTTON,
  month_caption: 'flex h-(--cell) items-center justify-center px-(--cell)',
  caption_label: 'text-paragraph-sm-semibold text-foreground first-letter:uppercase',
  dropdowns: 'flex items-center gap-x-1.5',
  month_grid: 'border-collapse',
  weekdays: 'flex',
  weekday: 'flex w-(--cell) items-center justify-center text-paragraph-xs text-foreground-subtle',
  weeks: 'flex flex-col gap-y-0.5',
  week: 'flex',
  day: 'relative size-(--cell) p-0',
  day_button: DAY_BUTTON,
  hidden: 'invisible',
  // A day from the next or previous month repeats one the other grid shows; it never carries a range.
  outside: 'bg-none! bg-transparent!',
  // Day-picker adds these with `classList.add`, so each is one class; the motion is in the styles.
  weeks_after_enter: 'calendar-weeks-after-enter',
  weeks_before_enter: 'calendar-weeks-before-enter',
  weeks_before_exit: 'calendar-weeks-before-exit',
  weeks_after_exit: 'calendar-weeks-after-exit',
  caption_after_enter: 'calendar-caption-enter',
  caption_before_enter: 'calendar-caption-enter',
  caption_after_exit: 'calendar-caption-exit',
  caption_before_exit: 'calendar-caption-exit',
};

/*
 * Day-picker names a day ", seleccionado" from its own `selected` flag; a day from a neighbouring
 * month carries no state here, so its name must not claim one either.
 */
const spanishDayLabel = es.labels?.labelDayButton;
const dayLabel = typeof spanishDayLabel === 'function' ? spanishDayLabel : labelDayButton;
const DAY_LABELS: CalendarProps['labels'] = {
  labelDayButton: (date, modifiers, options, dateLib) =>
    dayLabel(
      date,
      modifiers.outside ? { ...modifiers, selected: false } : modifiers,
      options,
      dateLib,
    ),
};

function CalendarChevron({ orientation }: { orientation?: 'up' | 'down' | 'left' | 'right' }) {
  const Icon = orientation === 'left' ? ChevronLeftIcon : ChevronRightIcon;
  return <Icon aria-hidden="true" className="size-4 group-focus-visible/nav:animate-focus-bump" />;
}

/*
 * A day's state reaches its button as data attributes, so the styles can be written against the
 * states a range picker adds (`range_from`, `range_to`, `in_range`, `preview`) as well as day-picker's.
 */
function CalendarDayButton({
  day,
  modifiers,
  range = false,
  className,
  ...props
}: DayButtonProps & { range?: boolean }) {
  const ref = React.useRef<HTMLButtonElement>(null);

  // Day-picker moves focus by re-rendering with a new `focused` day; its own button does this too.
  React.useEffect(() => {
    if (modifiers.focused) ref.current?.focus();
  }, [modifiers.focused]);

  return (
    <button
      ref={ref}
      data-day={day.isoDate}
      data-today={modifiers.today || undefined}
      data-outside={modifiers.outside || undefined}
      data-selected={
        (!modifiers.outside &&
          (range ? modifiers.range_from || modifiers.range_to : modifiers.selected)) ||
        undefined
      }
      data-in-range={(!modifiers.outside && modifiers.in_range) || undefined}
      data-preview={(!modifiers.outside && modifiers.preview) || undefined}
      className={cn('group/day', className)}
      {...props}
    >
      <span className="relative">
        {props.children}
        <span
          aria-hidden="true"
          className="absolute -bottom-1 left-1/2 hidden size-1 -translate-x-1/2 bg-primary rounded-full group-data-today/day:block group-data-selected/day:bg-primary-foreground"
        />
      </span>
    </button>
  );
}

/*
 * A day from a neighbouring month (greyed, or a blank cell beside two months) belongs to the other
 * grid, so it is never announced selected here either.
 */
function CalendarDay({ day, modifiers, ...props }: DayProps) {
  return (
    <td
      {...props}
      data-day={day.isoDate}
      aria-selected={modifiers.outside ? undefined : props['aria-selected']}
    />
  );
}

/*
 * In a range, only its two ends are drawn selected, read from the picker's own `range_from` and
 * `range_to`. Day-picker includes a custom modifier only while some visible day matches it, so its
 * absence means "not an end", never "fall back to `selected`".
 */
function RangeDayButton(props: DayButtonProps) {
  return <CalendarDayButton {...props} range />;
}

/*
 * The month and year selects are the design system's own dropdown, so the calendar never shows the
 * browser's. Day-picker hands over a `<select>`'s props; the change goes back as the event it expects.
 */
function CalendarDropdown({ options = [], value, onChange, disabled, ...props }: DropdownProps) {
  return (
    <Combobox
      options={options.map((option) => ({
        value: String(option.value),
        label: option.label,
        disabled: option.disabled,
      }))}
      value={value === undefined ? null : String(value)}
      onValueChange={(next) =>
        onChange?.({ target: { value: next } } as React.ChangeEvent<HTMLSelectElement>)
      }
      placeholder={props['aria-label'] ?? ''}
      aria-label={props['aria-label']}
      disabled={disabled}
      className="h-8 w-auto gap-x-1 px-2 capitalize"
      contentClassName="max-h-72"
      contentMinWidth={112}
    />
  );
}

/*
 * The design system's month grid: Spanish, weeks starting on Monday, every state on Coti's tokens.
 * It is the inside of `DatePicker` and `DateRangePicker`; reach for those rather than placing it bare.
 * Day-picker owns the keyboard: arrows move a day or a week, Page Up/Down a month (with Shift a year),
 * Home/End the week's edges.
 */
function Calendar({ className, classNames, components, animate, ...props }: CalendarProps) {
  // The month slide never ends under reduced motion, and day-picker waits for its end to clean up.
  const reduced = useReducedMotion();

  return (
    <DayPicker
      locale={es}
      // With two months side by side, a neighbour's days would repeat the other grid's, ends included.
      showOutsideDays={(props.numberOfMonths ?? 1) === 1}
      labels={DAY_LABELS}
      animate={animate ?? !reduced}
      className={cn('[--cell:--spacing(10)] sm:[--cell:--spacing(9)]', className)}
      classNames={{ ...CLASS_NAMES, ...classNames }}
      components={{
        Chevron: CalendarChevron,
        Day: CalendarDay,
        DayButton: props.mode === 'range' ? RangeDayButton : CalendarDayButton,
        Dropdown: CalendarDropdown,
        ...components,
      }}
      {...props}
    />
  );
}

export { Calendar };
export type { CalendarProps };
