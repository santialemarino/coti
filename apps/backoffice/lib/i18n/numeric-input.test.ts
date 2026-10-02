import { describe, expect, it } from 'vitest';

import { stepCanonical } from '@/lib/i18n/numeric-input';

describe('stepCanonical', () => {
  it('steps a whole quantity to a whole quantity, whatever decimals the field allows', () => {
    expect(stepCanonical('ArrowUp', '1', { maxDecimals: 2 })).toBe('2');
    expect(stepCanonical('ArrowDown', '5', { maxDecimals: 2 })).toBe('4');
  });

  it('keeps the decimals already written', () => {
    expect(stepCanonical('ArrowUp', '1.5', { maxDecimals: 2 })).toBe('2.5');
    expect(stepCanonical('ArrowUp', '1.50', { maxDecimals: 2 })).toBe('2.50');
  });

  it('never writes more decimals than the field allows', () => {
    expect(stepCanonical('ArrowUp', '1', { step: 0.25, maxDecimals: 1 })).toMatch(/^1\.\d$/);
    expect(stepCanonical('ArrowUp', '1', { step: 0.5, maxDecimals: 0 })).toMatch(/^\d+$/);
  });

  it('does not let floating point drift into the value', () => {
    expect(stepCanonical('ArrowUp', '0.2', { step: 0.1 })).toBe('0.3');
  });

  it('stops at the minimum and ignores keys that are not arrows', () => {
    expect(stepCanonical('ArrowDown', '0', { maxDecimals: 2 })).toBe('0');
    expect(stepCanonical('Enter', '1', { maxDecimals: 2 })).toBeNull();
  });
});
