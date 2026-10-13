import { describe, expect, it } from 'vitest';

import { hasValidCuitCheckDigit, isValidCuit } from '@/lib/utils/cuit';

describe('isValidCuit', () => {
  it('accepts a CUIT with or without separators', () => {
    expect(isValidCuit('30-71234567-1')).toBe(true);
    expect(isValidCuit('20329642330')).toBe(true);
  });

  it('refuses a wrong check digit or a value that is not eleven digits', () => {
    expect(isValidCuit('30-71234567-9')).toBe(false);
    expect(isValidCuit('28.123.456')).toBe(false);
  });
});

describe('hasValidCuitCheckDigit', () => {
  it('only judges eleven-digit values', () => {
    expect(hasValidCuitCheckDigit('28.123.456')).toBe(true);
    expect(hasValidCuitCheckDigit('30-71234567-9')).toBe(false);
  });
});
