import { numberFormat } from '@/lib/i18n/intl-cache';
import { getLocaleTag } from '@/lib/i18n/locales';

// Minor-unit exponent per currency (ARS/USD → 2 decimals). Extend for 0-decimal (e.g. JPY) or
// 3-decimal currencies before quoting in them.
const MINOR_UNITS: Record<string, number> = { ARS: 2, USD: 2 };

function minorUnits(currency: string): number {
  return MINOR_UNITS[currency] ?? 2;
}

// Formats an integer amount in MINOR units (e.g. centavos) as localized currency —
// 123456 ARS → "$ 1.234,56". Money crosses the API boundary as integer minor units (matching the
// Go side's int64 centavos) to avoid floating-point drift; this is the single place it becomes a
// display string.
export function formatCurrency(minorAmount: number, locale?: string, currency = 'ARS'): string {
  const exp = minorUnits(currency);
  const major = minorAmount / 10 ** exp;
  return numberFormat(getLocaleTag(locale), {
    style: 'currency',
    currency,
    minimumFractionDigits: exp,
    maximumFractionDigits: exp,
  }).format(major);
}
