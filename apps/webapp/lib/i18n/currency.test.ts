import { describe, expect, it } from 'vitest';

import { formatCurrency } from '@/lib/i18n/currency';

describe('formatCurrency', () => {
  /*
   * Money crosses the API as a decimal string matching NUMERIC(14,2) — never a float, never
   * int64 centavos — and this is the one place it becomes display text.
   */
  it('renders a decimal string with the local separators', () => {
    expect(formatCurrency('1234.56')).toContain('1.234,56');
  });

  it('always shows both decimals, including on a round amount', () => {
    expect(formatCurrency('1000')).toContain('1.000,00');
  });

  it('keeps the precision the wire format carries', () => {
    expect(formatCurrency('0.05')).toContain('0,05');
  });

  it('renders a negative amount', () => {
    expect(formatCurrency('-250.00')).toContain('250,00');
    expect(formatCurrency('-250.00')).toMatch(/-/);
  });

  /*
   * Number('') is 0, so a blank would render "$ 0,00" and invent an amount that was never
   * quoted. Passing it through is what keeps an absent price visibly absent.
   */
  it.each(['', '   '])('passes a blank value (%p) through untouched', (amount) => {
    expect(formatCurrency(amount)).toBe(amount);
  });

  it.each(['not a number', '12,34'])('passes the non-numeric %p through untouched', (amount) => {
    expect(formatCurrency(amount)).toBe(amount);
  });

  it('quotes in the currency it is given', () => {
    expect(formatCurrency('1234.56', undefined, 'USD')).toContain('1.234,56');
  });
});
