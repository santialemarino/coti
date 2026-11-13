import { describe, expect, it } from 'vitest';

import { displayDay, parseDay, toDay } from './dates';

describe('parseDay', () => {
  // UTC midnight is the previous evening in Argentina, which would land the calendar a day early.
  it('anchors a day at local midnight', () => {
    const date = parseDay('2026-11-13')!;

    expect([date.getFullYear(), date.getMonth(), date.getDate(), date.getHours()]).toEqual([
      2026, 10, 13, 0,
    ]);
  });

  it('refuses anything that is not a real calendar day', () => {
    expect(parseDay('')).toBeUndefined();
    expect(parseDay(undefined)).toBeUndefined();
    expect(parseDay('13/11/2026')).toBeUndefined();
    expect(parseDay('2026-02-30')).toBeUndefined();
    expect(parseDay('2026-11-13T00:00:00')).toBeUndefined();
  });
});

describe('toDay', () => {
  it('writes the local calendar day, zero-padded', () => {
    expect(toDay(new Date(2026, 0, 5, 23, 59))).toBe('2026-01-05');
  });
});

describe('displayDay', () => {
  it('reads as an Argentine date', () => {
    expect(displayDay('2026-01-05')).toBe('05/01/2026');
  });

  it('leaves an unreadable value as it came', () => {
    expect(displayDay('mañana')).toBe('mañana');
  });
});
