import { describe, expect, it } from 'vitest';

import { formatTaxId, hasTaxIdLength, keepAllowed, taxIdDigits } from '@/lib/utils/tax-id';

describe('formatTaxId', () => {
  it('adds each CUIT separator only once a digit follows it', () => {
    expect(formatTaxId('20', 'cuit')).toBe('20');
    expect(formatTaxId('201', 'cuit')).toBe('20-1');
    expect(formatTaxId('2012345678', 'cuit')).toBe('20-12345678');
    expect(formatTaxId('20123456789', 'cuit')).toBe('20-12345678-9');
  });

  it('groups a DNI in thousands from the right', () => {
    expect(formatTaxId('123', 'dni')).toBe('123');
    expect(formatTaxId('1234567', 'dni')).toBe('1.234.567');
    expect(formatTaxId('12345678', 'dni')).toBe('12.345.678');
  });

  it('reads a CUIT or DNI as a DNI up to eight digits and as a CUIT from nine', () => {
    expect(formatTaxId('12345678', 'cuit-or-dni')).toBe('12.345.678');
    expect(formatTaxId('201234567', 'cuit-or-dni')).toBe('20-1234567');
  });
});

describe('hasTaxIdLength', () => {
  it('holds a CUIT to eleven digits and a DNI to seven or eight', () => {
    expect(hasTaxIdLength('20123456789', 'cuit')).toBe(true);
    expect(hasTaxIdLength('12345678', 'cuit')).toBe(false);
    expect(hasTaxIdLength('1234567', 'dni')).toBe(true);
    expect(hasTaxIdLength('123456', 'dni')).toBe(false);
    expect(hasTaxIdLength('20123456789', 'cuit-or-dni')).toBe(true);
    expect(hasTaxIdLength('12345678', 'cuit-or-dni')).toBe(true);
    expect(hasTaxIdLength('123456789', 'cuit-or-dni')).toBe(false);
  });
});

describe('taxIdDigits', () => {
  it('keeps only the digits of any common shape', () => {
    expect(taxIdDigits(' 20.12345678.9 ')).toBe('20123456789');
    expect(taxIdDigits('20 12345678 9')).toBe('20123456789');
  });
});

describe('keepAllowed', () => {
  const GROSS_INCOME = /^[\d-]$/;

  it('drops what the pattern refuses and keeps the caret after what it followed', () => {
    expect(keepAllowed('90.1a-23', 5, GROSS_INCOME)).toEqual({ value: '901-23', caret: 3 });
  });

  it('settles a pasted number with spaces and dots', () => {
    expect(keepAllowed(' 901 234.567-8 ', 15, GROSS_INCOME)).toEqual({
      value: '901234567-8',
      caret: 11,
    });
  });
});
