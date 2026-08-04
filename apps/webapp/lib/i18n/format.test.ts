import { describe, expect, it } from 'vitest';

import {
  formatDate,
  formatList,
  formatRatePct,
  formatSignedValue,
  formatTimestamp,
  formatValue,
} from '@/lib/i18n/format';
import { TIME_ZONE } from '@/lib/i18n/locales';

/*
 * These assert what the module decides — rounding, sign, anchoring, notation — and not the
 * exact glyphs ICU emits, which change with the Node build. A test pinned to "2 ene 2025"
 * goes red on an ICU upgrade without anything having broken.
 */

describe('formatValue', () => {
  it('groups thousands', () => {
    expect(formatValue(1234567)).toBe('1.234.567');
  });

  // An integer quantity should not render as "5,00" just because the type allows decimals.
  it('drops the fraction for a whole number', () => {
    expect(formatValue(5)).toBe('5');
  });

  it('keeps up to two decimals by default', () => {
    expect(formatValue(1234.5)).toBe('1.234,5');
    expect(formatValue(1234.567)).toBe('1.234,57');
  });

  it('honours a tighter maxDecimals', () => {
    expect(formatValue(1234.567, { maxDecimals: 1 })).toBe('1.234,6');
  });

  // \s rather than a literal space: ICU separates the unit with a non-breaking one.
  it('abbreviates in compact notation', () => {
    expect(formatValue(1_500_000, { compact: true })).toMatch(/^1,5\sM$/);
  });

  it('handles zero without a sign', () => {
    expect(formatValue(0)).toBe('0');
  });
});

describe('formatSignedValue', () => {
  it.each([
    [5, '+5'],
    [-5, '-5'],
    [0, '0'],
  ])('renders %p as %p', (value, expected) => {
    expect(formatSignedValue(value)).toBe(expected);
  });

  /*
   * signDisplay: 'exceptZero' is evaluated against the ROUNDED value, so a magnitude that
   * rounds away to nothing renders "0" rather than the "-0" a naive sign check produces.
   */
  it('never renders a negative zero', () => {
    expect(formatSignedValue(-0.004)).toBe('0');
  });
});

describe('formatRatePct', () => {
  it.each([
    [0.21, '21%'],
    [0.105, '10,5%'],
    [0, '0%'],
    [1, '100%'],
  ])('renders the ratio %p as %p', (ratio, expected) => {
    expect(formatRatePct(ratio)).toBe(expected);
  });
});

describe('formatDate', () => {
  /*
   * A date-only value is anchored at local midnight. Parsed as UTC it would shift back a day
   * everywhere west of Greenwich — Argentina included — so a quote dated the 2nd would read
   * as the 1st.
   */
  it('does not shift a date-only value to the previous day', () => {
    expect(formatDate('2025-01-02')).toContain('2');
    expect(formatDate('2025-01-02')).not.toContain('1 de');
  });

  it('renders the day, month and year of a full timestamp', () => {
    const formatted = formatDate('2025-03-15T18:30:00Z');
    expect(formatted).toMatch(/15/);
    expect(formatted).toMatch(/mar/i);
    expect(formatted).toMatch(/2025/);
  });
});

describe('formatTimestamp', () => {
  /*
   * Rendered in the given zone so the calendar day is right for the viewer: 02:30 UTC on the
   * 3rd is still the evening of the 2nd in Buenos Aires. Comparing the two zones asserts that
   * without pinning a clock format, which differs by ICU version.
   */
  it('renders the calendar day of the given zone, not of UTC', () => {
    const instant = '2025-01-03T02:30:00Z';
    expect(formatTimestamp(instant, undefined, TIME_ZONE)).toMatch(/\b2\b/);
    expect(formatTimestamp(instant, undefined, 'UTC')).toMatch(/\b3\b/);
  });
});

describe('formatList', () => {
  it('joins with a conjunction', () => {
    expect(formatList(['cemento', 'arena', 'cal'])).toBe('cemento, arena y cal');
  });

  it.each([
    [['cemento'], 'cemento'],
    [[], ''],
  ])('handles the %p case', (items, expected) => {
    expect(formatList(items)).toBe(expected);
  });
});
