import { describe, expect, it } from 'vitest';

import {
  deleteAcrossSeparator,
  editTaxId,
  formatTaxId,
  hasTaxIdLength,
  keepAllowed,
  pastedTaxId,
  taxIdDigits,
} from '@/lib/utils/tax-id';

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

describe('editTaxId', () => {
  it('keeps a separator typed at the end where the next literal goes', () => {
    expect(editTaxId('20-', 3, '20', 'cuit')).toEqual({ digits: '20', display: '20-', caret: 3 });
    expect(editTaxId('20-12345678-', 12, '20-12345678', 'cuit').display).toBe('20-12345678-');
    expect(editTaxId('12.', 3, '12', 'dni').display).toBe('12.');
    expect(editTaxId('1.234.', 6, '1.234', 'dni').display).toBe('1.234.');
  });

  it('takes the literal of the shape the next digit gives a CUIT or DNI', () => {
    expect(editTaxId('20.', 3, '20', 'cuit-or-dni').display).toBe('20.');
    expect(editTaxId('20-', 3, '20', 'cuit-or-dni').display).toBe('20');
    expect(editTaxId('20-12345678-', 12, '20-12345678', 'cuit-or-dni').display).toBe(
      '20-12345678-',
    );
  });

  it('drops a separator anywhere else, and the caret stays put', () => {
    expect(editTaxId('20.', 3, '20', 'cuit')).toEqual({ digits: '20', display: '20', caret: 2 });
    expect(editTaxId('201-', 4, '20-1', 'cuit')).toEqual({
      digits: '201',
      display: '20-1',
      caret: 4,
    });
  });

  it('steps the caret over the same literal typed right before it', () => {
    expect(editTaxId('20--1234', 3, '20-1234', 'cuit')).toEqual({
      digits: '201234',
      display: '20-1234',
      caret: 3,
    });
    expect(editTaxId('20.-1234', 3, '20-1234', 'cuit').caret).toBe(2);
  });

  it('keeps the caret after a separator a letter was typed behind', () => {
    expect(editTaxId('20-x1', 4, '20-1', 'cuit').caret).toBe(3);
  });

  it('keeps a separator left at the end by a deleted digit', () => {
    expect(editTaxId('20-', 3, '20-1', 'cuit').display).toBe('20-');
    expect(editTaxId('1.23', 4, '1.234', 'dni').display).toBe('123');
  });
});

describe('deleteAcrossSeparator', () => {
  it('takes the digit beyond a separator inside the number', () => {
    expect(deleteAcrossSeparator('20-1234', 3, 'Backspace', 'cuit')).toEqual({
      digits: '21234',
      display: '21-234',
      caret: 1,
    });
    expect(deleteAcrossSeparator('20-1234', 2, 'Delete', 'cuit')).toEqual({
      digits: '20234',
      display: '20-234',
      caret: 2,
    });
  });

  it('leaves a separator at the end, and any digit, to the browser', () => {
    expect(deleteAcrossSeparator('20-', 3, 'Backspace', 'cuit')).toBeNull();
    expect(deleteAcrossSeparator('20-', 2, 'Delete', 'cuit')).toBeNull();
    expect(deleteAcrossSeparator('20-1234', 5, 'Backspace', 'cuit')).toBeNull();
    expect(deleteAcrossSeparator('20-1234', 3, 'ArrowLeft', 'cuit')).toBeNull();
  });
});

describe('pastedTaxId', () => {
  it('keeps the digits that fit, with the separators among them', () => {
    expect(pastedTaxId(' 20-12345678-9 ', 11)).toBe('20-12345678-9');
    expect(pastedTaxId('20.123.456', 4)).toBe('20.12');
    expect(pastedTaxId('abc', 4)).toBe('');
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
