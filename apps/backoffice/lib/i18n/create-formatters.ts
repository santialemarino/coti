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
 * The locale-bound formatter set. Every method closes over the resolved locale (and timezone, for
 * timestamps) so call sites never thread them — which removes the silent-default footgun where a
 * forgotten `locale` argument renders the wrong locale with no error. Pure — no React, no
 * next-intl — so both the client hook (`useFormatters`) and the server helper (`getFormatters`)
 * reuse it.
 */
export function createFormatters(locale: string, timeZone?: string) {
  return {
    // The resolved locale, for the rare call site that needs it directly (e.g. localeCompare).
    locale,
    value: (value: number, options?: Omit<FormatValueOptions, 'locale'>) =>
      formatValue(value, { ...options, locale }),
    // Integer minor units (centavos) → localized currency. Defaults to ARS.
    currency: (minorAmount: number, currency?: string) =>
      formatCurrency(minorAmount, locale, currency),
    signedValue: (value: number) => formatSignedValue(value, locale),
    ratePct: (ratio: number) => formatRatePct(ratio, locale),
    date: (iso: string) => formatDate(iso, locale),
    timestamp: (iso: string) => formatTimestamp(iso, locale, timeZone),
    list: (items: Iterable<string>) => formatList(items, locale),
  };
}

export type Formatters = ReturnType<typeof createFormatters>;
