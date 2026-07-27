import { numberFormat } from '@/lib/i18n/intl-cache';
import { getLocaleTag } from '@/lib/i18n/locales';

// Extend before quoting in a 0-decimal (JPY) or 3-decimal currency.
const CURRENCY_DECIMALS: Record<string, number> = { ARS: 2, USD: 2 };

function currencyDecimals(currency: string): number {
  return CURRENCY_DECIMALS[currency] ?? 2;
}

// "1234.56" ARS → "$ 1.234,56". Money crosses the API as a decimal string matching NUMERIC(14,2);
// this is the one place it becomes display text. Blank/non-numeric passes through, since Number('')
// is 0 and would render a spurious "$ 0,00".
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
