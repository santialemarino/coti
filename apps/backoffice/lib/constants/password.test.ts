import { describe, expect, it } from 'vitest';

import {
  hasEveryCharacterClass,
  PASSWORD_MIN_LENGTH,
  passwordByteLength,
  passwordChecks,
} from '@/lib/constants/password';

describe('passwordChecks', () => {
  it('reports every rule the password satisfies', () => {
    expect(passwordChecks('Corralon-2026!')).toEqual({
      length: true,
      uppercase: true,
      lowercase: true,
      number: true,
      symbol: true,
    });
  });

  it.each([
    ['uppercase', 'corralon-2026!'],
    ['lowercase', 'CORRALON-2026!'],
    ['number', 'Corralon-abcd!'],
    ['symbol', 'Corralon20261'],
  ] as const)('reports %s as missing', (missing, password) => {
    expect(passwordChecks(password)[missing]).toBe(false);
    expect(hasEveryCharacterClass(password)).toBe(false);
  });

  /*
   * Accented letters count as letters and the ñ as a lowercase one, so a Spanish password is not
   * quietly told it has no lowercase — the checks are unicode-aware for the language the product
   * is written in.
   */
  it('reads Spanish letters as the classes they are', () => {
    expect(passwordChecks('Ñoñerías-2026')).toMatchObject({
      uppercase: true,
      lowercase: true,
      number: true,
      symbol: true,
    });
  });

  // An ordinary hyphen or period is a symbol; the rule is "not a letter or a number", so a caller
  // is never rejected for choosing punctuation that is not on some private list.
  it.each(['-', '.', '_', ' ', '!'])('counts %p as a symbol', (char) => {
    expect(passwordChecks(`Corralon2026${char}`).symbol).toBe(true);
  });

  it('measures length in characters and the cap in bytes', () => {
    const accented = 'á'.repeat(PASSWORD_MIN_LENGTH);

    expect(passwordChecks(accented).length).toBe(true);
    expect(passwordByteLength(accented)).toBe(PASSWORD_MIN_LENGTH * 2);
  });
});
