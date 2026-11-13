import { describe, expect, it } from 'vitest';

import { dateRangePreset } from '@/lib/utils/date-ranges';

// A Wednesday early in a month, so "last 7/30 days" cross a month boundary and "last month" a year one.
const TODAY = new Date(2026, 0, 7, 18, 30);

describe('dateRangePreset', () => {
  it('counts today among the last N days', () => {
    expect(dateRangePreset('last7', TODAY)).toEqual({ from: '2026-01-01', to: '2026-01-07' });
    expect(dateRangePreset('last30', TODAY)).toEqual({ from: '2025-12-09', to: '2026-01-07' });
  });

  it('runs this month and this year up to today', () => {
    expect(dateRangePreset('thisMonth', TODAY)).toEqual({ from: '2026-01-01', to: '2026-01-07' });
    expect(dateRangePreset('thisYear', TODAY)).toEqual({ from: '2026-01-01', to: '2026-01-07' });
  });

  it('takes the whole previous month, across the year', () => {
    expect(dateRangePreset('lastMonth', TODAY)).toEqual({ from: '2025-12-01', to: '2025-12-31' });
    expect(dateRangePreset('lastMonth', new Date(2028, 2, 15))).toEqual({
      from: '2028-02-01',
      to: '2028-02-29',
    });
  });
});
