import { describe, expect, it } from 'vitest';

import { schemaText } from '@repo/vitest-config/schema-text';
import { branchSchema, type BranchValues } from '@/app/(protected)/settings/branches/form-schema';
import { EXPIRY_MAX_DAYS, EXPIRY_MIN_DAYS } from '@/lib/constants/branch';
import { TEXT_FIELD_MAX_LENGTH } from '@/lib/constants/forms';

const VALID: BranchValues = { name: 'Casa Central', address: '', defaultExpiryDays: '7' };

function messagesFor(values: Partial<BranchValues>): Record<string, string> {
  const result = branchSchema().safeParse({ ...VALID, ...values });
  if (result.success) return {};
  return Object.fromEntries(
    result.error.issues.map((issue) => [String(issue.path[0]), issue.message]),
  );
}

describe('branchSchema', () => {
  it('accepts a branch with no address', () => {
    expect(branchSchema().safeParse(VALID).success).toBe(true);
  });

  it('trims the name it accepts', () => {
    const parsed = branchSchema().safeParse({ ...VALID, name: '  Villa Bosch  ' });

    expect(parsed.success && parsed.data.name).toBe('Villa Bosch');
  });

  // Trimmed before the check, or a name of nothing but spaces reaches the API as a blank the
  // listing cannot render.
  it('refuses a name of nothing but spaces', () => {
    expect(messagesFor({ name: '   ' }).name).toBe('name.required');
  });

  it('refuses a name past the length the API stores', () => {
    expect(messagesFor({ name: 'a'.repeat(TEXT_FIELD_MAX_LENGTH + 1) }).name).toBe('tooLong');
  });

  /*
   * The expiry stays a string in the form, so the range has to be checked on what the caller
   * typed. Anything that is not a plain count of days is refused rather than coerced — `Number('')`
   * is 0 and `Number('7d')` is NaN, and both would otherwise slip past a bare comparison.
   */
  it.each(['', ' '])('reports an expiry of %p as missing', (raw) => {
    expect(messagesFor({ defaultExpiryDays: raw }).defaultExpiryDays).toBe(
      'defaultExpiryDays.required',
    );
  });

  it.each(['0', '7.5', '-3', '7d', 'siete', String(EXPIRY_MAX_DAYS + 1)])(
    'reports an expiry of %p as out of range',
    (raw) => {
      expect(messagesFor({ defaultExpiryDays: raw }).defaultExpiryDays).toBe(
        'defaultExpiryDays.outOfRange',
      );
    },
  );

  it.each([String(EXPIRY_MIN_DAYS), '30', String(EXPIRY_MAX_DAYS)])(
    'accepts an expiry of %p',
    (raw) => {
      expect(branchSchema().safeParse({ ...VALID, defaultExpiryDays: raw }).success).toBe(true);
    },
  );

  it('resolves each message through the catalog it belongs to', () => {
    const schema = branchSchema(schemaText(true));

    expect(schema.safeParse({ ...VALID, name: '' }).error?.issues[0]?.message).toBe(
      'field:name.required',
    );
    expect(
      schema.safeParse({ ...VALID, name: 'a'.repeat(TEXT_FIELD_MAX_LENGTH + 1) }).error?.issues[0]
        ?.message,
    ).toBe('shared:tooLong');
  });
});
