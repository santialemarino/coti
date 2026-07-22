import { dateTimeFormat, listFormat, numberFormat } from '@/lib/i18n/intl-cache';
import { getLocaleTag } from '@/lib/i18n/locales';

export interface FormatValueOptions {
  locale?: string;
  compact?: boolean;
  // Max fraction digits for non-compact output.
  maxDecimals?: number;
}

// Formats a number with locale thousand separators, stripping .00 for integers. Pass
// `compact: true` for abbreviated output (e.g. "1,5 M"); the compact branch caps at 1 decimal.
export function formatValue(value: number, options: FormatValueOptions = {}): string {
  const { locale, compact = false, maxDecimals = 2 } = options;
  if (compact) {
    return numberFormat(getLocaleTag(locale), {
      notation: 'compact',
      compactDisplay: 'short',
      maximumFractionDigits: 1,
    }).format(value);
  }
  const hasDecimals = value % 1 !== 0;
  return numberFormat(getLocaleTag(locale), {
    minimumFractionDigits: 0,
    maximumFractionDigits: hasDecimals ? maxDecimals : 0,
  }).format(value);
}

// Formats a number with an explicit +/- sign. `signDisplay: 'exceptZero'` is evaluated against
// the ROUNDED value, so a magnitude that rounds to zero renders "0" — never a spurious "-0".
export function formatSignedValue(value: number, locale?: string): string {
  const hasDecimals = value % 1 !== 0;
  return numberFormat(getLocaleTag(locale), {
    minimumFractionDigits: 0,
    maximumFractionDigits: hasDecimals ? 2 : 0,
    signDisplay: 'exceptZero',
  }).format(value);
}

// Formats a decimal ratio as a percentage (e.g. 0.21 → "21%", 0.105 → "10,5%").
export function formatRatePct(ratio: number, locale?: string): string {
  return numberFormat(getLocaleTag(locale), {
    style: 'percent',
    minimumFractionDigits: 0,
    maximumFractionDigits: 1,
  }).format(ratio);
}

// Formats an ISO date as a medium, locale-ordered label (e.g. "2 ene 2025"). Date-only inputs
// (YYYY-MM-DD) are anchored at local midnight and never timezone-shifted; use `formatTimestamp`
// for full timestamps that must render in a specific zone.
export function formatDate(iso: string, locale?: string): string {
  const date = iso.length === 10 ? new Date(iso + 'T00:00:00') : new Date(iso);
  return dateTimeFormat(getLocaleTag(locale), {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
  }).format(date);
}

// Formats a full ISO timestamp as date + time (e.g. "2 ene 2025, 14:30"), rendered in `timeZone`
// (the app's Argentina zone) so the calendar day + clock time are correct for the viewer.
export function formatTimestamp(iso: string, locale?: string, timeZone?: string): string {
  return dateTimeFormat(getLocaleTag(locale), {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    timeZone,
  }).format(new Date(iso));
}

// Formats a list into a locale-aware conjunction (e.g. "cemento, arena y cal").
export function formatList(items: Iterable<string>, locale?: string): string {
  return listFormat(getLocaleTag(locale), { style: 'long', type: 'conjunction' }).format(items);
}
