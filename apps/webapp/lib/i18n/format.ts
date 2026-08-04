import { dateTimeFormat, listFormat, numberFormat } from '@/lib/i18n/intl-cache';
import { getLocaleTag } from '@/lib/i18n/locales';

export interface FormatValueOptions {
  locale?: string;
  compact?: boolean;
  // Max fraction digits for non-compact output.
  maxDecimals?: number;
}

// Thousand separators, stripping .00 for integers. `compact: true` abbreviates ("1,5 M").
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

// `signDisplay: 'exceptZero'` is evaluated against the ROUNDED value, so a magnitude rounding to
// zero renders "0", never "-0".
export function formatSignedValue(value: number, locale?: string): string {
  const hasDecimals = value % 1 !== 0;
  return numberFormat(getLocaleTag(locale), {
    minimumFractionDigits: 0,
    maximumFractionDigits: hasDecimals ? 2 : 0,
    signDisplay: 'exceptZero',
  }).format(value);
}

// 0.21 → "21%", 0.105 → "10,5%".
export function formatRatePct(ratio: number, locale?: string): string {
  return numberFormat(getLocaleTag(locale), {
    style: 'percent',
    minimumFractionDigits: 0,
    maximumFractionDigits: 1,
  }).format(ratio);
}

// "2 de ene de 2025". Date-only input (YYYY-MM-DD) is anchored at local midnight so it never
// timezone-shifts to the previous day.
export function formatDate(iso: string, locale?: string): string {
  const date = iso.length === 10 ? new Date(iso + 'T00:00:00') : new Date(iso);
  return dateTimeFormat(getLocaleTag(locale), {
    day: 'numeric',
    month: 'short',
    year: 'numeric',
  }).format(date);
}

// "2 de ene de 2025, 11:30 p. m.", rendered in `timeZone` so the calendar day is right for
// the viewer.
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

// "cemento, arena y cal".
export function formatList(items: Iterable<string>, locale?: string): string {
  return listFormat(getLocaleTag(locale), { style: 'long', type: 'conjunction' }).format(items);
}
