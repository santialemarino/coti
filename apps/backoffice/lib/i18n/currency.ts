import { numberFormat } from '@/lib/i18n/intl-cache';
import { getLocaleTag } from '@/lib/i18n/locales';

// Fraction digits per currency (ARS/USD → 2). Extend for 0-decimal (e.g. JPY) or 3-decimal
// currencies before quoting in them.
const CURRENCY_DECIMALS: Record<string, number> = { ARS: 2, USD: 2 };

function currencyDecimals(currency: string): number {
  return CURRENCY_DECIMALS[currency] ?? 2;
}

// Formats a decimal money string as localized currency — "1234.56" ARS → "$ 1.234,56". Money
// crosses the API as a decimal STRING matching the DB's NUMERIC(14,2) (never float, never int64
// centavos) to avoid precision loss; this is the single place it becomes a display string. A
// blank or non-numeric input passes through unchanged (Number('') is 0, which would otherwise
// render a spurious "$ 0,00").
export function formatCurrency(amount: string, locale?: string, currency = 'ARS'): string {
  if (!amount.trim()) return amount;
  const num = Number(amount);
  if (Number.isNaN(num)) return amount;
  const decimals = currencyDecimals(currency);
  return numberFormat(getLocaleTag(locale), {
    style: 'currency',
    currency,
    minimumFractionDigits: decimals,
    maximumFractionDigits: decimals,
  }).format(num);
}
