import { toDay } from '@repo/ui/lib';

export const DATE_RANGE_PRESETS = [
  'last7',
  'last30',
  'thisMonth',
  'lastMonth',
  'thisYear',
] as const;
export type DateRangePresetKey = (typeof DATE_RANGE_PRESETS)[number];

// Each preset ends today at the latest, both ends inclusive, as the calendar days the API takes.
export function dateRangePreset(
  key: DateRangePresetKey,
  today: Date,
): { from: string; to: string } {
  const year = today.getFullYear();
  const month = today.getMonth();
  const day = today.getDate();
  switch (key) {
    case 'last7':
      return { from: toDay(new Date(year, month, day - 6)), to: toDay(today) };
    case 'last30':
      return { from: toDay(new Date(year, month, day - 29)), to: toDay(today) };
    case 'thisMonth':
      return { from: toDay(new Date(year, month, 1)), to: toDay(today) };
    case 'lastMonth':
      // Day 0 of a month is the last day of the one before.
      return { from: toDay(new Date(year, month - 1, 1)), to: toDay(new Date(year, month, 0)) };
    case 'thisYear':
      return { from: toDay(new Date(year, 0, 1)), to: toDay(today) };
  }
}
