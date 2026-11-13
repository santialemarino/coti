import { format, isValid, parse } from 'date-fns';

// The calendar day a form holds and the API speaks, e.g. `2026-11-13`.
const DAY_FORMAT = 'yyyy-MM-dd';
// How a day reads on screen in Argentina.
const DISPLAY_FORMAT = 'dd/MM/yyyy';

/*
 * A calendar day as a local-midnight Date. A bare `new Date('2026-11-13')` is UTC midnight, which is
 * the previous evening in Argentina and lands the calendar one day early.
 */
export function parseDay(value: string | undefined): Date | undefined {
  if (!value || !/^\d{4}-\d{2}-\d{2}$/.test(value)) return undefined;
  const date = parse(value, DAY_FORMAT, new Date());
  return isValid(date) ? date : undefined;
}

export function toDay(date: Date): string {
  return format(date, DAY_FORMAT);
}

// `13/11/2026` for `2026-11-13`; an unreadable value is returned as it came.
export function displayDay(value: string): string {
  const date = parseDay(value);
  return date ? format(date, DISPLAY_FORMAT) : value;
}
