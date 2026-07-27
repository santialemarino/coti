import { formatCurrency } from '@/lib/i18n/currency';
import {
  formatDate,
  formatList,
  formatRatePct,
  formatSignedValue,
  formatTimestamp,
  formatValue,
  type FormatValueOptions,
} from '@/lib/i18n/format';

/*
 * The locale-bound formatter set. Methods close over the locale and timezone so call sites never
 * thread them and cannot silently format in the wrong locale. Pure, so the client hook and the
 * server helper both reuse it.
 */
export function createFormatters(locale: string, timeZone?: string) {
  return {
    locale,
    value: (value: number, options?: Omit<FormatValueOptions, 'locale'>) =>
      formatValue(value, { ...options, locale }),
    currency: (amount: string, currency?: string) => formatCurrency(amount, locale, currency),
    signedValue: (value: number) => formatSignedValue(value, locale),
    ratePct: (ratio: number) => formatRatePct(ratio, locale),
    date: (iso: string) => formatDate(iso, locale),
    timestamp: (iso: string) => formatTimestamp(iso, locale, timeZone),
    list: (items: Iterable<string>) => formatList(items, locale),
  };
}

export type Formatters = ReturnType<typeof createFormatters>;
