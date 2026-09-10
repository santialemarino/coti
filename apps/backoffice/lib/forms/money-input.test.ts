import { describe, expect, it } from 'vitest';

import { decimalToMoneyInput, maskMoneyInput, moneyInputToDecimal } from '@/lib/forms/money-input';

describe('maskMoneyInput', () => {
  it.each([
    ['1', '1'],
    ['13', '13'],
    ['132', '132'],
    ['1324', '1.324'],
    ['13246', '13.246'],
    ['132467', '132.467'],
    ['1324678', '1.324.678'],
  ])('groups %s as %s while it is typed', (typed, shown) => {
    expect(maskMoneyInput(typed)).toBe(shown);
  });

  it('regroups what it already produced, so each keystroke is idempotent', () => {
    expect(maskMoneyInput('132.467')).toBe('132.467');
    expect(maskMoneyInput(maskMoneyInput('132467'))).toBe('132.467');
  });

  it('keeps a comma the seller has started but not filled', () => {
    expect(maskMoneyInput('132467,')).toBe('132.467,');
  });

  it('takes at most two decimals', () => {
    expect(maskMoneyInput('132467,899')).toBe('132.467,89');
  });

  it('drops separators the seller should never have to type', () => {
    expect(maskMoneyInput('1.3.2.4.6.7')).toBe('132.467');
    expect(maskMoneyInput('$ 132.467,89')).toBe('132.467,89');
    expect(maskMoneyInput('abc')).toBe('');
  });

  it('strips a leading zero rather than growing one', () => {
    expect(maskMoneyInput('0450')).toBe('450');
  });

  it('keeps a bare comma readable as nought-point-something', () => {
    expect(maskMoneyInput(',5')).toBe('0,5');
  });
});

describe('moneyInputToDecimal', () => {
  it.each([
    ['132.467,89', '132467.89'],
    ['132.467', '132467'],
    ['1.324', '1324'],
    ['0,5', '0.5'],
    ['132.467,', '132467'],
  ])('sends %s to the API as %s', (shown, decimal) => {
    expect(moneyInputToDecimal(shown)).toBe(decimal);
  });

  it('reports an empty field as empty, never as zero', () => {
    expect(moneyInputToDecimal('')).toBe('');
    expect(moneyInputToDecimal(',')).toBe('');
  });
});

describe('decimalToMoneyInput', () => {
  it.each([
    ['132467.89', '132.467,89'],
    ['132467', '132.467,00'],
    ['780.5', '780,50'],
    ['0.00', '0,00'],
  ])('seeds the field from %s as %s', (decimal, shown) => {
    expect(decimalToMoneyInput(decimal)).toBe(shown);
  });

  it('round-trips a stored amount back to the same decimal', () => {
    expect(moneyInputToDecimal(decimalToMoneyInput('132467.89'))).toBe('132467.89');
  });
});
