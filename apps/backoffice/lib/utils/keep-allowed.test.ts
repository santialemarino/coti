import { describe, expect, it } from 'vitest';

import { keepAllowed } from '@/lib/utils/keep-allowed';

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
